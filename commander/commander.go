package commander

import (
	"fmt"
	"log/slog"
	"mycontainer/logging"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func NewCommander(logger logging.Logger) (Commander, error) {
	args := os.Args[1:]
	if len(args) < 1 {
		return nil, fmt.Errorf("no command specified")
	}
	return &commander{
		args:    args,
		logging: logger,
	}, nil
}

type commander struct {
	debug         bool
	rootfs        string
	args          []string
	memoryRequest int
	memoryLimit   int
	cpuRequest    int
	cpuLimit      int
	cpuPeriod     int

	logging logging.Logger
}

type Commander interface {
	Parse() error
}

func (c *commander) Parse() error {
	if err := c.parseArgs(); err != nil {
		return err
	}
	if len(c.args) == 0 {
		return fmt.Errorf("no command specified")
	}
	switch c.args[0] {
	case runCommand:
		return c.run()
	case childCommand:
		return c.child()
	default:
		return fmt.Errorf("unknown command: %s", c.args[0])
	}
}

func (c *commander) run() error {
	if len(c.args) == 1 {
		return fmt.Errorf("no command specified")
	}
	commands := []string{childCommand}
	if c.rootfs != "" {
		commands = append(commands, "--rootfs", c.rootfs)
	}
	if c.debug {
		commands = append(commands, "--debug")
	}

	commands = append(commands, "--memory-limit", strconv.Itoa(c.memoryLimit))
	commands = append(commands, "--memory-request", strconv.Itoa(c.memoryRequest))
	commands = append(commands, "--cpu-request", strconv.Itoa(c.cpuRequest))

	if c.cpuLimit == -1 {
		commands = append(commands, "--cpu-limit", "max", strconv.Itoa(c.cpuPeriod))
	} else {
		commands = append(commands, "--cpu-limit", strconv.Itoa(c.cpuLimit), strconv.Itoa(c.cpuPeriod))
	}
	defer func() {
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Microsecond)
			if _, err := os.Stat(cgroupPath); err == nil {
				c.logging.Debugf("[host] Container finished. Cleaning up cgroup: %s", cgroupPath)
				if err = os.Remove(cgroupPath); err != nil {
					c.logging.Debugf("[host] Warning: failed to remove cgroup dir: %v", err)
					continue
				}
				c.logging.Debug("[host] Cgroup cleaned up successfully!")
				return
			}
		}
	}()
	cmd := exec.Command("/proc/self/exe", append(commands, c.args[1:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}
	return cmd.Run()
}

func (c *commander) child() error {
	if err := c.createCgroups(); err != nil {
		return err
	}
	if err := syscall.Sethostname([]byte("my-container")); err != nil {
		return err
	}
	if err := c.mountRootFS(); err != nil {
		return err
	}
	cmd := exec.Command(c.args[1], c.args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm", // Чтобы работали clear и стрелочки
	}
	return cmd.Run()
}

func (c *commander) createCgroups() error {
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "memory.max"), []byte(strconv.Itoa(c.memoryLimit)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "memory.low"), []byte(strconv.Itoa(c.memoryRequest)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "cpu.weight"), []byte(strconv.Itoa(c.cpuRequest)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "pids.max"), []byte("100"), 0644,
	); err != nil {
		return err
	}
	var cpuMaxStr string
	if c.cpuLimit == -1 {
		cpuMaxStr = fmt.Sprintf("max %d", c.cpuPeriod)
	} else {
		cpuMaxStr = fmt.Sprintf("%d %d", c.cpuLimit, c.cpuPeriod)
	}
	if err := os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte(cpuMaxStr), 0644); err != nil {
		return fmt.Errorf("failed to write cpu.max: %w", err)
	}
	pidStr := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("failed to move process to cgroup (cgroup.procs): %w", err)
	}
	return nil
}

func (c *commander) mountRootFS() error {
	if c.rootfs == "" {
		return fmt.Errorf("no rootfs specified")
	}

	// ХАК ДЛЯ SYSTEMD (Критично для pivot_root!):
	// Делаем ВЕСЬ корень "/" внутри этого namespace приватным.
	// Это отвязывает пространство монтирования контейнера от хоста.
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root mount private: %w", err)
	}

	// Шаг 1: Bind Mount rootfs на саму себя. Теперь это честный mount point.
	if err := syscall.Mount(c.rootfs, c.rootfs, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("step 1 (bind mount) failed: %w", err)
	}

	// Шаг 2: Создание директории для отката старой FS хоста.
	// filepath.Join соберет правильный путь, например: /home/yaroslav/alpine_rootfs/.put_old
	putOldDir := filepath.Join(c.rootfs, ".put_old")
	if err := os.MkdirAll(putOldDir, 0700); err != nil {
		return fmt.Errorf("step 2 (mkdir .put_old) failed: %w", err)
	}

	// Шаг 3: Смена корня местами. Старый корень улетает в .put_old
	if err := syscall.PivotRoot(c.rootfs, putOldDir); err != nil {
		return fmt.Errorf("step 3 (pivot_root) failed: %w", err)
	}

	// Шаг 4: Сдвигаем рабочую директорию процесса в новый, честный корень "/"
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("step 4 (chdir to /) failed: %w", err)
	}

	// Шаг 5: Монтируем изолированный /proc
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("step 5 (mount /proc) failed: %w", err)
	}

	if err := syscall.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil {
		return fmt.Errorf("step 5.5 (mount /sys) failed: %w", err)
	}

	// Шаг 6: Лениво отмонтируем старую хостовую FS, которая сейчас лежит в /.put_old
	// Флаг MNT_DETACH полностью скрывает её из видимости контейнера
	if err := syscall.Unmount("/.put_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("step 6 (unmount /.put_old) failed: %w", err)
	}

	// Шаг 7: Удаляем за собой временную папку. Теперь она пустая и доступна для удаления.
	if err := os.Remove("/.put_old"); err != nil {
		return fmt.Errorf("step 7 (remove /.put_old) failed: %w", err)
	}

	return nil
}

func (c *commander) parseArgs() error {
	args := make([]string, 0, len(c.args)+1)
	skipArgs := 0
	stopParsingFlag := false
	c.memoryRequest = memoryRequestDefault
	c.memoryLimit = memoryLimitDefault
	c.cpuRequest = cpuRequestDefault
	c.cpuLimit = cpuLimitDefault
	c.cpuPeriod = cpuPeriodDefault
	for i, arg := range c.args {
		if skipArgs > 0 {
			skipArgs = skipArgs - 1
			continue
		}
		if stopParsingFlag {
			args = append(args, arg)
			continue
		}
		if arg == "--" {
			stopParsingFlag = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg[2:] {
			case debugParam:
				c.logging.SetLogLevel(slog.LevelDebug)
				continue
			case rootfsParam:
				skipArgs = 1
				if len(c.args) > i+1 && c.args[i+1] != "" {
					c.rootfs = c.args[i+1]
				}
				continue
			case memoryLimitParam:
				skipArgs = 1
				if len(c.args) > i+1 && c.args[i+1] != "" {
					limit, err := strconv.Atoi(c.args[i+1])
					if err != nil {
						return err
					}
					if limit < 0 {
						return fmt.Errorf("memory limit cannot be negative: %d", limit)
					}
					c.memoryLimit = limit
				}
				continue
			case memoryRequestParam:
				skipArgs = 1
				if len(c.args) > i+1 && c.args[i+1] != "" {
					request, err := strconv.Atoi(c.args[i+1])
					if err != nil {
						return err
					}
					if request < 0 {
						return fmt.Errorf("invalid memory request: %d", request)
					}
					c.memoryRequest = request
				}
				continue
			case cpuRequestParam:
				skipArgs = 1
				if len(c.args) > i+1 && c.args[i+1] != "" {
					request, err := strconv.Atoi(c.args[i+1])
					if err != nil {
						return err
					}
					if request < 0 {
						return fmt.Errorf("invalid CPU request: %d", request)
					}
					c.cpuRequest = request
				}
				continue
			case cpuLimitParam:
				skipArgs = 2
				if i+2 >= len(c.args) {
					return fmt.Errorf("flag --cpu-limit requires TWO arguments: [limit] [period] (e.g. --cpu-limit 20000 100000 или max 100000)")
				}
				if c.args[i+1] != "" && c.args[i+2] != "" {
					if c.args[i+1] == "max" {
						c.cpuLimit = -1
					} else {
						limit, err := strconv.Atoi(c.args[i+1])
						if err != nil {
							return err
						}
						if limit < 0 {
							return fmt.Errorf("max limit should be greater or equal to 0")
						}
						c.cpuLimit = limit
					}
					period, err := strconv.Atoi(c.args[i+2])
					if err != nil {
						return err
					}
					c.cpuPeriod = period
				}
				continue
			}
		}
		args = append(args, arg)
	}
	c.args = args
	c.logging.Debugf("args: %v", args)
	c.logging.Debugf("args: %v", c.args)
	c.logging.Debugf("memoryLimit: %d", c.memoryLimit)
	c.logging.Debugf("memoryRequest: %d", c.memoryRequest)
	c.logging.Debugf("cpuRequest: %d", c.cpuRequest)
	c.logging.Debugf("cpuLimit: %d", c.cpuLimit)
	c.logging.Debugf("cpuPeriod: %d", c.cpuPeriod)
	return nil
}

// commands for execution
const runCommand = "run"
const childCommand = "child"

// execution params
const rootfsParam = "rootfs"
const memoryLimitParam = "memory-limit"
const memoryRequestParam = "memory-request"
const cpuLimitParam = "cpu-limit"
const cpuRequestParam = "cpu-request"
const debugParam = "debug"

// path
const cgroupPath = "/sys/fs/cgroup/my-container"

// default limits
const memoryLimitDefault = 536870912 // 512MB
const memoryRequestDefault = 0
const cpuLimitDefault = -1 // max
const cpuRequestDefault = 100
const cpuPeriodDefault = 100000 // 100ms
