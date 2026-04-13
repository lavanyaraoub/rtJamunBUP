package hotspot

import (
	"EtherCAT/helper"
	"EtherCAT/logger"
	"errors"
	"os"
	"os/exec"
)

func ConfigureWifi(ssid, pwd, country string) error {
	err := os.Chmod(helper.AppendWDPath("/scripts/wifi.sh"), 0777)
	if err != nil {
		return err
	}

	c := exec.Command("sudo", helper.AppendWDPath("/scripts/wifi.sh"), country, ssid, pwd)
	logger.Trace("exec wifi.sh")

	if err := c.Run(); err != nil {
		return errors.New("Error: " + err.Error())
	} else {
		return nil
	}
}
