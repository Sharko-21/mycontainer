package commander

import (
	"fmt"
	"log/slog"
	"mycontainer/logging"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	containerID   string

	listAll bool

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
	case listCommand:
		return c.list()
	case deleteCommand:
		return c.delete()
	case createCommand:
		return c.create()
	case startCommand:
		return c.start()
	case childCommand:
		return c.child()
	default:
		return fmt.Errorf("unknown command: %s", c.args[0])
	}
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
	if len(c.args) <= 0 {
		return fmt.Errorf("no command specified")
	}
	if c.args[0] == deleteCommand {
		if len(c.args) < 2 {
			return fmt.Errorf("no container_id specified")
		}
		c.containerID = c.args[1]
	}
	if c.args[0] == startCommand {
		if len(c.args) < 2 {
			return fmt.Errorf("no container_id specified")
		}
		c.containerID = c.args[1]
	}
	if c.args[0] == listCommand {
		if len(c.args) >= 2 && c.args[1] == "--all" {
			c.listAll = true
		}
	}

	if c.args[0] == childCommand {
		if len(c.args) < 2 {
			return fmt.Errorf("no container_id specified")
		}
		c.containerID = c.args[1]
	}

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
	c.logging.Debugf("args: %v", c.args)
	c.logging.Debugf("memoryLimit: %d", c.memoryLimit)
	c.logging.Debugf("memoryRequest: %d", c.memoryRequest)
	c.logging.Debugf("cpuRequest: %d", c.cpuRequest)
	c.logging.Debugf("cpuLimit: %d", c.cpuLimit)
	c.logging.Debugf("cpuPeriod: %d", c.cpuPeriod)
	return nil
}

func (c *commander) getCgroupPath(containerID string) string {
	return filepath.Join("/sys/fs/cgroup/system.slice", "mycontainer-"+containerID)
}

// commands for execution
const startCommand = "start"
const createCommand = "create"
const listCommand = "list"
const childCommand = "child"
const deleteCommand = "delete"

// execution params
const rootfsParam = "rootfs"
const memoryLimitParam = "memory-limit"
const memoryRequestParam = "memory-request"
const cpuLimitParam = "cpu-limit"
const cpuRequestParam = "cpu-request"
const debugParam = "debug"

// default limits
const memoryLimitDefault = 536870912 // 512MB
const memoryRequestDefault = 0
const cpuLimitDefault = -1 // max
const cpuRequestDefault = 100
const cpuPeriodDefault = 100000 // 100ms
