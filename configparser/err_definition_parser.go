package configparser

import (
	"EtherCAT/helper"
	"EtherCAT/logger"
	"bufio"
	"os"
	"strings"
)

var parsedErrFile bool
var errKeyValue map[string]string

//GetErrorString returns the error based on the error configured in error_definition.txt
func GetErrorString(id string) string {
	if parsedErrFile == false {
		err := parseErrFile()
		if err != nil {
			logger.Error(err)
		}
	}
	if _, ok := errKeyValue[id]; !ok {
		return "Unknown error code " + id
	}
	return errKeyValue[id]
}

func parseErrFile() error {
	errKeyValue = make(map[string]string)
	file, err := os.Open(helper.AppendWDPath("/configs/error_definition.txt"))
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	var txtlines []string

	for scanner.Scan() {
		txtlines = append(txtlines, scanner.Text())
		line := scanner.Text()
		if strings.HasPrefix(line, "/") {
			continue
		}
		splitted := strings.Split(line, ":")
		errKeyValue[strings.TrimSpace(splitted[0])] = strings.TrimSpace(splitted[1])
	}
	parsedErrFile = true
	return nil
}
