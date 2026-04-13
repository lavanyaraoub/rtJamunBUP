package motordriver

import (
	logger "EtherCAT/logger"
	settings "EtherCAT/settings"
	"errors"
)

//This file contains the helper functions for different dirver operations

//hasDeclamped verify whether declamping happened
func hasDeclamped(masterDevice MasterDevice, envSettings settings.DriverSettings) (bool, error) {
	driver := GetMotorDriver()
	// envSettings := settings.GetDriverSettings(masterDevice.Name)
	if envSettings.ClampDeclamp == 1 {
		breakOff(masterDevice)
		logger.Debug("Clamp declamp enabled, checking the status")
		isDeclamped, clampErr := driver.readDeclampSignal(masterDevice, envSettings.ClampDeclampTiming)
		return isDeclamped, clampErr
	}
	return true, nil
}

// hasClamped verify whether clamping happened
// return true if clamped successfully, false with error if not clamped
func hasClamped(masterDevice MasterDevice, envSettings settings.DriverSettings) (bool, error) {
	driver := GetMotorDriver()
	if envSettings.ClampDeclamp == 1 {
		breakOn(masterDevice)

		// Restored old build behaviour: always call FastPowerOff after brake engage.
		// FastPowerOff is now PDO-safe — when PDO is active it sets pdoShutdownActive=true
		// so the cyclic task sends CW=0x0006 (Shutdown), walking the drive from
		// Operation Enabled → Ready To Switch On through the PDO channel without any
		// SDO/PDO race on 0x6040. The physical clamp solenoid requires the drive to be
		// de-energised before the CL input signal (bit 6) goes high.
		if err := FastPowerOff(masterDevice); err != nil {
			return false, err
		}

		logger.Debug("Clamp declamp enabled, checking the status")
		isDeclamped, clampErr := driver.readClampSignal(masterDevice, envSettings.ClampDeclampTiming)
		if clampErr != nil {
			return false, clampErr
		}
		if !isDeclamped {
			return false, errors.New("Clamping error")
		}
		// clamping is enabled and clamp activated, return true
		return true, nil
	}
	// clamping not enabled, return false.
	return false, nil
}

//applyClampIfSettingsChanged when a setting changed and it enables clamp/declamp then apply clamping if motor is
//not running. If motor is running dont do any action.
func applyClampIfSettingsChanged() {
	for _, dev := range masterDevices {
		devSetting := settings.GetDriverSettings(dev.Name)
		stat := getCurrentDriverStatus(dev.Name)
		if stat.isMotorRunning {
			return
		}

		if devSetting.ClampDeclamp == 1 {
			logger.Debug("CL/DL enabled power off driver", dev.Name)
			FastPowerOff(*dev)
		} else {
			logger.Debug("CL/DL disabled power on driver", dev.Name)
			FastPowerOn(*dev)
		}
	}

}