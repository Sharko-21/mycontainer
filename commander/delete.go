package commander

import (
	"fmt"
	container2 "mycontainer/container"
	"mycontainer/store"
)

func (c *commander) delete() error {
	container, err := store.GetContainerByID(c.containerID)
	if err != nil {
		return err
	}
	if container.Status == container2.RunningStatus {
		fmt.Println("Cannot delete a running container. Stop it first")
		return nil
	}
	err = store.DeleteContainer(c.containerID)
	if err != nil {
		return err
	}
	return container2.RemoveContainerDirs(c.containerID)
}
