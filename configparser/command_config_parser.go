package configparser

import (
	dt "EtherCAT/datatypes"
	"EtherCAT/helper"
	logger "EtherCAT/logger"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

//ParseExececutionConfigYML parse execution yml file
func ParseExececutionConfigYML() (dt.YamlConfig, error) {
	var yamlConfig dt.YamlConfig
	yamlFile, err := ioutil.ReadFile(helper.AppendWDPath("/configs/execution.yml"))
	if err != nil {
		logger.Error("Error reading execution config yaml file: %s", err)
		return yamlConfig, err
	}

	err = yaml.Unmarshal(yamlFile, &yamlConfig)
	return yamlConfig, err
}
