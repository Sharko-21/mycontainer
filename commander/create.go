package commander

import (
	"fmt"
	"mycontainer/container"
	"mycontainer/store"
	"strings"
	"time"
)

func (c *commander) create() error {
	containerID, err := container.GenerateContainerID()
	if err != nil {
		return err
	}
	upper, work, merged, err := container.CreateContainerDirs(containerID)
	if err != nil {
		return err
	}
	err = store.InsertContainer(container.Container{
		ID:            containerID,
		PID:           0,
		Status:        container.CreatedStatus,
		LowerDir:      c.rootfs,
		UpperDir:      upper,
		WorkDir:       work,
		MergedDir:     merged,
		TargetCmd:     strings.Join(c.args[1:], " "),
		MemoryLimit:   c.memoryLimit,
		MemoryRequest: c.memoryRequest,
		CpuRequest:    c.cpuRequest,
		CPULimit:      c.cpuLimit,
		CPUPeriod:     c.cpuPeriod,
		CreatedAt:     time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	fmt.Println("Created container: ", containerID)
	return nil
}
