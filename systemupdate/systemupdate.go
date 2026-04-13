package systemupdate

import (
	"EtherCAT/channels"
	"EtherCAT/helper"
	"EtherCAT/logger"
	"EtherCAT/motordriver"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/osext"
)

var isUpdateInProgress bool

func isAnUpdateInProgress() bool {
	return isUpdateInProgress
}

//PerformSystemUpdate perform system update
func PerformSystemUpdate(sendUINotification bool) {
	if isUpdateInProgress {
		logger.Info("An update already in progress, ignoring the request")
		return
	}
	logger.Info("Start system update...")
	isUpdateInProgress = true
	downloadedUpdateFileName := readFileNameOfUpdate()
	//if the update file name is empty then no updates exist
	if downloadedUpdateFileName == "" {
		logger.Info("No updates found, ignoring system update request")
		return
	}
	logger.Info("Updating system to ", downloadedUpdateFileName)

	updateBuildPath := helper.AppendWDPath("/updates/release/" + strings.Replace(downloadedUpdateFileName, ".tar.gz", "", 1))

	tarPath := helper.AppendWDPath("/updates/" + downloadedUpdateFileName)
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		logger.Info("Update artifact file not found in", tarPath, ". Ignoring system update request")
		return
	}

	if motordriver.HasDriverConnected() {
		motordriver.StopSystem()
	}

	logger.Info("Backing up current system before system update")
	backupPath, backupErr := backupExistingSystem()
	if backupErr != nil {
		logger.Error("Unable to backup the existing system. Terminating system update")
		logger.Error(backupErr)
		return
	}

	logger.Info("Backed up current system to", backupPath)
	_, err := applyUpdate(updateBuildPath, tarPath)
	if err != nil {
		logger.Error("Unable to perform system update. Rolling back to previous state")
		logger.Error(err)
		rollbackErr := rollbackUpdate(backupPath)
		if rollbackErr != nil {
			logger.Error("Unable to rollback to previous state. Backup of system before update is taken to ", backupPath)
			logger.Error(rollbackErr)
		}
	}
	os.Remove(tarPath)
	os.Remove(helper.AppendWDPath("/updates/current_update"))
	os.RemoveAll(helper.AppendWDPath("/updates/release/"))
	logger.Info("system update completed")
	for i := 0; i < 3; i++ {
		if sendUINotification {
			channels.SendAlarm("System update completed. Restart the handheld device now.")
		}
		time.Sleep(time.Duration(10) * time.Microsecond)
	}
	// err = syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
	// if err != nil {
	// 	logger.Error("error when rebooting system")
	// 	logger.Error(err)
	// }
	isUpdateInProgress = false
	os.Exit(2)
}

//readFileNameOfUpdate if there is any update the CheckSystemUpdate will write the new filename to ./updates/current_update file
//this function will read the file name
func readFileNameOfUpdate() string {
	file, err := os.Open(helper.AppendWDPath("/updates/current_update"))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		currLine := scanner.Text()
		if strings.HasPrefix(currLine, "Name") {
			splitted := strings.Split(currLine, ":")
			return strings.TrimSpace(splitted[1])
		}
	}
	return ""
}

//ApplyUpdate apply the update to the existing program
func applyUpdate(updateBuildPath, tarPath string) (string, error) {

	backupPath := ""
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return backupPath, err
	}
	err = Untar(tarFile, helper.AppendWDPath("/updates"))
	if err != nil {
		return backupPath, err
	}

	execPath, _ := osext.Executable()
	updateDir := filepath.Dir(updateBuildPath)

	currentExecFile := filepath.Base(execPath)

	newExecFile := filepath.Join(updateDir, fmt.Sprintf(".%s.new", currentExecFile))
	fp, err := os.Create(newExecFile)
	if err != nil {
		return backupPath, err
	}
	defer fp.Close()
	os.Chmod(newExecFile, 0777)

	//read the updated program from updates directory
	newPgm, err := os.Open(updateBuildPath + "/" + currentExecFile)
	if err != nil {
		return backupPath, err
	}
	//read the new program and return as io.reader
	newBytes, err := ioutil.ReadAll(newPgm)
	if err != nil {
		return backupPath, err
	}
	//copy the new program to the temp file created
	_, err = io.Copy(fp, bytes.NewReader(newBytes))
	if err != nil {
		return backupPath, err
	}
	fp.Close()
	//remove the current running program
	os.Remove(execPath)
	//rename the temp app file to the actual program file name
	renErr := os.Rename(newExecFile, execPath)
	if renErr != nil {
		return backupPath, renErr
	}

	os.Chmod(execPath, 0777)
	cpErr := copyOtherFilesAndDirs(updateBuildPath)

	return backupPath, cpErr
}

func copyOtherFilesAndDirs(updatePath string) error {
	combinedPath := fmt.Sprintf("%s/commands", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyDir(combinedPath, helper.AppendWDPath("/commands"))
		if cpErr != nil {
			return cpErr
		}
	}

	combinedPath = fmt.Sprintf("%s/configs", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyDir(combinedPath, helper.AppendWDPath("/configs"))
		if cpErr != nil {
			return cpErr
		}
	}
	combinedPath = fmt.Sprintf("%s/ethercatinterface.h", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/ethercatinterface.h"))
		if cpErr != nil {
			return cpErr
		}
	}

	combinedPath = fmt.Sprintf("%s/libethercatinterface.so", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/libethercatinterface.so"))
		if cpErr != nil {
			return cpErr
		}
	}

	combinedPath = fmt.Sprintf("%s/scripts/start.sh", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/scripts/start.sh"))
		if cpErr != nil {
			return cpErr
		}
		os.Chmod(helper.AppendWDPath("/scripts/start.sh"), 0777)
	}
	combinedPath = fmt.Sprintf("%s/scripts/hotspot.sh", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/scripts/hotspot.sh"))
		if cpErr != nil {
			return cpErr
		}
		os.Chmod(helper.AppendWDPath("/scripts/hotspot.sh"), 0777)
	}

	combinedPath = fmt.Sprintf("%s/scripts/wifi.sh", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/scripts/wifi.sh"))
		if cpErr != nil {
			return cpErr
		}
		os.Chmod(helper.AppendWDPath("/scripts/wifi.sh"), 0777)
	}

	combinedPath = fmt.Sprintf("%s/scripts/tunnel.sh", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyFile(combinedPath, helper.AppendWDPath("/scripts/tunnel.sh"))
		if cpErr != nil {
			return cpErr
		}
		os.Chmod(helper.AppendWDPath("/scripts/tunnel.sh"), 0777)
	}

	combinedPath = fmt.Sprintf("%s/www_v2", updatePath)
	if _, err := os.Stat(combinedPath); !os.IsNotExist(err) {
		cpErr := helper.CopyDir(combinedPath, helper.AppendWDPath("/www_v2"))
		if cpErr != nil {
			return cpErr
		}
	}
	return nil
}

func backupExistingSystem() (backupPath string, err error) {
	currentTime := time.Now()
	backupPathTmp := helper.AppendWDPath("/jamun_backup")
	errTmp := os.RemoveAll(backupPathTmp)
	if errTmp != nil {
		logger.Error("failed removing previous backup files")
		logger.Error(errTmp)
	}
	backupPath = fmt.Sprintf("%s/%s", backupPathTmp, currentTime.Format("2006-01-02-15-04-05"))
	err = helper.CopyDir(helper.AppendWDPath("/"), backupPath)
	return
}

func rollbackUpdate(backupPath string) (err error) {
	err = helper.CopyDir(backupPath, helper.AppendWDPath("/"))
	return
}

func read(mpath string) (*os.File, error) {
	f, err := os.OpenFile(mpath, os.O_RDONLY, 0444)
	if err != nil {
		return f, err
	}
	return f, nil
}
