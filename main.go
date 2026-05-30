package main

import (
	"log"
	"mycontainer/commander"
	"mycontainer/dirs"
	"mycontainer/logging"
)

func main() {
	logger := logging.NewLogger()
	if err := dirs.InitDirs(); err != nil {
		log.Fatal(err)
	}
	cmd, err := commander.NewCommander(logger)
	if err != nil {
		log.Fatal(err)
	}
	if err = cmd.Parse(); err != nil {
		log.Fatal(err)
	}
}
