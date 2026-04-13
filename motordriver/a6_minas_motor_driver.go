package motordriver

/**
Specialised implementation for the Panasonic MINAS A6 motor driver.

Key fixes vs the old version:
 1. ioStatusListener reads digital inputs from the PDO buffer (GetLastPDODigitalInputs)
    when PdoDIReady=true, instead of issuing an SDO read after ecrt_master_activate.
    Mixing SDO reads with a running PDO cyclic task can corrupt EtherCAT frames.
 2. isIOOn performs a bounds check on binpos before indexing the string,
    preventing a runtime panic when IntToBinary returns fewer than 8 characters.
 3. stopIOStatChan is now a slice of per-goroutine channels so stopPollIOStat()
    reliably stops ALL listener goroutines, not just one.
 4. hasTargetReached: removed unreachable `return nil` after the infinite loop.
**/

import (
	ethercatDevice "EtherCAT/ethercatdevicedatatypes"
	logger "EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	"errors"
	"sync"
	"time"
)

// A6Minas is the specialised implementation for the Panasonic MINAS A6 driver.
type A6Minas struct{}

// hasTargetReached polls the drive statusword (via SDO) until the target-reached
// bit is set.  This is A6-specific because the CiA-402 Target Reached bit (bit 10)
// and the Set-Point Acknowledge bit (bit 12) require a handshake sequence.
func (a6 A6Minas) hasTargetReached(masterDevice MasterDevice, action int, immediate int, operation ethercatDevice.Operation) error {
	logger.Trace("A6Minas waiting for target reached")
	firstStep := operation.Steps[0]
	secondStep := operation.Steps[1]

	SDODownload(masterDevice.Master, masterDevice.Position, firstStep)

	for {
		result, _ := SDOUpload2(masterDevice.Master, masterDevice.Position, secondStep)
		sw := uint16(result)
		// bit 10 = Target Reached, bit 12 = Set-Point Acknowledge
		if (sw>>10)&1 == 1 && (sw>>12)&1 == 0 {
			logger.Trace("A6Minas target reached")
			return nil
		}
		if (sw>>10)&1 == 1 && (sw>>12)&1 == 1 {
			// Acknowledge the new set-point
			firstStep.Value = "0x004f"
			SDODownload(masterDevice.Master, masterDevice.Position, firstStep)
		}
	}
	// Note: the loop above only exits via return; this line is never reached
	// but is required to satisfy the Go compiler for the named return type.
}

// potNotEnabled checks whether the POT or NOT input signal is active.
// Not used in the current flow but retained for interface compatibility.
func (a6 A6Minas) potNotEnabled(masterDevice MasterDevice) (bool, error) {
	inputStatus, err := readInputSignal(masterDevice)
	if err != nil {
		return false, err
	}
	// bit 0 = POT, bit 1 = NOT (active-high in this check)
	return (inputStatus & 0x03) != 0, nil
}

// readDeclampSignal polls until the declamp (DCL) input signal is asserted (bit 7).
func (a6 A6Minas) readDeclampSignal(masterDevice MasterDevice, declampTiming int) (bool, error) {
	logger.Debug("waiting for declamp status")
	start := time.Now()
	for {
		inputStatus, err := readInputSignal(masterDevice)
		if err != nil {
			break
		}
		if (inputStatus & (1 << 7)) != 0 { // bit 7 = DCL high
			return true, nil
		}
		if time.Since(start).Milliseconds() > int64(declampTiming) {
			break
		}
		time.Sleep(time.Microsecond)
	}
	statusnotifier.DriverError(65379)
	return false, errors.New("declamp failed")
}

// readClampSignal polls until the clamp (CL) input signal is asserted (bit 6).
func (a6 A6Minas) readClampSignal(masterDevice MasterDevice, clampTiming int) (bool, error) {
	logger.Debug("waiting for clamp status", clampTiming, "ms")
	start := time.Now()
	for {
		inputStatus, err := readInputSignal(masterDevice)
		if err != nil {
			break
		}
		if (inputStatus & (1 << 6)) != 0 { // bit 6 = CL high
			return true, nil
		}
		if time.Since(start).Milliseconds() > int64(clampTiming) {
			break
		}
		time.Sleep(time.Microsecond)
	}
	statusnotifier.DriverError(65378)
	return false, errors.New("clamp failed")
}

// receivedECS polls for the ECS (External Command Signal) input to go high.
// Returns:
//
//	1 — ECS received
//	2 — stop signal received on stopECSChan
func (a6 A6Minas) receivedECS(masterDevice MasterDevice, operation ethercatDevice.Operation, stopECSChan chan bool) int {
	for {
		select {
		case <-stopECSChan:
			logger.Debug("stopping ECS polling")
			return 2
		default:
			inputStatus, err := readECSSignal(masterDevice)
			if err != nil {
				time.Sleep(10 * time.Microsecond)
				continue
			}
			if (inputStatus & 0x01) != 0 { // bit 0 HIGH = ECS active
				return 1
			}
			time.Sleep(10 * time.Microsecond)
		}
	}
}

// receivedECSZero polls for the ECS input to return to zero (de-asserted).
// Returns:
//
//	1 — ECS de-asserted
//	2 — stop signal received on stopECSChan
func (a6 A6Minas) receivedECSZero(masterDevice MasterDevice, operation ethercatDevice.Operation, stopECSChan chan bool) int {
	for {
		select {
		case <-stopECSChan:
			logger.Debug("stopping ECS zero polling")
			return 2
		default:
			inputStatus, err := readECSSignal(masterDevice)
			if err != nil {
				time.Sleep(10 * time.Microsecond)
				continue
			}
			if (inputStatus & 0x01) == 0 { // bit 0 LOW = ECS de-asserted
				return 1
			}
			time.Sleep(10 * time.Microsecond)
		}
	}
}

func (a6 A6Minas) sendFinishSignal(masterDevice MasterDevice, operation ethercatDevice.Operation) error {
	return nil
}

// ioStopChansMu protects ioStopChans against concurrent access between
// pollIOStat (writer) and stopPollIOStat (reader+writer) which are called
// from different goroutines during reset.
var ioStopChansMu sync.Mutex

// ioStopChans holds one stop channel per ioStatusListener goroutine.
// Using a slice of per-goroutine channels ensures stopPollIOStat() stops
// ALL goroutines, not just one random receiver on a shared channel.
var ioStopChans []chan struct{}

// pollIOStat starts one I/O status listener goroutine per device.
func (a6 A6Minas) pollIOStat(availableDevices []*MasterDevice) {
	logger.Debug("starting A6Minas I/O status listener")
	ioStopChansMu.Lock()
	ioStopChans = make([]chan struct{}, 0, len(availableDevices))
	ioStopChansMu.Unlock()
	for _, d := range availableDevices {
		ch := make(chan struct{})
		ioStopChansMu.Lock()
		ioStopChans = append(ioStopChans, ch)
		ioStopChansMu.Unlock()
		go a6.ioStatusListener(d, ch)
	}
}

// stopPollIOStat stops ALL I/O status listener goroutines.
func (a6 A6Minas) stopPollIOStat() {
	ioStopChansMu.Lock()
	defer ioStopChansMu.Unlock()
	for _, ch := range ioStopChans {
		close(ch)
	}
	ioStopChans = nil
}

// GetCurrentAlarm returns the last alarm string sent to the UI.
func GetCurrentAlarm() string {
	return statusnotifier.GetCurrentAlarm()
}

// ioStatusListener reads digital inputs from PDO buffer and publishes I/O status.
// Startup guard: waits for sw&0x006F==0x0027 (Op Enabled) before enabling NOT/POT
// checks — prevents false alarms from zero-initialized DI register (all bits=0,
// both active-low limits appear triggered before first real PDO frame arrives).
func (a6 A6Minas) ioStatusListener(masterDev *MasterDevice, stop <-chan struct{}) {
	pdoStabilised := false
	if masterDev.PdoDIReady {
		stabiliseDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(stabiliseDeadline) {
			sw := GetLastPDOStatusword()
			if sw&0x006F == 0x0027 { // Operation Enabled
				pdoStabilised = true
				logger.Info("[IO] PDO stabilised, drive in Operation Enabled — enabling NOT/POT protection for:", masterDev.Name)
				break
			}
			select {
			case <-stop:
				return
			default:
				time.Sleep(20 * time.Millisecond)
			}
		}
		if !pdoStabilised {
			logger.Warn("[IO] PDO did not reach Operation Enabled within 2s — NOT/POT protection disabled for this session:", masterDev.Name)
		}
	} else {
		pdoStabilised = true // SDO path has no startup race
	}

	lastPOT := false
	lastNOT := false

	for {
		select {
		case <-stop:
			logger.Debug("stopping A6Minas I/O status listener for:", masterDev.Name)
			return
		default:
		}

		var inputStatus int
		var err error

		// Prefer PDO input signal register (0x4F25) when available — no EtherCAT I/O cost.
		if masterDev.PdoDIReady {
			inputStatus = int(GetLastPDODigitalInputs())
		} else {
			inputStatus, err = readInputSignal(*masterDev)
			if err != nil {
				logger.Error("error reading I/O input signals:", err)
				time.Sleep(time.Millisecond)
				continue
			}
		}

		var ioStat statusnotifier.IOStatus

		// Parse I/O bits directly — eliminates IntToBinary string allocation per poll cycle.
		di := inputStatus
		ioStat.ECS   = (di & (1 << 0)) != 0  // bit 0 HIGH = ECS active
		ioStat.POT   = (di & (1 << 1)) == 0  // bit 1 LOW  = POT active (active-low)
		ioStat.NOT   = (di & (1 << 2)) == 0  // bit 2 LOW  = NOT active (active-low)
		ioStat.HOME  = (di & (1 << 3)) == 0  // bit 3 LOW  = HOME active (active-low)
		ioStat.ALMIN = (di & (1 << 4)) == 0  // bit 4 LOW  = ALMIN active (active-low)
		ioStat.CL    = (di & (1 << 6)) != 0  // bit 6 HIGH = Clamp
		ioStat.DCL   = (di & (1 << 7)) != 0  // bit 7 HIGH = Declamp

		// ── Bit 5: Hardware RESET input ────────────────────────────────────
		if pdoStabilised && (di&(1<<5)) != 0 {
			logger.Info("[IO] Hardware reset input detected (bit 5 high) — triggering system reset for:", masterDev.Name)
			go performSysReset(true)
		}

		// Driver state-dependent virtual I/O
		driverState := getCurrentDriverStatus(masterDev.Name)
		ioStat.FIN   = driverState.isSendingFinSignal
		ioStat.SOLOP = driverState.isDriverOnOff

		statusnotifier.NotifyIOStatus(ioStat)

		// ================================================================
		// POT/NOT HARDWARE EMERGENCY STOP
		// Only active after pdoStabilised=true (drive reached Operation
		// Enabled). This prevents false alarms from the zero-initialized
		// DI register before the drive enters OP state.
		// ================================================================
		if masterDev.Device.StopWhenHWPOTNOT && pdoStabilised {
			if ioStat.NOT && !lastNOT {
				statusnotifier.Alarm("NOT Limit Exceeded")
				logger.Error("hardware NOT activated")
				FastPowerOff(*masterDev)
				StopJog(masterDev)
			}
			if ioStat.POT && !lastPOT {
				statusnotifier.Alarm("POT Limit Exceeded")
				logger.Error("hardware POT activated")
				FastPowerOff(*masterDev)
				StopJog(masterDev)
			}
		}
		lastNOT = ioStat.NOT
		lastPOT = ioStat.POT

		interval := 1000 // default 1 ms
		if masterDev.Device.IOPollingInterval > 0 {
			interval = masterDev.Device.IOPollingInterval
		}
		time.Sleep(time.Duration(interval) * time.Microsecond)
	}
}