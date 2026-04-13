package motordriver

import (
	helper "EtherCAT/helper"
	"EtherCAT/logger"
	settings "EtherCAT/settings"
	"fmt"
	"math"
)

/*
Functions related to driver angle manipulation. Converting degrees to pulses, get absolute degrees
find degrees based on the current pulse of the driver, etc
*/

//getPulsesFromDegree returns the value to send to driver based on the degree passed by the client
func getPulsesFromDegree(masterDevice *MasterDevice, degree float64) int64 {

	// driverSettings := settings.GetDriverSettings(masterDevice.Name)
	pulse := float64(masterDevice.Device.DriveXRatio) * degree
	// pulse = helper.RoundFloatTo3(pulse + float64(driverSettings.BackLash*float64((settings.BacklashRequired(int(pulse))*masterDevice.Device.DriveXRatio))))

	//this rounding is commented as it cause wrong value if the value is big and thus the rotation is always wrong
	// pulse = helper.RoundFloatTo3(pulse)
	pulseForDisp := fmt.Sprintf("%f", pulse)
	logger.Debug("degree: ", degree, " pulse: ", pulseForDisp, " integer: ", int32(pulse), "drive x ratio:", masterDevice.Device.DriveXRatio)
	//tcpserver.js line# 1307
	// if pulse < 0 {
	// 	pulse = pulse * -1
	// }
	return int64(pulse)
}

/*
currentPosition returns the angle in degree based on the postion value received from driver.
 driveXRatio: is a config value configured along with the master device in configs/device-configuration.yml
 pos: position received from driver, this will be in pulses
*/
func currentPosition(pos int32, driveXRatio int, driveName string) (float64, float64) {
	driverSettings := settings.GetDriverSettings(driveName)
	// angleWithPitchError := float32(0.00)

	driveOffset := driverSettings.HomingOffset * float32(driveXRatio)
	drivePosition := float64(pos) - float64(driveOffset)
	drivePosition = drivePosition / float64(driveXRatio)
	drivePosition = math.Mod(float64(drivePosition), 360)
	if drivePosition < 0 {
		drivePosition = drivePosition + 360
	}
	if drivePosition >= 359.999 {
		drivePosition = 0
	}
	drivePositionWithErrorCorrection := (drivePosition) //+ (float64(driverSettings.FactorBacklash) * driverSettings.BackLash)
	return helper.RoundFloat(drivePosition, 3), helper.RoundFloat(drivePositionWithErrorCorrection, 3)
}

//TODO add function to find pitch error like below, this is the value sending to ui
// drivePosition = drivePosition + float32((settings.FactorBacklash() * driverSettings.BackLash)) - angleWithPitchError

/*
getAbsolutePosition move the motor to absolute degree

returns
	move to position for driver
	destination position to communicate to clients to display the destination postion

definition

target position is 30 then motor will rotate to 30degree
irrespective where the current position is
based on getDestinationAngle() in machine_parser.js line# 420
*/
func getAbsolutePosition(currentPos float64, targetPos float64, useShortestPath bool) (float64, float64) {
	return helper.GetAbsolutePosition(currentPos, targetPos, useShortestPath)
}

// /*
// getAbsolutePath get the angle to rotate in absolute path

// for e.g. if current position is 50 and target position is 10 then rotate all the way
// to 360 and go to 10
// */
// func getAbsolutePath(currentPos float64, targetPos float64) float64 {
// 	toMove := math.Mod(currentPos, 360)
// 	toMove = toMove - targetPos
// 	toMove = 360 - toMove
// 	if math.Abs(currentPos) > 360 {
// 		toMove = math.Mod(toMove, 360)
// 	}

// 	if targetPos < 0 {
// 		toMove = toMove - 360
// 		if toMove <= -360 {
// 			toMove = toMove + 360
// 		}
// 	}

// 	return toMove
// }

// /*
// getAbsolutePositionWithShortestPath move to absolute postion but uses shortest path

// for eg. if current position is 50 and target position is 40 then will -10 and move to 40
// */
// func getAbsolutePositionWithShortestPath(currentPos float64, targetPos float64) float64 {
// 	currentPos = math.Mod(currentPos, 360)
// 	modeDiff := math.Mod((targetPos - currentPos), 360)
// 	shortestDistance := 180 - math.Abs(math.Abs(modeDiff)-180)

// 	test := math.Mod((modeDiff + 360), 360)
// 	if test < 180 {
// 		return shortestDistance * 1
// 	}
// 	return shortestDistance * -1
// }

/*
getRelativePosition returns the relative position based on the current position

for e.g. if the current position is 10 degree and ordered 20 degree then
motor will move to 30degree (10+20=30)
based on getDestinationAngle() in machine_parser.js line# 420
*/
func getRelativePosition(currentPos float64, targetPos float64, prevDestinationAngle float64) (float64, float64) {
	// If previous destination is too far from current, use current position instead.
	if math.Abs(currentPos-prevDestinationAngle) > 1.0 { // 1° threshold, tune as needed
		prevDestinationAngle = currentPos
	}
	return helper.GetRelativePosition(currentPos, targetPos, prevDestinationAngle)
}

/*
getPitchError returns the configured pitch error

It’s position calibration done to compensate mechanical error.
The mechanical systems shall be manufactured and assembled to the closest precision possible
before entering the pitch error compensation. Pitch error is a position error measured at equal
intervals of rotary table. In our case we shall keep interval for every 10 degrees. That is 0 degrees
till 360 Degrees shall have a total of 36 intervals. The pitch error on the rotary table shall be
measured on the final output shaft / Rotary table top. The error at different intervals is measured
by external fine measuring equipment.

Whenever the controller is moving or commanded to any position. The commanded value will be
added or subtracted with error defined in the interval chart. The user display of position data will
always show the original position command, all the error data shall be acting only in the backend
calculation. When the controller is being operated the user display shall not physically show
the compensated error.
*/
func getPitchError(driveName string, targetPos float64) float64 {
	index := int(math.Abs(targetPos / 10))
	index = index - 1
	if index < 0 {
		return 0
	}
	if index > 35 {
		return 0
	}
	driverSettings := settings.GetDriverSettings(driveName)
	return float64(driverSettings.PitchError[index])
}

// func reversePitchError(currentPos float64) float64 {
// 	index := int(math.Abs(currentPos / 10))
// 	index = index - 1
// 	if index < 0 {
// 		return 0
// 	}
// 	// if index > 35 {
// 	// 	return 0
// 	// }
// 	driverSettings := settings.GetDriverSettings()
// 	return float64(driverSettings.PitchError[index] * -1)
// }
