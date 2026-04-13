package configparser

import (
	dt "EtherCAT/ethercatdevicedatatypes"
	"EtherCAT/helper"
	logger "EtherCAT/logger"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// ParseEthercatAddressConfig parse ethercat-device-addressing.yml file
// https://stackoverflow.com/a/39832919
func ParseEthercatAddressConfig(fileName string) (dt.Ethercat, error) {
	var ethercatOperation dt.Ethercat
	yamlFile, err := ioutil.ReadFile(helper.AppendWDPath(fileName))
	if err != nil {
		logger.Error("Error reading ethercat-device-addressing.yml file: %s", err)
		return ethercatOperation, err
	}

	err = yaml.Unmarshal(yamlFile, &ethercatOperation)
	return ethercatOperation, err
}
