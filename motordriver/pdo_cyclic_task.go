package motordriver

/*
#cgo CFLAGS: -g -Wall -I/opt/etherlab/include -I/home/pi/gosrc/src/EtherCAT
#cgo LDFLAGS: -L/home/pi/gosrc/src/EtherCAT -L/opt/etherlab/lib/ -lethercatinterface -lethercat
#include "ecrt.h"
#include "ethercatinterface.h"
*/
import "C"

import (
	"EtherCAT/logger"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CiA-402 state machine for Panasonic MINAS A6.
// SW & 0x6F: 0x40=SwitchOnDisabled 0x21=ReadyToSwitchOn 0x23=SwitchedOn 0x27=OpEnabled 0x08=Fault
// CW: 0x80=FaultReset 0x06=Shutdown(→0x21) 0x07=SwitchOn(→0x23) 0x0F=EnableOp(→0x27)
// Controlword is written every 1ms cycle to keep the drive in its current state.
func cia402NextControlword(status uint16, opEnabled *bool, faultReset bool) uint16 {
	*opEnabled = false

	// 1. Fault Handling (Bit 3)
	// If the fault bit is set, we only command a Fault Reset (Bit 7)
	// if a reset was actively requested (creating the 0->1 rising edge).
	if (status & 0x0008) != 0 {
		if faultReset {
			return 0x0080
		}
		return 0x0000 
	}

	// Mask for state bits (lower byte 0x6F)
	state := status & 0x006F

	switch state {
	case 0x0000, 0x0040:
		// State: Not Ready to Switch On or Switch On Disabled
		// Action: Command Shutdown (0x06) to move to "Ready to Switch On"
		return 0x0006

	case 0x0021:
		// State: Ready to Switch On
		// Action: Command "Switch On" (0x07)
		return 0x0007

	case 0x0023:
		// State: Switched On
		// Action: Command "Enable Operation" (0x0F)
		return 0x000F

	case 0x0027:
		// State: Operation Enabled
		// Action: Maintain Operation Enabled
		*opEnabled = true
		return 0x000F

	default:
		// Unknown state: revert to Shutdown to restart the sequence
		return 0x0006
	}
}




// ============================================================
// Shared state (accessed by cyclic goroutine and callers)
// ============================================================
// pdoCyclicMu serialises Start/Stop so they can be called across resets.
// Unlike sync.Once, a mutex lets us restart the cyclic task after a reset.
var pdoCyclicMu sync.Mutex

// pdoFaultResetMu prevents two goroutines (e.g. emergency + reset worker)
// from running PDOFaultReset simultaneously.
// Without this, both goroutines set pdoFaultResetActive, both sleep for
// up to 2.5s, and the first defer clears the flag while the second is still
// mid-sequence — causing the cyclic to stop sending 0x80 fault-reset
// controlwords prematurely and leaving the drive in Fault state.
var pdoFaultResetMu sync.Mutex

var (
	// pdoStopCh is recreated on every start so the goroutine receives a fresh signal.
	pdoStopCh chan struct{}
	// pdoCyclicDone is closed by the goroutine when it finishes its graceful PDS
	// shutdown and actually returns. StopPDOCyclic blocks on this so the caller
	// (StopSystem, ShutdownMasters, process exit) never proceeds while the drive
	// is still in Operation Enabled state.
	pdoCyclicDone chan struct{}
	pdoActive     atomic.Bool

	lastPDOPos    int32         // 0x6064 position actual value
	lastPDOStatus atomic.Uint32 // 0x6041 statusword
	lastPDOErr    atomic.Uint32 // 0x603F error code (uint16 in low bits)
	lastPDOVel    int32         // 0x606C velocity actual (if mapped)
	lastPDODI     atomic.Uint32 // 0x4F25 input signal register (vendor, replaces 0x60FD)

	lastDebugLog atomic.Int64
)

func GetLastPDOPosition() int32       { return atomic.LoadInt32(&lastPDOPos) }
func GetLastPDOStatusword() uint16    { return uint16(lastPDOStatus.Load() & 0xFFFF) }
func GetLastPDOErrorCode() uint16     { return uint16(lastPDOErr.Load() & 0xFFFF) }
func GetLastPDOVelocityActual() int32 { return atomic.LoadInt32(&lastPDOVel) }
func GetLastPDODigitalInputs() uint32 { return lastPDODI.Load() }
func IsPDOActive() bool               { return pdoActive.Load() }

// PDOFaultReset clears a CiA-402 drive fault via PDO atomics.
// Disables motion, waits up to 2s for bit3 to clear, releases brake, waits for Op Enabled.
// Returns true if fault cleared, false if hardware emergency button is still pressed.
func PDOFaultReset(devices []*MasterDevice) bool {
	if len(devices) == 0 || !pdoActive.Load() {
		return false
	}

	// Prevent two goroutines (e.g. emergency + resetSystemWorker) from running
	// the fault-reset sequence concurrently. The second caller returns false
	// immediately — the first call will clear the fault, so the second is a no-op.
	if !pdoFaultResetMu.TryLock() {
		logger.Warn("[PDO-RESET] concurrent PDOFaultReset call detected — skipping (already in progress)")
		return false
	}
	defer pdoFaultResetMu.Unlock()

	d := devices[0]

	// Step 1: Stop all motion, hold current position.
	d.EnableJogPDO(false)
	d.EnablePosPDO(false)
	d.desiredTargetVelocity.Store(0)
	_ = d.SetTargetPositionPDO(GetLastPDOPosition())
	// ADD THIS: Trigger the RISING EDGE for Fault Reset
	d.pdoFaultResetActive.Store(true)
	defer d.pdoFaultResetActive.Store(false) // Ensures it goes low again when the function exits

	// Step 2: Give the cyclic task a few cycles to send the 0x80 fault reset
	// controlword (cia402NextControlword emits 0x80 automatically when bit3 is set).
	time.Sleep(50 * time.Millisecond)

	// Step 3: Wait for Fault bit (bit3) to clear - up to 2 seconds.
	// If the hardware emergency button is still physically pressed the drive
	// cannot exit fault state. We detect this and return false to the caller.
	faultCleared := false
	timeout := time.Now().Add(2 * time.Second)
	for time.Now().Before(timeout) {
		if GetLastPDOStatusword()&0x0008 == 0 {
			faultCleared = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Step 4: Release brake - mirrors YAML steps 2-4 (0x60FE:01/02 = 0).
	d.desiredDigOutMask.Store(0)
	d.desiredDigOutVal.Store(0)
	time.Sleep(500 * time.Millisecond) // mirrors 500ms delay in YAML

	// Write again (mirrors YAML "set brake 2 again" step).
	d.desiredDigOutMask.Store(0)
	d.desiredDigOutVal.Store(0)

	if !faultCleared {
		logger.Warn("[PDO-RESET] fault reset timed out — drive still in fault state. sw=",
			fmt.Sprintf("0x%04X", GetLastPDOStatusword()),
			"— hardware emergency may still be pressed")
		return false
	}

	// Step 5: Wait for state machine to reach Operation Enabled.
	// After fault reset: Fault -> Switch On Disabled -> Ready To Switch On
	//                          -> Switched On -> Operation Enabled.
	timeout2 := time.Now().Add(1 * time.Second)
	for time.Now().Before(timeout2) {
		if GetLastPDOStatusword()&0x006F == 0x0027 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Step 6: Clear the cached error code.
	// 0x603F is hardware-latching on the Panasonic A6 — it never auto-clears.
	// The cyclic task only writes lastPDOErr when bit3=1, so this is a
	// belt-and-suspenders clear to ensure the poller sees a clean state.
	lastPDOErr.Store(0)

	logger.Info("[PDO-RESET] fault reset complete. sw=", fmt.Sprintf("0x%04X", GetLastPDOStatusword()))
	return true
}

// StopPDOCyclic signals the cyclic goroutine to stop and BLOCKS until it has
// confirmed PDS = "Ready To Switch On", then calls ecrt_release_master to walk
// the ESM gracefully back to INIT. This is the complete shutdown sequence that
// prevents Err88.2.
//
// Callers MUST set pdoShutdownActive=true on all devices and poll statusword
// until sw&0x6F==0x0021 BEFORE calling this function (done by ShutdownMasters).
// The goroutine stop case is a safety net — the primary PDS walk happens via
// the still-running ticker before this function is called.
func StopPDOCyclic() {
	pdoCyclicMu.Lock()
	if !pdoActive.Load() {
		pdoCyclicMu.Unlock()
		return
	}
	pdoActive.Store(false)
	done := pdoCyclicDone // capture before unlock — restart could race it
	close(pdoStopCh)
	pdoCyclicMu.Unlock()

	// Block until goroutine confirms PDS walk complete (or 2s timeout).
	select {
	case <-done:
		logger.Info("[PDO] StopPDOCyclic: goroutine exited cleanly")
	case <-time.After(2 * time.Second):
		logger.Warn("[PDO] StopPDOCyclic: timed out — proceeding with release")
	}

	// ecrt_release_master: paired call to ecrt_request_master(). Lets IgH walk
	// ESM down gracefully (SAFEOP→PREOP→IDLE) — prevents Err88.2 on exit.
	for _, dev := range masterDevicesForRelease {
		if dev.Master != nil {
			logger.Info("[PDO] Releasing EtherCAT master for device:", dev.Name)
			C.ecrt_release_master(dev.Master)
		}
	}
}

// masterDevicesForRelease holds the device list for use in StopPDOCyclic.
// Set by StartPDOCyclic so StopPDOCyclic does not depend on package-level
// masterDevices (which lives in init_master.go).
var masterDevicesForRelease []*MasterDevice

// PDOSetDigitalOutput atomically queues a digital output write for the next
// PDO cycle. mask selects which output bits are driven by the EtherCAT master
// (1 = controlled, 0 = left to drive default). val sets the output levels.
//
// Example — assert bit 0 (OUT1) high:
//   PDOSetDigitalOutput(devices, 0x00000001, 0x00000001)
// Example — release all outputs:
//   PDOSetDigitalOutput(devices, 0x00000000, 0x00000000)
//
// Thread-safe: may be called from any goroutine while PDO is active.
func PDOSetDigitalOutput(devices []*MasterDevice, mask uint32, val uint32) {
	if len(devices) == 0 {
		return
	}
	d := devices[0]
	if !d.PdoDigOutReady {
		return
	}
	d.desiredDigOutMask.Store(mask)
	d.desiredDigOutVal.Store(val)
	fmt.Printf("[PDO-DIGOUT] queued mask=0x%08X val=0x%08X\n", mask, val)
}

// StartPDOCyclic — owns master_receive / master_send.
// Safe to call after every reset. The mutex ensures only one cyclic
// goroutine runs at a time.
func StartPDOCyclic(devices []*MasterDevice) error {
	pdoCyclicMu.Lock()
	defer pdoCyclicMu.Unlock()

	if len(devices) == 0 {
		return errors.New("StartPDOCyclic: no devices")
	}

	d := devices[0]

	if !d.PdoReady || d.Domain == nil {
		return errors.New("StartPDOCyclic: PDO not configured — call SetupPDOPosition first")
	}

	if pdoActive.Load() {
		return nil // already running
	}

	// Activate master — after this point NO more PDO registrations allowed.
	if C.ecrt_master_activate(d.Master) != 0 {
		return errors.New("StartPDOCyclic: ecrt_master_activate failed")
	}

	// Get process data pointer.
	pd := C.ecrt_domain_data(d.Domain)
	if pd == nil {
		return errors.New("StartPDOCyclic: ecrt_domain_data returned nil")
	}
	d.DomainPD = (*C.uint8_t)(pd)

	// Fresh stop/done channels for this run.
	pdoStopCh     = make(chan struct{})
	pdoCyclicDone = make(chan struct{})
	// masterDevicesForRelease is written here (under pdoCyclicMu) and read by
	// StopPDOCyclic (also under pdoCyclicMu). Both functions hold the mutex for
	// their full critical section, so there is no window where one sees a stale
	// value from a previous run. Safe.
	masterDevicesForRelease = devices
	pdoActive.Store(true)

	go func() {
		logger.Info("[PDO] cyclic task started (1 ms period)")

		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()
		latchCounter := 0

		for {
			select {
			case <-pdoStopCh:
				// This is the safety net if ShutdownMasters was called
				if d.PdoRxReady {
					logger.Info("[PDO] Final check: ensuring 'Ready To Switch On'...")
					deadline := time.Now().Add(500 * time.Millisecond)
					for time.Now().Before(deadline) {
						C.ecrt_master_receive(d.Master)
						C.ecrt_domain_process(d.Domain)
						
						sw := uint16(C.read_u16(d.DomainPD, d.OffStatus))
						// Force the Shutdown command to the hardware
						C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(0x0006))
						
						C.ecrt_domain_queue(d.Domain)
						C.ecrt_master_send(d.Master)
						
						if sw&0x006F == 0x0021 || sw&0x006F == 0x0040 {
							break
						}
						time.Sleep(1 * time.Millisecond)
					}
				}
				close(pdoCyclicDone)
				return

			case <-ticker.C:
				// ---- Receive ----
				C.ecrt_master_receive(d.Master)
				C.ecrt_domain_process(d.Domain)

				// ============================================================
				// 1. READ TxPDO (drive -> master)
				// ============================================================
				var status uint16
				if d.PdoStatusReady {
					status = uint16(C.read_u16(d.DomainPD, d.OffStatus))
					lastPDOStatus.Store(uint32(status))
				}
				if d.PdoReady {
					pos := C.read_s32(d.DomainPD, d.OffPos)
					atomic.StoreInt32(&lastPDOPos, int32(pos))
				}
				if d.PdoErrorReady {
					ec := C.read_u16(d.DomainPD, d.OffErrorCode)
					// FIX: Only cache the error code when the drive is in Fault state
					// (statusword bit3 = 1). 0x603F is a hardware-latching register on
					// the Panasonic A6 — it never auto-clears even after a CiA-402 fault
					// reset. Writing it unconditionally every 1ms overwrites the Store(0)
					// in PDOFaultReset and causes pollDriveErrWorker to re-fire alarm 87
					// on every reset. When bit3 = 0 (no fault) we write 0 instead.
					if status&0x0008 != 0 {
						lastPDOErr.Store(uint32(ec))
					} else {
						lastPDOErr.Store(0)
					}
				}
				if d.PdoDIReady {
					di := C.read_u32(d.DomainPD, d.OffDigitalInputs)
					lastPDODI.Store(uint32(di))
				}

				// ============================================================
				// 2. CiA-402 STATE MACHINE -- always run, every cycle.
				// NOTE: 0x1600 has 4 entries only (CW, OpMode, TargetPos, TargetVel).
				// No placeholder fields exist -- do NOT write to computed offsets.
				// ============================================================
				opEnabled := false
				doFaultReset := d.pdoFaultResetActive.Load()
				cw := cia402NextControlword(status, &opEnabled, doFaultReset)

				// ---- Debug (throttled): print jog state and command values ----
				if d.IsJogEnabled() {
					now := time.Now().UnixNano()
					last := lastDebugLog.Load()
					if now-last > int64(500*time.Millisecond) {
						lastDebugLog.Store(now)
						cwDbg := uint16(d.desiredControlWord.Load() & 0xFFFF)
						opDbg := int8(d.desiredOpMode.Load())
						velDbg := d.desiredTargetVelocity.Load()
						logger.Info("[PDO-DBG] status=0x", fmt.Sprintf("%04X", status),
							"opEnabled=", opEnabled,
							"cw=", fmt.Sprintf("%04X", cwDbg),
							"op=", opDbg,
							"vel=", velDbg,
							"pos=", GetLastPDOPosition())
					}
				}

				// ============================================================
				// 3. WRITE RxPDO (master -> drive, motion commands)
				// ============================================================
				if opEnabled {
					// Drive is in Operation Enabled — we can command motion.
					switch {
					case d.IsJogEnabled():
						// ---- VELOCITY / JOG MODE (mode 3 = Profile Velocity) ----
						cwJog := uint16(d.desiredControlWord.Load() & 0xFFFF)
						if cwJog == 0 {
							cwJog = 0x000F
						}
						C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(cwJog))
						op := int8(d.desiredOpMode.Load())
						if op == 0 {
							op = 3
						}
						C.write_s8(d.DomainPD, d.OffOpMode, C.int8_t(op))
						vel := d.desiredTargetVelocity.Load()
						C.write_s32(d.DomainPD, d.OffTargetVel, C.int32_t(vel))
						// Hold target pos at current actual so CSP won't lurch on mode switch
						actual := atomic.LoadInt32(&lastPDOPos)
						C.write_s32(d.DomainPD, d.OffTargetPos, C.int32_t(actual))
						d.currentTargetPosition.Store(actual)

					case d.IsPosEnabled():
						// ---- POSITION MOVE (mode 1 = Profile Position) ----
						goal := d.desiredTargetPosition.Load()
						C.write_s8(d.DomainPD, d.OffOpMode, C.int8_t(1)) // Profile Position
						C.write_s32(d.DomainPD, d.OffTargetPos, C.int32_t(goal))
						C.write_s32(d.DomainPD, d.OffTargetVel, C.int32_t(0))
						
						base := uint16(0x000F | 0x0020)
						
						if d.ppSetpointPending.Load() {
							// RACE CONDITION FIX: Emulate old SDO sequential safety.
							// PDOs send all data simultaneously. We must hold bit 4 LOW for a few 
							// cycles to guarantee the drive processes 0x607A (Target Position)
							// BEFORE it processes 0x6040 (Controlword).
							if latchCounter < 10 {
								latchCounter++
								C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(base)) // keep bit4 LOW
							} else {
								C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(base|0x0010)) // set bit4
								// Wait for drive to acknowledge the command
								if (status & 0x1000) != 0 {
									d.ppSetpointPending.Store(false)
									latchCounter = 0 // reset for next move
								}
							}
						} else {
							C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(base)) // bit4 cleared
							latchCounter = 0
						}
						d.currentTargetPosition.Store(goal)

					default:
						// ---- STANDBY / SHUTDOWN / MULTITURN RESET ----
						if d.pdoShutdownActive.Load() || d.pdoMTResetActive.Load() {
							// pdoShutdownActive: graceful app shutdown — walk PDS to Ready To Switch On.
							// pdoMTResetActive:  multiturn reset — drive needs SRV-OFF state.
							// Both need CW=0x0006 (Shutdown).
							C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(0x0006))
						} else {
							C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(0x000F))
						}
						// HOLD IN MODE 3 (Profile Velocity, vel=0) instead of switching
						// to mode 8 (CSP). The Panasonic A6 reinitialises its position
						// controller on a mode 3→8 transition and fires a correction-torque
						// pulse — the visible jerk even when the motor is already stopped.
						// Staying in mode 3 with vel=0 holds via the velocity loop with
						// zero mode-switch overhead.
						C.write_s8(d.DomainPD, d.OffOpMode, C.int8_t(3))
						actual := atomic.LoadInt32(&lastPDOPos)
						C.write_s32(d.DomainPD, d.OffTargetPos, C.int32_t(actual))
						C.write_s32(d.DomainPD, d.OffTargetVel, C.int32_t(0))
						d.currentTargetPosition.Store(actual)
					}
				} else {
					// Drive not yet in Operation Enabled — write state-machine CW,
					// set safe defaults on all motion outputs.
					// If jog/pos is requested, force-enable (0x000F) unless faulted.
					if (d.IsJogEnabled() || d.IsPosEnabled()) && (status&0x0008) == 0 {
						cw = 0x000F
					}
					// Lock the state machine in Shutdown during multiturn reset or app shutdown.
					if d.pdoMTResetActive.Load() || d.pdoShutdownActive.Load() {
						cw = 0x0006
					}
					C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(cw))
					// Choose opmode based on requested motion while enabling.
					if d.IsJogEnabled() {
						cwJog := uint16(d.desiredControlWord.Load() & 0xFFFF)
						if cwJog == 0 {
							cwJog = 0x000F
						}
						C.write_u16(d.DomainPD, d.OffControlWord, C.uint16_t(cwJog))
						op := int8(d.desiredOpMode.Load())
						if op == 0 {
							op = 3
						}
						C.write_s8(d.DomainPD, d.OffOpMode, C.int8_t(op)) // Profile Velocity
						vel := d.desiredTargetVelocity.Load()
						C.write_s32(d.DomainPD, d.OffTargetVel, C.int32_t(vel))
					} else {
						C.write_s8(d.DomainPD, d.OffOpMode, C.int8_t(3)) // mode 3 vel=0: no mode-switch jerk on enable
						C.write_s32(d.DomainPD, d.OffTargetVel, C.int32_t(0))
					}
					actual := atomic.LoadInt32(&lastPDOPos)
					C.write_s32(d.DomainPD, d.OffTargetPos, C.int32_t(actual))
					d.currentTargetPosition.Store(actual)
				}

				// Write digital outputs: desiredDigOutMask→0x60FE:01 (pin levels),
				// desiredDigOutVal→0x60FE:02 (EtherCAT ownership mask, kept 0xFFFFFFFF).
				if d.PdoDigOutReady {
					outputVals := d.desiredDigOutMask.Load() // 0x60FE:01: which pins to drive HIGH
					ownership := d.desiredDigOutVal.Load()   // 0x60FE:02: which pins EtherCAT owns
					C.write_u32(d.DomainPD, d.OffDigOutMask, C.uint32_t(outputVals))
					C.write_u32(d.DomainPD, d.OffDigOutVal, C.uint32_t(ownership))
				}

				// NOTE: Multiturn reset (0x4D01/0x4D00) is handled via async SDO requests,
				// NOT written here per-cycle. IgH services the SDO mailbox automatically
				// inside ecrt_master_receive() above. See triggerMultiTurnReset() in reset.go.

				// ---- Queue and send ----
				C.ecrt_domain_queue(d.Domain)
				C.ecrt_master_send(d.Master)
			}
		}
	}()

	return nil
}