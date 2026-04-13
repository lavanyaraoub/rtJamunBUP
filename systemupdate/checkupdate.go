package systemupdate

import (
	"EtherCAT/channels"
	"EtherCAT/constants"
	"EtherCAT/helper"
	"EtherCAT/logger"
	"EtherCAT/settings"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
)

var checkForUpdateInProgress bool

//CheckforUpdates will check the server to see any updates available. If available then download the update
func CheckforUpdates(sendUINotification bool) {
	if checkForUpdateInProgress {
		logger.Info("Checking for update in progress, ignoring the request")
		return
	}
	if isAnUpdateInProgress() {
		logger.Info("System update already in progress, ignoring the request")
		return
	}
	logger.Info("Checking for system update...")
	checkForUpdateInProgress = true
	err := fetchRelease(sendUINotification)
	if err != nil {
		logger.Error("Error downloading release")
		logger.Error(err)
	}
	checkForUpdateInProgress = false
}

//fetch the release from release server
func fetchRelease(sendUINotification bool) error {
	_ = os.Mkdir(helper.AppendWDPath("/updates"), 0777)

	envSetting := settings.GetEnvSettings()
	apiURL := constants.ReleaseServiceURL + fmt.Sprintf("/v1.0/get/release/%s/%s?currentversion=%s", constants.ProjectShortID, envSetting.ReleaseChannel, constants.SystemVersion)
	resp, err := http.Get(apiURL)
	if err != nil {
		logger.Info("Unable to check for updates. Verify whether the system is connected to internet")
		if sendUINotification {
			channels.SendAlarm("Unable to check for updates. Verify whether the system is connected to internet")
		}
		return err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-type")
	if contentType == "application/octet-stream" {
		_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
		if err != nil {
			return err
		}
		filename := params["filename"]
		out, err := os.Create(helper.AppendWDPath("/updates/" + filename))
		if err != nil {
			return err
		}
		defer out.Close()

		// Write the body to file
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return err
		}
		//write the latest release file name to a file for updater to use
		updateMetaFileName, err := os.Create(helper.AppendWDPath("/updates/current_update"))
		if err != nil {
			logger.Error(err)
		}
		defer updateMetaFileName.Close()
		updateMetaFileName.WriteString("Name:" + filename)

		logger.Debug("Successfully dowloaded the release file. File name: " + filename)
		if sendUINotification {
			channels.SendAlarm("An update is available and dowloaded. Click Update button to update the system. Stop all the work before updating the system")
		}
	} else {
		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		logger.Debug(string(content))
		logger.Info("System is upto date. No updates available at the moment")
		if sendUINotification {
			channels.SendAlarm("System is upto date. No updates available at the moment")
		}
	}
	return nil
}
