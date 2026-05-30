package dirs

import (
	"mycontainer/consts"
	"os"
)

func InitDirs() error {
	err := os.MkdirAll(consts.DefaultPath, 0777)
	if err != nil {
		return err
	}
	err = os.MkdirAll(consts.ContainersPath, 0777)
	if err != nil {
		return err
	}
	return nil
}
