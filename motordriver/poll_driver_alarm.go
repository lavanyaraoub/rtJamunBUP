package motordriver

/**
Functions in this file poll the error status of the driver
**/
import (
	logger "EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	"errors"
	"sync/atomic"
	"time"
)

var stopErrPollingChan chan bool
// errPollingRunning must be atomic: written by resetSystemWorker goroutine,
// read by stopErrorPolling from the action-listener goroutine concurrently.
var errPollingRunning atomic.Bool

//pollDriveError polls the errors from the driver
func pollDriveError(avilableDevices []*MasterDevice) error {
	logger.Debug("starting driver error listener")
	if len(avilableDevices) <= 0 {
		return errors.New("no driver found")
	}
	// Buffered(1) so stopErrorPolling() never blocks even if the worker has
	// already exited or was never started (e.g. when PDO is active and
	// pollDriveError is skipped by initListeners).
	stopErrPollingChan = make(chan bool, 1)
	errPollingRunning.Store(true)
	for _, device := range avilableDevices {
		go pollDriveErrWorker(device)
	}
	return nil
}

// stopErrorPolling signals the error-polling goroutine to stop.
// Safe to call even when pollDriveError was never started (PDO mode).
func stopErrorPolling() {
	if !errPollingRunning.Load() {
		return // no goroutine running — nothing to stop, nothing to block on
	}
	errPollingRunning.Store(false)
	stopErrPollingChan <- true
}

// pollDriveErrWorker polls the drive error code (0x603F) and notifies the
// status subsystem.
//
// PDO path (preferred): when PDO is active, 0x603F is read by the cyclic task
// every 10ms and cached in lastPDOErr. We read from that atomic buffer here —
// no SDO, no mailbox, no risk of blocking.
//
// SDO path: used when PDO is not active (e.g. pre-activation diagnostics).
// initListeners already skips calling pollDriveError() when PDO is active,
// so this worker should only ever reach the SDO branch before activation.
func pollDriveErrWorker(device *MasterDevice) {
	logger.Info("polling error of driver: ", device.Name)

	// Only fetch the SDO operation descriptor if we might need it.
	// If PDO is active this is never used.
	var sdoStep interface{} // placeholder — only loaded when PDO is not active
	_ = sdoStep

	// Load SDO operation only when PDO is not active (avoids wasted config parse).
	var usePDO bool
	if IsPDOActive() && device.PdoErrorReady {
		usePDO = true
		logger.Info("[PDO] pollDriveErrWorker: reading 0x603F from PDO buffer for driver:", device.Name)
	} else {
		operation, _ := GetEtherCATOperation("readError", device.Device.AddressConfigName)
		if len(operation.Steps) <= 0 {
			logger.Warn("pollDriveErrWorker: no readError steps configured for", device.Name)
			return
		}
		step := operation.Steps[0]

		// SDO polling loop (pre-activation path).
		for {
			select {
			default:
				if IsPDOActive() {
					// PDO became active mid-poll — switch to PDO path by breaking
					// out of this loop and falling through to the PDO loop below.
					usePDO = true
					goto pdoLoop
				}
				errCode, _ := SDOUpload2(device.Master, device.Position, step)
				statusnotifier.DriverError(errCode)
				time.Sleep(100 * time.Millisecond)
			case <-stopErrPollingChan:
				logger.Debug("stopping driver error listener (SDO path)")
				return
			}
		}
	}

pdoLoop:
	if !usePDO {
		return
	}

	// lastReportedErrCode gives rising-edge detection: only call DriverError
	// when the error code CHANGES. This is a second layer of defence —
	// the primary fix is in pdo_cyclic_task.go (lastPDOErr only written when
	// bit3=1). Together they prevent the alarm storm seen in logs where alarm 87
	// kept firing every 100ms long after the reset completed.
	var lastReportedErrCode int

	// PDO polling loop: read from atomic updated by cyclic task.
	for {
		select {
		default:
			sw := GetLastPDOStatusword()
			if sw&0x0008 != 0 {
				errCode := int(GetLastPDOErrorCode())
				// Only fire on new/changed error codes — not on every poll tick.
				if errCode != 0 && errCode != lastReportedErrCode {
					lastReportedErrCode = errCode
					statusnotifier.DriverError(errCode)
				}
			} else {
				// Fault cleared — reset tracker so same code fires again if fault recurs.
				lastReportedErrCode = 0
			}
			time.Sleep(100 * time.Millisecond)
		case <-stopErrPollingChan:
			logger.Debug("stopping driver error listener (PDO path)")
			return
		}
	}
}