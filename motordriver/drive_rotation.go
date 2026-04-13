package motordriver

import (
	helper "EtherCAT/helper"
	logger "EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	settings "EtherCAT/settings"
	"errors"
	"fmt"
	"time"
)

// reverseDir writes the direction-reversal polarity parameter via SDO.
// Safe only before ecrt_master_activate — called exclusively from InitMaster.
func reverseDir(masterDevice MasterDevice) error {
	if IsPDOActive() {
		logger.Warn("[PDO] reverseDir: called after activation — skipping (direction set at startup only)")
		return nil
	}
	logger.Trace("Reverse direction activated for driver", masterDevice.Name)
	operation, err := GetEtherCATOperation("reverse", masterDevice.Device.AddressConfigName)
	if err != nil {
		return err
	}
	runSDOOperation(masterDevice.Master, masterDevice.Position, operation)
	return nil
}

// nonReverseDir writes the non-reversed direction polarity parameter via SDO.
// Safe only before ecrt_master_activate — called exclusively from InitMaster.
func nonReverseDir(masterDevice MasterDevice) error {
	if IsPDOActive() {
		logger.Warn("[PDO] nonReverseDir: called after activation — skipping (direction set at startup only)")
		return nil
	}
	logger.Trace("Non reverse direction activated for driver", masterDevice.Name)
	operation, err := GetEtherCATOperation("nonreverse", masterDevice.Device.AddressConfigName)
	if err != nil {
		return err
	}
	runSDOOperation(masterDevice.Master, masterDevice.Position, operation)
	return nil
}

//ManualJog start jogging. Direction param can be 1 or -1
//1 will drive the motor in clock wise and -1 in counter clock wise
func ManualJog(masterDevice *MasterDevice, direction int) error {
	driverStatus := getCurrentDriverStatus(masterDevice.Device.Name)
	if driverStatus.potNotExceeded {
		logger.Error("pot/not exceeded, exiting from move command")
		return errors.New("pot/not exceeded, exiting from move command")
	}
	logger.Trace("manual jog, driver: ", masterDevice.Name)
	envSettings := settings.GetDriverSettings(masterDevice.Name)
	notifyDriverStatus("set_backlash", fmt.Sprintf("%f", 0.00), *masterDevice)
	// Restored old build behaviour: always power on before declamp + jog.
	// FastPowerOn in PDO mode clears pdoShutdownActive (set by a prior hasClamped)
	// so the cyclic task re-enables the drive before the brake is released.
	if err := FastPowerOn(*masterDevice); err != nil {
		logger.Error(err)
		statusnotifier.Alarm(err.Error())
		return err
	}

	_, declampErr := hasDeclamped(*masterDevice, envSettings)
	if declampErr != nil {
		logger.Error(declampErr)
		statusnotifier.Alarm(declampErr.Error())
		return declampErr
	}

	notifyDriverStatus("motor_running", "true", *masterDevice)
	
	// Calculate target velocity (RPM)
	rpm := direction * int(masterDevice.Device.RPMConst*envSettings.JogFeed)
    
	if !IsPDOActive() || !masterDevice.PdoJogReady {
		return fmt.Errorf("[JOG] PDO not active or RxPDO not ready — jog is PDO-only in this build")
	}

	// PDO path: set velocity setpoint and arm cyclic jog.
	// The CiA-402 state machine (cia402NextControlword) keeps the drive in
	// Operation Enabled; this call just sets the target velocity and mode.
	_ = masterDevice.SetJogPDOSetpoints(0x000F, 3, int32(rpm))
	masterDevice.EnableJogPDO(true)
	logger.Info("[PDO-JOG] Velocity handed to cyclic task. RPM:", rpm)
	return nil
}
//StopJog stop jogging
func StopJog(masterDevice *MasterDevice) error {
	logger.Trace("stop jog, driver: ", masterDevice.Name)

	if !IsPDOActive() || !masterDevice.PdoJogReady {
		// PDO is mandatory in this build. If not active, log and return — do not
		// attempt SDO writes which would block waiting for a mailbox response.
		logger.Warn("[PDO] StopJog: PDO not active — cannot stop jog safely for driver:", masterDevice.Name)
		return fmt.Errorf("StopJog: PDO not active — jog control is PDO-only in this build")
	}

	// Zero velocity then give the drive 250ms to decelerate to a stop
	// before reverting to CSP standby (holds position).
	_ = masterDevice.SetTargetVelocityPDO(0)
	time.Sleep(250 * time.Millisecond)
	masterDevice.desiredOpMode.Store(8) // CSP mode — standby holds actual position
	masterDevice.EnableJogPDO(false)
	logger.Info("[PDO] StopJog: velocity zeroed, jog disabled for driver:", masterDevice.Name)

	notifyDriverStatus("motor_running", "false", *masterDevice)
	envSettings := settings.GetDriverSettings(masterDevice.Name)
	_, clampErr := hasClamped(*masterDevice, envSettings)
	if clampErr != nil {
		logger.Error(clampErr)
		statusnotifier.Alarm(clampErr.Error())
		return clampErr
	}
	return nil
}

// hasTargetReached waits for CiA-402 PP completion: sw bit10=1 (Target Reached)
// AND bit12=0 (Set-Point Ack cleared), stable for stableWindow.
// Uses statusword bits — not position error — because the drive runs its own ramp.
func hasTargetReached(masterDevice *MasterDevice) error {
	// --- PDO path: Profile Position statusword check ---
	if masterDevice.PdoPosReady && IsPDOActive() {
		const (
			bitTargetReached = uint16(1 << 10) // SW bit10: motion complete, motor at target
			bitSetPointAck   = uint16(1 << 12) // SW bit12: PP handshake ack (must be 0 when done)
			bitFault         = uint16(1 << 3)  // SW bit3:  drive fault
			moveTimeout      = 30 * time.Second

			// stableWindow: bit10 must stay HIGH continuously this long.
			// Time-based so single-sample noise glitches cause no latency penalty.
			stableWindow = 10 * time.Millisecond
		)

		logger.Trace("[PDO-PP] waiting for Target Reached (bit10=1 && bit12=0). goal=",
			masterDevice.desiredTargetPosition.Load())

		// ----------------------------------------------------------
		// PHASE 1 — Wait for CiA-402 handshake completion
		// The cyclic task holds CW bit 4 HIGH until the drive replies
		// with SW bit 12 HIGH. Once acknowledged, ppSetpointPending
		// becomes false, and we can safely monitor for completion.
		// ----------------------------------------------------------
		handshakeDeadline := time.Now().Add(2 * time.Second)
		for masterDevice.ppSetpointPending.Load() {
			sw := GetLastPDOStatusword()

			if sw&bitFault != 0 {
				return fmt.Errorf("[PDO-PP Phase1] drive fault during handshake: sw=0x%04X err=0x%04X",
					sw, GetLastPDOErrorCode())
			}
			if !IsPDOActive() {
				return fmt.Errorf("[PDO-PP Phase1] aborted: PDO cyclic stopped (system reset)")
			}
			if time.Now().After(handshakeDeadline) {
				return fmt.Errorf("[PDO-PP Phase1] handshake timeout: drive did not acknowledge set-point (bit 12)")
			}
			time.Sleep(1 * time.Millisecond)
		}

		logger.Debug("[PDO-PP] drive acknowledged set-point — waiting for bit10 to clear. goal=",
			masterDevice.desiredTargetPosition.Load(), " actual=", GetLastPDOPosition())

		// PHASE 1.5 — Wait for bit10 to go LOW (confirms new move started).
		bit10ClearDeadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(bit10ClearDeadline) {
			sw := GetLastPDOStatusword()
			if sw&bitFault != 0 {
				return fmt.Errorf("[PDO-PP Phase1.5] drive fault: sw=0x%04X", sw)
			}
			if !IsPDOActive() {
				return fmt.Errorf("[PDO-PP Phase1.5] aborted: PDO cyclic stopped")
			}
			if sw&bitTargetReached == 0 {
				logger.Debug("[PDO-PP] bit10 cleared — motor moving...")
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		// ----------------------------------------------------------
		// PHASE 2 — Wait for bit10 stable for stableWindow
		//
		// Use a time-based stability gate: track the wall-clock time
		// when bit10 first went HIGH. If it stays HIGH for stableWindow
		// without interruption, the move is complete.
		//
		// A glitch (bit10 drops to 0 for 1-2ms due to noise) resets
		// stableFrom but does NOT add fixed 50ms penalty — recovery is
		// instant on the next 1ms poll when bit10 returns to 1.
		// ----------------------------------------------------------
		timeout := time.Now().Add(moveTimeout)
		var stableFrom time.Time
		stableStarted := false

		for {
			sw := GetLastPDOStatusword()

			// Abort immediately on drive fault (bit3)
			if sw&bitFault != 0 {
				return fmt.Errorf("[PDO-PP] drive fault during move: sw=0x%04X errCode=0x%04X",
					sw, GetLastPDOErrorCode())
			}

			// Exit immediately if PDO was shut down
			if !IsPDOActive() {
				return fmt.Errorf("[PDO-PP] aborted: PDO cyclic stopped (system reset)")
			}

			// Exit immediately if emergency cancelled this move.
			if masterDevice.posMoveAborted.Load() {
				masterDevice.posMoveAborted.Store(false)
				return fmt.Errorf("[PDO-PP] aborted: position move cancelled by emergency")
			}

			// CiA-402 PP completion condition: bit10=1 AND bit12=0
			if sw&bitTargetReached != 0 && sw&bitSetPointAck == 0 {
				if !stableStarted {
					stableFrom = time.Now()
					stableStarted = true
				} else if time.Since(stableFrom) >= stableWindow {
					// bit10 has been continuously HIGH for stableWindow — done.
					logger.Trace("[PDO-PP] target reached.",
						"sw=", fmt.Sprintf("0x%04X", sw),
						"goal=", masterDevice.desiredTargetPosition.Load(),
						"pos=", GetLastPDOPosition())
					return nil
				}
			} else {
				// bit10 dropped — reset the stability timer
				stableStarted = false
			}

			if time.Now().After(timeout) {
				return fmt.Errorf("[PDO-PP] timeout waiting for Target Reached: sw=0x%04X goal=%d pos=%d",
					sw, masterDevice.desiredTargetPosition.Load(), GetLastPDOPosition())
			}
			time.Sleep(1 * time.Millisecond)
		}
	}

	return fmt.Errorf("hasTargetReached: PDO not active or PdoPosReady=false")
}


//freeRotate will not wait for ECS or will not send fin signal.
func freeRotate(masterDevice *MasterDevice, valueinDegree float64) error {
	logger.Trace("freeRotate to postion, driver", masterDevice.Name, "degree:", valueinDegree)
	err := doRotate(masterDevice, valueinDegree)
	if err != nil {
		return err
	}
	doneDriverAction()
	logger.Trace("freeRotate to postion completed, driver", masterDevice.Name)

	return nil
}

//doRotate function which power on and rotate to a degree, it waits for declamping and clamping.
func doRotate(masterDevice *MasterDevice, valueinDegree float64) error {

	// If PDO cyclic not active, refuse to rotate (this build is PDO-only)
	if !(IsPDOActive() && masterDevice.PdoPosReady) {
		return fmt.Errorf("PDO position mode not ready — SDO fallback removed")
	}

	logger.Info("[PDO] Starting rotation in PP mode")

	// Restored old build behaviour: power on before declamp + move.
	// FastPowerOn in PDO mode clears pdoShutdownActive so the cyclic task
	// returns to CW=0x000F and walks the drive back to Operation Enabled —
	// required if hasClamped set pdoShutdownActive in the previous cycle.
	if err := FastPowerOn(*masterDevice); err != nil {
		logger.Error(err)
		statusnotifier.Alarm(err.Error())
		return err
	}

	// Declamp if required (safe even if no clamp logic mapped)
	envSettings := settings.GetDriverSettings(masterDevice.Name)
	if _, declampErr := hasDeclamped(*masterDevice, envSettings); declampErr != nil {
		logger.Error("[PDO-PP] doRotate: declamp failed, aborting move:", declampErr)
		statusnotifier.Alarm(declampErr.Error())
		return declampErr
	}

	notifyDriverStatus("motor_running", "true", *masterDevice)

	// Convert degree → relative pulse delta
	delta := int32(getPulsesFromDegree(masterDevice, helper.RoundFloatTo3(valueinDegree)))

	// Convert to ABSOLUTE target (0x607A requires absolute position in PP)
	actual := GetLastPDOPosition()
	goal := actual + delta

	logger.Info("[PDO-PP] delta:", delta, " actual:", actual, " goal:", goal)

	// Enable position mode in cyclic task
	masterDevice.EnablePosPDO(true)

	// Write absolute goal
	if err := masterDevice.SetTargetPositionPDO(goal); err != nil {
		masterDevice.EnablePosPDO(false)
		return err
	}

	// Wait for target reached (bit10=1 && bit12=0)
	if err := hasTargetReached(masterDevice); err != nil {
		masterDevice.EnablePosPDO(false)
		return err
	}

	// Disable PP mode after completion
	masterDevice.EnablePosPDO(false)

	notifyDriverStatus("motor_running", "false", *masterDevice)

	// Clamp if required
	_, _ = hasClamped(*masterDevice, envSettings)

	logger.Info("[PDO-PP] rotation completed successfully")

	return nil
}