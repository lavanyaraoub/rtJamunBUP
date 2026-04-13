package motordriver

/*
#include "ethercatinterface.h"
#include <errno.h>
*/
import "C"

import (
	"fmt"
	logger "EtherCAT/logger"
	"time"
)

// ResetDriver clears a CiA-402 fault. PDO path delegates to PDOFaultReset;
// SDO path used pre-activation only.
func ResetDriver(avilableDevices []*MasterDevice) error {
	if IsPDOActive() {
		// Delegate to PDOFaultReset which handles the full fault-reset sequence
		// via the running cyclic task (write 0x80, wait for bit3=0, re-enable).
		logger.Info("[PDO] ResetDriver: delegating to PDOFaultReset")
		PDOFaultReset(avilableDevices)
		return nil
	}

	// SDO path: pre-activation commissioning only.
	for _, device := range avilableDevices {
		operation, err := GetEtherCATOperation("faultReset", device.Device.AddressConfigName)
		if err != nil {
			return err
		}
		logger.Info("[SDO] ResetDriver: fault reset via SDO for drive:", device.Name)
		runSDOOperation(device.Master, device.Position, operation)
	}
	return nil
}

// resetMultiTurn resets the multi-turn encoder.
// PDO path uses async ec_sdo_request_t (IgH services it each cycle, no blocking).
// SDO path used pre-activation only. Power cycle required after reset.
func resetMultiTurn(availableDevices []*MasterDevice) error {
	if IsPDOActive() {
		return triggerMultiTurnResetAsync(availableDevices)
	}
	return triggerMultiTurnResetSDO(availableDevices)
}

// triggerMultiTurnResetAsync executes the multiturn reset via async SDO while PDO runs.
// Arms 3 steps (0x4D01:00=0x0031, 0x4D00:01 rising then falling edge) with EBUSY retry.
// pdoMTResetActive disables servo during reset (drive requires servo-off to execute).
func triggerMultiTurnResetAsync(availableDevices []*MasterDevice) error {
	const (
		pollInterval   = 2 * time.Millisecond   // between state polls
		armRetryDelay  = 5 * time.Millisecond   // wait for CoE mailbox to release between steps
		armRetryMax    = 10                      // max retries on -EBUSY before giving up
		stepTimeout    = 600 * time.Millisecond  // per-step ACK timeout
		disableTimeout = 2000 * time.Millisecond  // time for drive to reach Switched On
		enableTimeout  = 500 * time.Millisecond  // time for drive to re-reach Op Enabled
	)

	// armStep arms one SDO request, retrying on -EBUSY (CoE mailbox releasing).
	// This fixes the root cause of failure 2: after step 1 ACK, the mailbox
	// may still be in "releasing" state for 1-2 cycles. A flat -EBUSY return
	// previously caused silent abort of the sequence.
	armStep := func(step int, value uint32, desc string) error {
		for i := 0; i < armRetryMax; i++ {
			rc := int(C.trigger_mt_request_step(C.int(step), C.uint32_t(value)))
			if rc == 0 {
				return nil
			}
			if rc == -16 { // EBUSY = 16 on Linux
				logger.Info(fmt.Sprintf("[MT-ASYNC] %s: mailbox busy, retry %d/%d", desc, i+1, armRetryMax))
				time.Sleep(armRetryDelay)
				continue
			}
			// Any other error (EINVAL = NULL request, etc.) is fatal
			err := fmt.Errorf("[MT-ASYNC] %s: arm failed rc=%d (not retryable)", desc, rc)
			logger.Error(err.Error())
			return err
		}
		err := fmt.Errorf("[MT-ASYNC] %s: mailbox still busy after %d retries", desc, armRetryMax)
		logger.Error(err.Error())
		return err
	}

	// waitForStep polls until the drive ACKs (SUCCESS) or errors/times-out.
	waitForStep := func(step int, desc string) error {
		deadline := time.Now().Add(stepTimeout)
		for time.Now().Before(deadline) {
			state := int(C.get_mt_request_state(C.int(step)))
			if state == 1 {
				logger.Info("[MT-ASYNC]", desc, "— ACK received")
				return nil
			}
			if state == -1 {
				err := fmt.Errorf("[MT-ASYNC] %s: drive returned SDO ERROR (object may not support CoE mailbox write)", desc)
				logger.Error(err.Error())
				return err
			}
			time.Sleep(pollInterval)
		}
		err := fmt.Errorf("[MT-ASYNC] %s: timeout after %v", desc, stepTimeout)
		logger.Error(err.Error())
		return err
	}

	// CiA-402 state helpers
	// isServoOff: drive has left Operation Enabled and servo power is removed.
	// We accept Ready To Switch On (0x0021) OR Switched On (0x0023) — both have
	// servo de-energized. The Panasonic A6 special function requires SRV-OFF.
	// CW=0x0006 (Shutdown) transitions: Op Enabled → Ready To Switch On (0x0021).
	isServoOff := func() bool {
		sw := GetLastPDOStatusword() & 0x006F
		return sw == 0x0021 || sw == 0x0023 // Ready To Switch On OR Switched On
	}
	isOpEnabled := func() bool { return (GetLastPDOStatusword()&0x006F) == 0x0027 }

	for _, d := range availableDevices {
		if d == nil {
			continue
		}
		if !d.MTSdoReady {
			err := fmt.Errorf("[MT-ASYNC] MTSdoReady=false for %s — create_mt_sdo_requests() failed at setup", d.Name)
			logger.Error(err.Error())
			return err
		}

		logger.Info("[MT-ASYNC] Starting multiturn reset for drive:", d.Name,
			"sw=", fmt.Sprintf("0x%04X", GetLastPDOStatusword()))

		// ── Phase 1: Stop motion, disable servo ────────────────────────────
		// The Panasonic A6 accepts 0x4D00/0x4D01 SDO writes in ANY CiA-402
		// state and returns SUCCESS — but only EXECUTES the special function
		// when servo power is removed (Switched On state, CW=0x0007).
		// The cyclic standby branch respects pdoMTResetActive and writes 0x0007.
		d.EnableJogPDO(false)
		d.EnablePosPDO(false)
		d.desiredTargetVelocity.Store(0)
		_ = d.SetTargetPositionPDO(GetLastPDOPosition())
		d.pdoMTResetActive.Store(true)
		logger.Info("[MT-ASYNC] Servo disable requested (CW→0x0006 Shutdown). Waiting for servo-off (ReadyToSwitchOn/SwitchedOn)...")

		disableDeadline := time.Now().Add(disableTimeout)
		for !isServoOff() {
			if time.Now().After(disableDeadline) {
				d.pdoMTResetActive.Store(false)
				err := fmt.Errorf("[MT-ASYNC] timeout waiting for servo-off state (Ready To Switch On / Switched On) sw=0x%04X — aborting", GetLastPDOStatusword())
				logger.Error(err.Error())
				return err
			}
			time.Sleep(pollInterval)
		}
		logger.Info("[MT-ASYNC] Servo is OFF sw=", fmt.Sprintf("0x%04X", GetLastPDOStatusword()),
			"— executing special function sequence")

		// ── Phase 2: Execute 3-step SDO sequence ───────────────────────────
		// Step 1: select special function "multi-turn data reset"
		if err := armStep(0, 0x0031, "0x4D01:00=0x0031 (func select)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}
		if err := waitForStep(0, "0x4D01:00=0x0031 (func select)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}
		time.Sleep(armRetryDelay) // let mailbox fully release before next step

		// Step 2: rising edge — execute the reset
		if err := armStep(1, 0x00000200, "0x4D00:01=0x200 (trigger)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}
		if err := waitForStep(1, "0x4D00:01=0x200 (trigger)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}
		time.Sleep(armRetryDelay) // let mailbox fully release before clear

		// Step 3: falling edge — clear trigger
		if err := armStep(1, 0x00000000, "0x4D00:01=0x000 (clear)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}
		if err := waitForStep(1, "0x4D00:01=0x000 (clear)"); err != nil {
			d.pdoMTResetActive.Store(false)
			return err
		}

		// ── Phase 3: Re-enable servo ────────────────────────────────────────
		d.pdoMTResetActive.Store(false)
		logger.Info("[MT-ASYNC] Servo re-enabling (CW→0x000F). Waiting for Op Enabled...")

		enableDeadline := time.Now().Add(enableTimeout)
		for !isOpEnabled() {
			if time.Now().After(enableDeadline) {
				logger.Warn("[MT-ASYNC] timeout waiting for Op Enabled after reset — CiA-402 will recover on its own")
				break
			}
			time.Sleep(pollInterval)
		}

		logger.Info("[MT-ASYNC] Multiturn reset complete for drive:", d.Name,
			"sw=", fmt.Sprintf("0x%04X", GetLastPDOStatusword()),
			"pos=", GetLastPDOPosition())
	}
	return nil
}

// triggerMultiTurnResetSDO executes the multiturn reset via blocking SDO.
// ONLY valid before ecrt_master_activate() (pre-activation commissioning).
func triggerMultiTurnResetSDO(availableDevices []*MasterDevice) error {
	for _, device := range availableDevices {
		if device == nil {
			continue
		}
		operation, err := GetEtherCATOperation("resetMultiTurn", device.Device.AddressConfigName)
		if err != nil {
			return fmt.Errorf("triggerMultiTurnResetSDO: no config for device %s: %w", device.Name, err)
		}
		logger.Info("[SDO] triggerMultiTurnResetSDO: executing for drive:", device.Name)
		for _, step := range operation.Steps {
			if step.Action == "read" {
				val, _ := SDOUpload2(device.Master, device.Position, step)
				logger.Debug("[SDO] resetMultiTurn read:", step.Name, "val:", val)
			} else {
				if err := SDODownload(device.Master, device.Position, step); err != nil {
					logger.Warn("[SDO] resetMultiTurn write failed:", step.Name, err)
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		logger.Info("[SDO] triggerMultiTurnResetSDO complete for:", device.Name)
	}
	return nil
}