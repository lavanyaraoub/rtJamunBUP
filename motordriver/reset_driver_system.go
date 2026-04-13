package motordriver

import (
	channels "EtherCAT/channels"
	"EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	"fmt"
	//"time"
)
/*
resetSystemWorker is the single goroutine that handles all system resets.
It reads from channels.ResetDriverSystem and executes the full reset sequence.

RESET SEQUENCE — order is critical for IgH EtherCAT correctness:

  PHASE 1 — Stop motion and listeners (PDO cyclic still running)
    a. stopECSCheck             — unblock any goroutine waiting in ECS loop
    b. stopDriverPolling        — stop 50ms position-poll goroutines
    c. stopPollIOStat           — stop digital-input poll goroutines
    d. stopErrorPolling         — stop error-code poller (no-op when PDO active)
    e. stopDriverActionListener — signal action listener goroutine to exit
    f. stopDriveStatusListener  — signal status keeper goroutine to exit

  PHASE 2 — Guard: abort reset if PDO is not active (no safe SDO fallback)
    If IsPDOActive() == false →log error, re-start listeners, continue.

  PHASE 3 — Fault reset via PDO (cyclic MUST still be running)
    g. PDOFaultResetdis able jog/pos modes, clear drive fault via CiA-402
                       state machine. Cyclic is alive to process controlwords.
    h. Sleep 500ms  allow state machine to settle to Operation Enabled.

  PHASE 4 Restart listeners
    i. pollDrivePosition / pollIOStat / initDriverActionListener /
       startDriverStatusListener

  PDO IS MANDATORY THERE IS NO SDO FALLBACK AT RUNTIME:
    IgH EtherCAT has no ecrt_master_deactivate(). After ecrt_master_activate()
    the master owns the bus. SDO mailbox responses require ecrt_master_receive()
    to be running in the cyclic task. Stopping the cyclic before SDO calls
    causes those calls to block forever. If PDO failed to start during InitMaster,
    the process must be restarted no safe SDO recovery path exists at runtime.
*/

func listenSystemReset() {
	channels.ResetDriverSystem = make(chan bool)
	go resetSystemWorker()
}

func performSysReset(checkMotorRunning bool) {
	if checkMotorRunning {
		for _, dev := range masterDevices {
			stat := getCurrentDriverStatus(dev.Name)
			if stat.isMotorRunning {
				logger.Info("Unable to do system reset, motor", dev.Name, "is running")
				statusnotifier.Alarm("Unable to do system reset, motor " + dev.Name + " is running")
				statusnotifier.SocketMessage("reset_done", "reset completed")
				return
			}
		}
	}
	channels.ResetDriverSystem <- true
}

func resetSystemWorker() {
	for {
		msg := <-channels.ResetDriverSystem
		if msg == true {
			logger.Info("resetting driver system....")
			
			// 1. Stop high-level listeners (aborts any active command sequences)
			stopECSCheck() // MUST be first: unblocks any goroutine waiting in doECSCheck / doECSCheckZero
			stopDriverPolling()
			stopDriverActionListener()
			stopDriveStatusListener()
			stopErrorPolling()
			stopPollIOStat()
			doneDriverAction()
			
			channels.WriteCommandExecInput("reset", "")
			channels.NotifyCmdComplete()

			// 2. CLEAR FAULTS AND HALT MOTION SAFELY
			//
			// PDO is the ONLY safe reset path at runtime. IgH EtherCAT has no
			// ecrt_master_deactivate(). SDO mailbox responses require ecrt_master_receive()
			// to be running in the cyclic task. Stopping the cyclic before SDO calls
			// causes those calls to block forever. If PDO failed to start, the process
			// must be restarted — no safe SDO fallback exists at runtime.
			if !IsPDOActive() {
				logger.Error("[RESET] FATAL: PDO cyclic is not active — cannot perform safe reset. Restart required.")
				statusnotifier.Alarm("Reset failed: EtherCAT PDO not active — restart required")
				statusnotifier.SocketMessage("reset_done", "reset failed")
				// Re-start listeners so the system stays responsive for a restart command.
				pollDrivePosition(masterDevices)
				pollIOStat(masterDevices)
				initDriverActionListener()
				startDriverStatusListener()
				continue // keep goroutine alive to handle future reset requests
			}

			logger.Info("[PDO] Performing soft reset via continuous PDO state machine...")
			// PDOFaultReset mirrors the YAML faultReset sequence:
			//   1. Halt motion (disable jog/pos, zero velocity)
			//   2. Issue fault reset (CW=0x80 via cia402 state machine)
			//   3. Wait up to 2s for fault bit to clear
			//   4. Release brake (0x60FE:01/02 = 0, 500ms delay)
			//   5. Wait for Operation Enabled state
			//   6. Clear cached error code (lastPDOErr = 0)
			// Returns false if the hardware emergency is still physically pressed
			// (drive cannot exit fault while emergency input is active).
			resetOK := PDOFaultReset(masterDevices)

			// 3. RESTART LISTENERS
			// Always restart position, IO, action and status listeners — the UI
			// needs position updates and IO state regardless of fault status.
			pollDrivePosition(masterDevices)
			pollIOStat(masterDevices)
			initDriverActionListener()
			startDriverStatusListener()

			// Only restart the error poller when the reset succeeded (drive is healthy).
			// If resetOK=false the hardware emergency is still pressed — bit3=1 in
			// the statusword so lastPDOErr still holds the emergency error code.
			// Starting the poller now would immediately re-fire alarm 87 even though
			// we already sent the correct "release button" message to the UI.
			// The poller will be started on the next successful reset.
			if resetOK {
				pollDriveError(masterDevices)
			}

			logger.Info("resetting driver completed....")

			if !resetOK {
				logger.Warn("[RESET] Drive still faulted after reset — hardware emergency button may still be pressed. sw=",
					fmt.Sprintf("0x%04X", GetLastPDOStatusword()))
				statusnotifier.Alarm("Hardware Emergency pressed — release button and reset again")
				statusnotifier.SocketMessage("reset_done", "reset failed")
			} else {
				statusnotifier.Alarm("No Alarms")
				statusnotifier.SocketMessage("reset_done", "reset completed")
			}
		}
	}
}
// StopSystem stops all listeners and powers off drives cleanly.
func StopSystem() {
	if !HasDriverConnected() {
		return
	}
	stopECSCheck()
	stopDriverPolling()
	stopDriverActionListener()
	stopDriveStatusListener()
	stopErrorPolling()
	channels.WriteCommandExecInput("reset", "")
	channels.NotifyCmdComplete()
	// ShutdownMasters implements the 3-phase PDO shutdown:
	// arm pdoShutdownActive → poll PDS safe → StopPDOCyclic + ecrt_release_master.
	// This is the only correct way to stop without triggering Err88.2.
	ShutdownMasters()
}