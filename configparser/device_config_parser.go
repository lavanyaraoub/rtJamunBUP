package configparser

import (
	dt "EtherCAT/ethercatdevicedatatypes"
	"EtherCAT/helper"
	logger "EtherCAT/logger"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// ParseDeviceConfig parse device-configuration.yml file
// https://stackoverflow.com/a/39832919
func ParseDeviceConfig() (dt.Devices, error) {
	var devices dt.Devices
	yamlFile, err := ioutil.ReadFile(helper.AppendWDPath("/configs/device-configuration.yml"))
	if err != nil {
		logger.Error("Error reading device-configuration yaml file: %s", err)
		return devices, err
	}

	err = yaml.Unmarshal(yamlFile, &devices)
	return devices, err
}
