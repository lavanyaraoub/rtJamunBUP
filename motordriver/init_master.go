package motordriver

import (
	parser "EtherCAT/configparser"
	ethercatDevice "EtherCAT/ethercatdevicedatatypes"
	"EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	settings "EtherCAT/settings"
	"fmt"
	"time"
)

var driverConnectionStatus = "SUCCESS"

// masterDevices is a slice of POINTERS so that PDO flags set by SetupPDOPosition
// (e.g. PdoReady, PdoJogReady, OffControlWord …) are visible to every goroutine
// that holds a reference to the same device.  The old []MasterDevice (value slice)
// meant that goroutines launched with a copy never saw the flags written by
// SetupPDOPosition, so device.PdoReady was always false inside pollDrivePositionProcess.
var masterDevices []*MasterDevice

var ethercatAddressMapping map[string]ethercatDevice.Ethercat

// InitMaster initialises all EtherCAT masters and brings up the drives.
//
// Sequence (order matters for IgH EtherCAT master correctness):
//  1. Parse config
//  2. RequestMaster  — kernel module claims the bus
//  3. configureDriver — SDO parameter writes (mode, gains, limits …)
//  4. reverseDir / nonReverseDir — SDO polarity write
//  5. PowerOn — SDO enable
//  6. SetupPDOPosition — registers domain + PDO entry offsets (still pre-activation)
//  7. setupDrivers — SDO clamp/declamp decision  ← MUST be before activation
//  8. StartPDOCyclic — calls ecrt_master_activate(); NO MORE SDO after this point
//  9. Start listener goroutines
func InitMaster() error {
	ethercatAddressMapping = make(map[string]ethercatDevice.Ethercat)

	devices, err := parser.ParseDeviceConfig()
	if err != nil {
		return err
	}

	// Parse device-specific address mapping files
	for _, dev := range devices.Device {
		address, parseErr := parser.ParseEthercatAddressConfig(dev.AddressConfigFile)
		if parseErr != nil {
			return parseErr
		}
		ethercatAddressMapping[dev.AddressConfigName] = address
	}

	if len(devices.Device) == 0 {
		return fmt.Errorf("InitMaster: no devices configured")
	}

	// REQUEST MASTER ONCE — all slaves share a single EtherCAT master handle.
	// Calling ecrt_request_master(0) more than once per process causes the IgH
	// kernel module to return an error on the second call.
	// The old version had an explicit "REQUEST MASTER ONLY ONCE << FIX" comment
	// that was lost when the loop was refactored.
	master0, reqErr := RequestMaster(devices.Device[0])
	if reqErr != nil {
		logger.Error("RequestMaster error:", reqErr)
		return reqErr
	}
	if master0 == nil {
		return fmt.Errorf("InitMaster: RequestMaster returned nil master")
	}

	position := 0
	for _, i := range devices.Device {

		// Allocate on the heap so every holder sees the same struct.
		masterDevice := &MasterDevice{
			Master:   master0,
			Position: position,
			Name:     i.Name,
			Device:   i,
		}
		masterDevices = append(masterDevices, masterDevice)

		// ---- All SDO work BEFORE ecrt_master_activate ----

		configErr := configureDriver(*masterDevice)
		if configErr != nil {
			driverConnectionStatus = "ERROR"
			statusnotifier.DriverStatus(i.Name, "0")
			return configErr
		}

		drvSettings := settings.GetDriverSettings(masterDevice.Name)
		if drvSettings.MotorDirection == 1 {
			nonReverseDir(*masterDevice)
		} else {
			reverseDir(*masterDevice)
		}

		statusnotifier.DriverStatus(i.Name, "1")
		time.Sleep(500 * time.Millisecond)
		PowerOn(*masterDevice)
		time.Sleep(500 * time.Millisecond)

		// Register PDO domain entries. ecrt_master_activate has NOT been called yet,
		// so SDO and PDO registration can coexist here safely.
		if pdoErr := SetupPDOPosition(masterDevice); pdoErr != nil {
			logger.Warn("PDO setup failed for", i.Name, "— will fall back to SDO polling:", pdoErr)
		} else {
			logger.Info("PDO setup successful for", i.Name,
				"PdoJogReady:", masterDevice.PdoJogReady,
				"PdoPosReady:", masterDevice.PdoPosReady)
		}

		position++
	}

	if len(masterDevices) == 0 {
		return nil
	}

	// setupDrivers issues FastPowerOff / FastPowerOn via SDO.
	// This MUST happen before StartPDOCyclic calls ecrt_master_activate.
	setupDrivers(masterDevices)

	// Activate master and start cyclic task. After this line, no SDO calls
	// may be made directly — use the PDO process image exclusively.
	pdoOK := false
	if masterDevices[0].PdoReady {
		if startErr := StartPDOCyclic(masterDevices); startErr != nil {
			logger.Warn("PDO cyclic start failed — falling back to SDO polling:", startErr)
		} else {
			pdoOK = true
			logger.Info("[PDO] Cyclic task running. SDO access is now DISABLED.")
		}
	}

	initListeners(masterDevices, pdoOK)

	time.Sleep(500 * time.Millisecond)
	driverConnectionStatus = "SUCCESS"
	return nil
}

// setupDrivers ensures the drive CiA-402 PDS state is safe for ESM transitions.
// Must be called BEFORE ecrt_master_activate (i.e. before StartPDOCyclic).
//
// CRITICAL — Panasonic MINAS A6 Err88.2 (manual Note 2 & 4):
//   Err88.2 fires when ESM receives a state transition command while PDS is
//   already at "Operation Enabled". This locks the drive in Fault with
//   status=0x0000 for the entire session.
//
//   FastPowerOn writes 0x6040=0x000F which sets PDS=Operation Enabled.
//   If called before ecrt_master_activate(), the drive is at Operation Enabled
//   when IgH walks ESM PREOP→SAFEOP→OP → Err88.2 fires immediately.
//
//   Fix: always call FastPowerOff (0x6040=0x06, Shutdown command) here.
//   This leaves PDS at "Ready To Switch On" — safe for all ESM transitions.
//   The cia402NextControlword() state machine in the PDO cyclic task then walks
//   PDS all the way to "Operation Enabled" automatically once ESM=OP.
func setupDrivers(devices []*MasterDevice) {
	for _, dev := range devices {
		// ── Step 1: Fault reset — clear any stale fault from a previous session ──
		//
		// ROOT CAUSE OF Err80.4 PERSISTING ACROSS RESTARTS:
		//
		// The Panasonic A6's CiA-402 PDS (Power Drive System) state is retained
		// across EtherCAT resets. If the previous session ended while the drive was
		// in Fault state (e.g. the process was killed, or the Pi was rebooted without
		// a clean shutdown), the drive wakes up still in Fault on the next run.
		//
		// FastPowerOff writes CW=0x0006 (Shutdown), which is a valid transition from
		// "Operation Enabled" or "Switched On" to "Ready To Switch On". But CW=0x0006
		// has NO effect on the Fault state — the drive remains in Fault. Then when
		// IgH walks PREOP→SAFEOP→OP, the drive enters OP still in Fault state, and
		// the 0x603F register (hardware-latching) still holds the old error code.
		// The PDO error poller reads this and fires the alarm — including "ESM
		// unauthorized request error protection" (Err80.4) — on every startup.
		//
		// Fix: send CW=0x0080 (Fault Reset command) before FastPowerOff. In CiA-402:
		//   Fault → [CW=0x0080 rising edge] → Switch On Disabled → [CW=0x0006] → Ready To Switch On
		// This is safe pre-activation (pure SDO write) and is a no-op if the drive
		// is NOT in Fault state (the Shutdown walk handles that case).
		if operation, err := GetEtherCATOperation("faultReset", dev.Device.AddressConfigName); err == nil {
			logger.Info("[SETUP] Issuing pre-activation fault reset to clear any stale drive fault:", dev.Name)
			for _, step := range operation.Steps {
				if step.Action == "read" {
					SDOUpload2(dev.Master, dev.Position, step)
				} else {
					SDODownload(dev.Master, dev.Position, step)
				}
			}
			// Give the drive 200 ms to process the fault reset and walk
			// back to Switch On Disabled before we issue Shutdown.
			time.Sleep(200 * time.Millisecond)
		} else {
			logger.Warn("[SETUP] No faultReset operation configured — skipping pre-activation fault clear:", err)
		}

		// ── Step 2: Shutdown — leave PDS at "Ready To Switch On" ───────────────
		//
		// After the fault reset above (or if the drive was already healthy), issue
		// Shutdown (CW=0x0006) to walk PDS to "Ready To Switch On". This is the
		// required pre-activation state per the Err88.2 fix: the drive must NOT be
		// at "Operation Enabled" when IgH walks ESM PREOP→SAFEOP→OP.
		FastPowerOff(*dev)
	}
}

// initListeners starts all background goroutines.
// pdoOK tells it whether the PDO cyclic task is running.
func initListeners(devices []*MasterDevice, pdoOK bool) {
	if len(devices) == 0 {
		return
	}

	initDriverActionListener()
	initDriverStatusKeeperListener()
	listenSystemReset()

	// pollDrivePosition is the unified position poller.
	// Internally it checks device.PdoReady to decide whether to read from the
	// PDO memory buffer or fall back to SDO — and it always notifies the UI.
	pollDrivePosition(devices)
	// Only poll errors via SDO when PDO is not running.
	pollDriveError(masterDevices)
	pollIOStat(masterDevices)
}

// ShutdownMasters safely stops all drives and the PDO cyclic task.
//
// Three-phase shutdown — prevents Err88.2 ("ESM unauthorized request error
// protection" raised when the IgH kernel watchdog fires OP→SAFEOP while the
// drive PDS is still at Operation Enabled):
//
//   Phase 1 — Arm pdoShutdownActive on every device.
//     The cyclic standby branch writes CW=0x0006 (Shutdown) instead of
//     CW=0x000F on every 1ms tick. This walks PDS via the normal ticker —
//     no separate loop, no goroutine coordination needed yet.
//
//   Phase 2 — Poll statusword until PDS = "Ready To Switch On" (sw&0x6F==0x0021).
//     Uses the lastPDOStatus atomic updated by the still-running cyclic task.
//     The A6 transitions in ~5ms; we allow up to 1s.
//
//   Phase 3 — StopPDOCyclic.
//     The IgH watchdog OP→SAFEOP fires now, but PDS is already at a safe idle
//     state. StopPDOCyclic also calls ecrt_release_master which walks the ESM
//     down gracefully (SAFEOP→PREOP→IDLE) giving the drive time to settle.
//
// NOTE: PDOFaultReset is NOT called here. It would walk the drive BACK to
// Operation Enabled (step 5 waits for 0x0027) which is exactly the wrong state
// to be in before shutdown.
func ShutdownMasters() {
	if !IsPDOActive() {
		return
	}

	logger.Info("[SHUTDOWN] Starting 3-phase graceful shutdown to prevent Err88.2")

	// Phase 1: Halt motion + arm shutdown flag on all devices.
	for _, dev := range masterDevices {
		dev.EnableJogPDO(false)
		dev.EnablePosPDO(false)
		dev.desiredTargetVelocity.Store(0)
		dev.pdoShutdownActive.Store(true) // Forces cyclic loop to send CW=0x0006
	}
	logger.Info("[SHUTDOWN] pdoShutdownActive=true — cyclic now sends CW=0x0006 every tick")

	// Phase 2: Wait for PDS to reach a safe idle state (Ready To Switch On).
	// Increased deadline to 2 seconds to eliminate intermittent timeout warnings.
	deadline := time.Now().Add(2 * time.Second)
	var sw uint16
	for time.Now().Before(deadline) {
		sw = GetLastPDOStatusword()
		state := sw & 0x006F
		
		// 0x0021 = Ready To Switch On (Target State)
		// 0x0040 = Switch On Disabled (Also safe)
		// bit3   = Drive already faulted (Nothing more to do)
		if state == 0x0021 || state == 0x0040 || (sw&0x0008) != 0 {
			logger.Info(fmt.Sprintf("[SHUTDOWN] PDS safe — sw=0x%04X, proceeding to StopPDOCyclic", sw))
			break
		}
		// Brief sleep to avoid hammering the atomic CPU load
		time.Sleep(10 * time.Millisecond)
	}

	// Final check before proceeding
	if finalState := GetLastPDOStatusword() & 0x006F; finalState == 0x0027 {
		logger.Warn(fmt.Sprintf("[SHUTDOWN] Timeout: Drive still at Op Enabled (0x27) — sw=0x%04X", GetLastPDOStatusword()))
	}

	// Phase 3: Stop cyclic + release master (ecrt_release_master called inside).
	StopPDOCyclic()
}
// PowerOnMasters powers on all available masters.
func PowerOnMasters() {
	for _, device := range masterDevices {
		PowerOn(*device)
	}
}

// GetEtherCATOperation returns the operation configured in ethercat-device-addressing.yml.
func GetEtherCATOperation(operation string, deviceAddressConfigName string) (ethercatDevice.Operation, error) {
	addressMapping := getEtherCATAddress(deviceAddressConfigName)
	return addressMapping.GetOperation(operation)
}

// HasDriverConnected returns true when all drivers connected successfully.
func HasDriverConnected() bool {
	return driverConnectionStatus == "SUCCESS"
}

func getMasterDevices() []*MasterDevice {
	return masterDevices
}

func getEtherCATAddress(deviceAddressConfigName string) ethercatDevice.Ethercat {
	return ethercatAddressMapping[deviceAddressConfigName]
}