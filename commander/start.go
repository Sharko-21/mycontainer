package commander

import (
	"fmt"
	container2 "mycontainer/container"
	"mycontainer/store"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func (c *commander) collectCommands() ([]string, error) {
	if len(c.args) == 1 {
		return nil, fmt.Errorf("no command specified")
	}
	commands := []string{childCommand}
	if c.containerID != "" {
		commands = append(commands, c.containerID)
	}
	return commands, nil
}

func (c *commander) clearCgroup(containerID string) {
	cgroupPath := filepath.Join("/sys/fs/cgroup/system.slice", "mycontainer-"+containerID)
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
}

func (c *commander) start() error {
	cont, err := store.GetContainerByID(c.containerID)
	if err != nil {
		return err
	}
	c.rootfs = cont.LowerDir
	commands, err := c.collectCommands()
	if err != nil {
		return err
	}
	defer c.clearCgroup(cont.ID)
	// "/proc/self/exe" запускаем тот же бинарный файл, который исполняется
	cmd := exec.Command("/proc/self/exe", commands...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", cont.LowerDir, cont.UpperDir, cont.WorkDir)
	err = syscall.Mount("overlay", cont.MergedDir, "overlay", 0, options)
	if err != nil {
		return err
	}
	defer syscall.Unmount(cont.MergedDir, syscall.MNT_DETACH)
	err = cmd.Start()
	if err != nil {
		return err
	}
	if err := c.createCgroups(cont, cmd.Process.Pid); err != nil {
		return fmt.Errorf("cgroup allocation failed: %w", err)
	}
	err = store.UpdateContainerStatus(c.containerID, cmd.Process.Pid, container2.RunningStatus)
	if err != nil {
		return err
	}
	waitErr := cmd.Wait()
	finalStatus := container2.CompletedStatus
	if waitErr != nil {
		finalStatus = container2.ErrorStatus
	}
	if err := store.UpdateContainerStatus(c.containerID, 0, finalStatus); err != nil {
		return fmt.Errorf("failed to update status to stopped: %w", err)
	}
	return nil
}

func (c *commander) createCgroups(cont container2.Container, targetPid int) error {
	parentSubtree := "/sys/fs/cgroup/system.slice/cgroup.subtree_control"
	if err := os.WriteFile(parentSubtree, []byte("+memory +cpu +pids"), 0644); err != nil {
		// На некоторых дистрибутивах (например, в WSL2 или кастомных ядрах)
		// часть контроллеров может быть недоступна, поэтому просто логируем debug
		c.logging.Debugf("[cgroup] Root subtree control write note: %v", err)
	}
	cgroupPath := c.getCgroupPath(cont.ID)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "memory.max"), []byte(strconv.Itoa(cont.MemoryLimit)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "memory.low"), []byte(strconv.Itoa(cont.MemoryRequest)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "cpu.weight"), []byte(strconv.Itoa(cont.CpuRequest)), 0644,
	); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(cgroupPath, "pids.max"), []byte("100"), 0644,
	); err != nil {
		return err
	}
	var cpuMaxStr string
	if cont.CPULimit == -1 {
		cpuMaxStr = fmt.Sprintf("max %d", cont.CPUPeriod)
	} else {
		cpuMaxStr = fmt.Sprintf("%d %d", cont.CPULimit, cont.CPUPeriod)
	}
	if err := os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte(cpuMaxStr), 0644); err != nil {
		return fmt.Errorf("failed to write cpu.max: %w", err)
	}
	pidStr := strconv.Itoa(targetPid)
	if err := os.WriteFile(filepath.Join(cgroupPath, "cgroup.procs"), []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("failed to move process to cgroup (cgroup.procs): %w", err)
	}
	return nil
}
