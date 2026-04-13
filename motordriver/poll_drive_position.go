package motordriver

/**
Functions in this file poll the status of the driver: position and safety limits.

Key fixes vs the old version:
 1. masterDevices is now []*MasterDevice, so device.PdoReady is read from the
    SAME struct that SetupPDOPosition wrote — not a stale value copy.
 2. Each goroutine gets its own stop channel so StopDriverPolling() reliably
    stops ALL goroutines, not just one random one.
 3. checkPotNotLimit guard condition fixed: OR → AND.
 4. checkPotNotLimit safety stop uses the triggering device, not masterDevices[0].
**/
import (
	channels "EtherCAT/channels"
	logger "EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	notifier "EtherCAT/motordriver/statusnotifier"
	"EtherCAT/settings"
	"sync"
	"time"
)

// stopChansMu guards stopChans against concurrent close/range races during reset.
var stopChansMu sync.Mutex

// stopChans holds one channel per polling goroutine so every goroutine can be
// stopped independently and reliably.
var stopChans []chan struct{}

// pollDrivePosition starts one position-polling goroutine per device.
// devices is []*MasterDevice so goroutines always see the live PdoReady flag.
func pollDrivePosition(devices []*MasterDevice) error {
	logger.Debug("starting driver position listener")
	stopChansMu.Lock()
	stopChans = make([]chan struct{}, 0, len(devices))
	stopChansMu.Unlock()
	for _, device := range devices {
		ch := make(chan struct{})
		stopChansMu.Lock()
		stopChans = append(stopChans, ch)
		stopChansMu.Unlock()
		go pollDrivePositionProcess(device, ch)
	}
	return nil
}

// stopDriverPolling stops ALL position-polling goroutines.
func stopDriverPolling() {
	stopChansMu.Lock()
	defer stopChansMu.Unlock()
	for _, ch := range stopChans {
		close(ch)
	}
	stopChans = nil
}

// pollDrivePositionProcess is the per-device polling goroutine.
//
// It receives a *MasterDevice pointer so it always reads the flags and offsets
// that were written by SetupPDOPosition, even if those were written after the
// goroutine was created (which cannot happen with the new init order, but the
// pointer guarantees correctness regardless).
func pollDrivePositionProcess(device *MasterDevice, stop <-chan struct{}) {
	logger.Info("polling position of driver:", device.Name)

	for {
		select {
		case <-stop:
			logger.Debug("stopping driver position listener for:", device.Name)
			return
		default:
		}

		var rawPulses int32

		// Read position: prefer PDO buffer (updated every 10 ms by cyclic task).
		// Fall back to SDO only when PDO was not successfully configured.
		if device.PdoReady && IsPDOActive() {
			rawPulses = GetLastPDOPosition()
		} else {
			operation, opErr := GetEtherCATOperation("pollstatus", device.Device.AddressConfigName)
			if opErr == nil && len(operation.Steps) > 0 {
				sdoVal, _ := DrivePosition(device.Master, device.Position, operation.Steps[0])
				rawPulses = sdoVal
			}
		}

		driverStatus := getCurrentDriverStatus(device.Name)

		// Convert raw encoder counts to degrees.
		curPos, posWithErrCorrection := currentPosition(rawPulses, device.Device.DriveXRatio, device.Name)

		// Apply pitch error, backlash and work offset for the UI display value.
		pitchError := getPitchError(device.Name, driverStatus.destinationPosition)
		displayPos := (posWithErrCorrection - pitchError) + driverStatus.backlash - driverStatus.workOffset

		// Normalise to [0, 360).
		if displayPos >= 359.999 {
			displayPos = 0
		}

		// Notify UI with the display-corrected position.
		notifier.NotifyCurrentPosition(device.Name, displayPos)

		// Update internal status keeper with the raw degree position.
		currentDriverPosition(*device, curPos)

		// Software safety limit check.
		driverSettings := settings.GetDriverSettings(device.Name)
		checkPotNotLimit(curPos, device, driverSettings, driverStatus)

		// 50 ms is plenty when PDO already updates the buffer at 10 ms.
		time.Sleep(50 * time.Millisecond)
	}
}

// checkPotNotLimit checks software POT/NOT limits and triggers an emergency stop
// if the motor has exceeded them.
//
// Fixes vs old version:
//   - Guard condition: was (NOT >= 0 || POT <= 0) which skipped checking whenever
//     either limit was zero. Corrected to (NOT >= 0 && POT <= 0) so both limits
//     must be unset before skipping.
//   - Emergency stop: was masterDevices[0] (always device 0). Now uses the
//     triggering device pointer so multi-drive systems work correctly.
func checkPotNotLimit(
	currentPos float64,
	device *MasterDevice,
	driverSettings settings.DriverSettings,
	driverStatus driverCurrentStatus,
) (exceeded bool) {

	// Both limits unset (zero) — nothing to check.
	if driverSettings.NOT >= 0 && driverSettings.POT <= 0 {
		return false
	}

	// Limit was already flagged in a previous cycle — just keep alarming.
	if driverStatus.potNotExceeded {
		if driverStatus.potExceeded {
			statusnotifier.Alarm("POT Limit Exceeded")
		} else {
			statusnotifier.Alarm("NOT Limit Exceeded")
		}
		return true
	}

	threshold := device.Device.PotNotThreshold
	pot := float64(driverSettings.POT)
	not := 360.0 + float64(driverSettings.NOT) // NOT is stored as a negative angle

	// POT check (positive / clockwise direction)
	if driverStatus.direction == 1 && pot > 0 {
		if currentPos >= (pot-threshold) && currentPos <= pot+(threshold*10) {
			logger.Error("Software POT limit exceeded at", currentPos, "limit:", pot)
			FastPowerOff(*device)
			StopJog(device)
			channels.WriteCommandExecInput("stop_prog_exec", "")
			statusnotifier.Alarm("POT Limit Exceeded")
			notifyDriverStatus("pot_not_exceeded", "POT", *device)
			return true
		}
	}

	// NOT check (negative / counter-clockwise direction)
	if driverStatus.direction == -1 && not > 0 {
		if currentPos <= (not+threshold) && currentPos >= not-(threshold*10) {
			logger.Error("Software NOT limit exceeded at", currentPos, "limit:", not)
			FastPowerOff(*device)
			StopJog(device)
			channels.WriteCommandExecInput("stop_prog_exec", "")
			statusnotifier.Alarm("NOT Limit Exceeded")
			notifyDriverStatus("pot_not_exceeded", "NOT", *device)
			return true
		}
	}

	return false
}