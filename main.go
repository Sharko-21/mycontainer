package main

import (
	"log"
	"mycontainer/commander"
	"mycontainer/logging"
)

func main() {
	logger := logging.NewLogger()
	cmd, err := commander.NewCommander(logger)
	if err != nil {
		log.Fatal(err)
	}
	if err = cmd.Parse(); err != nil {
		log.Fatal(err)
	}
}
