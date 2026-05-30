package commander

import (
	"fmt"
	"mycontainer/store"
	"os"
	"strings"
	"text/tabwriter"
)

func (c *commander) list() error {
	containers, err := store.GetAllContainers()
	if err != nil {
		return fmt.Errorf("failed to fetch containers from DB: %w", err)
	}
	if c.listAll {
		// Вертикальный формат для детального просмотра (аналог \G в СУБД)
		for i, container := range containers {
			if i > 0 {
				fmt.Println(strings.Repeat("-", 60)) // Разделитель между контейнерами
			}
			fmt.Printf("CONTAINER ID : %s\n", container.ID)
			fmt.Printf("PID          : %d\n", container.PID)
			fmt.Printf("STATUS       : %s\n", container.Status)
			fmt.Printf("COMMAND      : %s\n", container.TargetCmd)
			fmt.Printf("LOWER DIR    : %s\n", container.LowerDir)
			fmt.Printf("UPPER DIR    : %s\n", container.UpperDir)
			fmt.Printf("WORK DIR     : %s\n", container.WorkDir)
			fmt.Printf("MERGED DIR   : %s\n", container.MergedDir)
			fmt.Printf("MEMORY LIMIT : %d bytes\n", container.MemoryLimit)
			fmt.Printf("MEMORY REQUEST : %d bytes\n", container.MemoryRequest)
			fmt.Printf("CPU REQUEST    : %d\n", container.CpuRequest)
			fmt.Printf("CPU LIMIT    : %d\n", container.CPULimit)
			fmt.Printf("CPU PERIOD   : %d\n", container.CPUPeriod)
			fmt.Printf("CREATED AT   : %s\n", container.CreatedAt)
		}
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

		// Стандартный компактный формат (твой исходный код с микро-багфиксом)
		fmt.Fprintln(w, "CONTAINER ID\tPID\tSTATUS\tCOMMAND\tCREATED")

		for _, container := range containers {
			cmd := container.TargetCmd
			if len(cmd) > 25 {
				cmd = cmd[:22] + "..."
			}

			fmt.Fprintf(w, "%s\t%d\t%s\t%q\t%s\n",
				container.ID,
				container.PID,
				container.Status,
				cmd,
				container.CreatedAt,
			)
		}
		return w.Flush()
	}
	return nil
}
