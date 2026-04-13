package motordriver

import (
	channels "EtherCAT/channels"
	logger "EtherCAT/logger"
	notifier "EtherCAT/motordriver/statusnotifier"
	"EtherCAT/settings"
	"fmt"
	"math"
	//"time"
)

// moveToZero moves the motor to the absolute zero reference position using
// the shortest path.
//
// FIX (Bug 15): The old version called configureDriver(device) immediately
// before freeRotate.  configureDriver sends SDO initialisation sequences
// (operating mode, gains, limits …) to the drive.  Calling it during live
// PDO-cyclic operation is unsafe: the IgH EtherCAT master does not serialise
// SDO requests against the cyclic send/receive, so the two can collide and
// produce frame errors or drive faults.
//
// configureDriver belongs in the one-time startup sequence (InitMaster) where
// the master has not yet been activated.  It must NOT be called from any
// motion function.  It has been removed here; the drive is already correctly
// configured at boot time.
func moveToZero(device *MasterDevice) error {
	logger.Debug("move to zero started")

	driverStatus := getCurrentDriverStatus(device.Device.Name)
	envSettings := settings.GetDriverSettings(device.Name)

	const targetPosition = 0.0
	notifyDriverStatus("set_backlash", fmt.Sprintf("%f", 0.0), *device)
	notifier.NotifyDestinationPosition(device.Name, float32(targetPosition))
	notifyDriverStatus("destination_position", fmt.Sprintf("%f", targetPosition), *device)

	// --- RS232 PROCEDURE: choose direction using HomeDirection ---
	// getPos returns "degrees to move" in that direction (CW positive, CCW negative in your code)
	var degToMove float64
	if envSettings.HomeDirection == 1 {
		degToMove = getPos(driverStatus.currentPosition, 0, true)
	} else {
		degToMove = getPos(driverStatus.currentPosition, 0, false)
	}
	logger.Debug("zero ref degToMove (RS232 style):", degToMove)

	// No configureDriver() here (your comment is correct: unsafe after activate)

	// --- PDO ONLY ---
	if !(IsPDOActive() && device.PdoPosReady) {
		return fmt.Errorf("PDO position not ready (PdoPosReady=false). Zero reference is PDO-only in this build")
	}

	_, declampErr := hasDeclamped(*device, envSettings)
	if declampErr != nil {
		logger.Error(declampErr)
		return declampErr
	}
	notifyDriverStatus("motor_running", "true", *device)

	// Force direction by converting "degToMove" into a relative pulse target:
	// targetPulse = currentPulse + (degToMove * DriveXRatio)
	currentPulse := GetLastPDOPosition()
	deltaPulse := int32(float64(device.Device.DriveXRatio) * degToMove)
	targetPulse := currentPulse + deltaPulse

	// Sync ramp start
	device.currentTargetPosition.Store(currentPulse)
	_ = device.SetTargetPositionPDO(targetPulse)
	device.EnablePosPDO(true)

	logger.Info("[PDO-ZERO] CSP ramp to zero via relative pulse target. current:", currentPulse, "target:", targetPulse)

	if err := hasTargetReached(device); err != nil {
		device.EnablePosPDO(false)
		notifyDriverStatus("motor_running", "false", *device)
		return err
	}

	device.EnablePosPDO(false)
	notifyDriverStatus("motor_running", "false", *device)

	_, clampErr := hasClamped(*device, envSettings)
	if clampErr != nil {
		logger.Error(clampErr)
		return clampErr
	}

	channels.DestinationReached()
	notifier.SocketMessage("gotozero_done", "goto zero completed")
	sendECSFinSignal(*device)
	return nil
}


// getPos returns the angular distance to travel from currentPos to targetPos.
// If clockwise is true, the result is the clockwise arc; otherwise counter-clockwise.
func getPos(currentPos float64, targetPos float64, clockwise bool) float64 {
	currentPos = math.Mod(currentPos, 360)
	modeDiff := math.Mod((currentPos - targetPos), 360)

	if clockwise {
		return math.Mod((360 - modeDiff), 360)
	}
	return math.Mod((modeDiff * -1), 360)
}