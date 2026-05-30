package commander

import (
	"fmt"
	"mycontainer/container"
	"mycontainer/store"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/shlex"
)

func parseCommand(targetCmd string) ([]string, error) {
	args, err := shlex.Split(targetCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to parse command line: %w", err)
	}

	if len(args) == 0 {
		return nil, fmt.Errorf("command line is empty")
	}

	return args, nil
}

func (c *commander) child() error {
	// TODO переделать временный костыль на синхронизацию через pipe
	time.Sleep(1 * time.Second)
	cont, err := store.GetContainerByID(c.containerID)
	if err != nil {
		return err
	}
	if err = syscall.Sethostname([]byte("my-container")); err != nil {
		return err
	}
	if err = c.mountRootFS(cont); err != nil {
		return err
	}
	if err := syscall.Unshare(syscall.CLONE_NEWCGROUP); err != nil {
		return fmt.Errorf("failed to unshare cgroup namespace: %w", err)
	}
	if err := syscall.Mount("none", "/sys/fs/cgroup", "cgroup2", 0, ""); err != nil {
		return fmt.Errorf("failed to mount cgroup2 inside namespace: %w", err)
	}
	args, err := parseCommand(cont.TargetCmd)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm", // Чтобы работали clear и стрелочки
	}
	return cmd.Run()
}

func (c *commander) mountRootFS(cont container.Container) error {
	if cont.MergedDir == "" {
		return fmt.Errorf("archive error: merged_dir is empty, overlay cannot be isolated")
	}

	// ХАК ДЛЯ SYSTEMD (Критично для pivot_root!):
	// Делаем ВЕСЬ корень "/" внутри этого namespace приватным.
	// Это отвязывает пространство монтирования контейнера от хоста,
	// чтобы unmount внутри контейнера не аффектил хост.
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root mount private: %w", err)
	}

	err := syscall.Mount(cont.MergedDir, cont.MergedDir, "", syscall.MS_BIND, "")
	if err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %w", err)
	}

	// Шаг 1: Создание директории для отката старой FS хоста.
	// Она создается внутри merged_dir, то есть виртуально в корне нашего будущего контейнера.
	putOldDir := filepath.Join(cont.MergedDir, ".put_old")
	if err := os.MkdirAll(putOldDir, 0700); err != nil {
		return fmt.Errorf("step 2 (mkdir .put_old) failed: %w", err)
	}

	// Шаг 2: Смена корня местами. Теперь корень контейнера — это merged_dir (солянка из слоев),
	// а старый корень хоста улетает во временную папку /.put_old
	if err := syscall.PivotRoot(cont.MergedDir, putOldDir); err != nil {
		return fmt.Errorf("step 3 (pivot_root) failed: %w", err)
	}

	// Шаг 3: Сдвигаем рабочую директорию процесса в новый, честный корень "/"
	// С этого момента пути вроде /var/lib/... хоста больше не существуют для процесса.
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("step 4 (chdir to /) failed: %w", err)
	}

	if err := os.MkdirAll("/proc", 0755); err != nil {
		return fmt.Errorf("failed to create /proc directory: %w", err)
	}

	// Шаг 4: Монтируем изолированный /proc для нового PID namespace
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("step 5 (mount /proc) failed: %w", err)
	}

	if err := os.MkdirAll("/sys", 0755); err != nil {
		return fmt.Errorf("failed to create /sys directory: %w", err)
	}
	// Шаг 4.5: Монтируем изолированный /sys (без этого cgroups и сети не заведутся)
	if err := syscall.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil {
		return fmt.Errorf("step 5.5 (mount /sys) failed: %w", err)
	}

	// Шаг 5: Лениво отмонтируем старую хостовую FS, которая сейчас лежит в /.put_old
	// Флаг MNT_DETACH полностью скрывает структуру хоста из видимости контейнера
	if err := syscall.Unmount("/.put_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("step 6 (unmount /.put_old) failed: %w", err)
	}

	// Шаг 6: Удаляем за собой временную папку. Теперь она пустая и доступна для удаления.
	if err := os.Remove("/.put_old"); err != nil {
		return fmt.Errorf("step 7 (remove /.put_old) failed: %w", err)
	}

	return nil
}
