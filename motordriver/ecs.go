package motordriver

import (
	"fmt"
    channels "EtherCAT/channels"
    logger "EtherCAT/logger"
    "EtherCAT/settings"
    "sync/atomic"
    "time"
)

var stopECSCheckChan chan bool
var isECSCheckInProgress atomic.Bool

func init() {
    stopECSCheckChan = make(chan bool, 1)
}

func stopECSCheck() {
    if isECSCheckInProgress.Load() {
        select {
        case stopECSCheckChan <- true:
        default:
        }
    }
    isECSCheckInProgress.Store(false)
}

// -------------------------------------------------------------------
// ECS “GO-HIGH” Phase — wait until ECS goes high (ready to move)
// -------------------------------------------------------------------
func doECSCheck(masterDevice MasterDevice, degree float64) int {
    envSettings := settings.GetDriverSettings(masterDevice.Name)
    if envSettings.ECS == 1 {
        logger.Debug("driver", masterDevice.Name, "waiting for ECS HIGH. rotate to", degree)
        ecsRec, _ := waitForECS(masterDevice)
        if ecsRec == 1 {
            logger.Debug("driver", masterDevice.Name, "received ECS HIGH")
        } else if ecsRec == 0 {
            logger.Error("driver", masterDevice.Name, "NOT received ECS HIGH")
            channels.SendAlarm("Not received ECS")
        } else {
            logger.Info("exiting ECS check due to stop/reset event")
        }
        return ecsRec
    }
    return 1
}

// -------------------------------------------------------------------
// ECS “GO-LOW” Phase — wait forever until ECS goes low again
// -------------------------------------------------------------------
func doECSCheckZero(masterDevice MasterDevice, degree float64) int {
    envSettings := settings.GetDriverSettings(masterDevice.Name)
    if envSettings.ECS == 1 {
        logger.Debug("driver", masterDevice.Name, "waiting for ECS LOW (zero). rotate to", degree)
        ecsRec, _ := waitForECSZero(masterDevice)
        if ecsRec == 1 {
            logger.Debug("driver", masterDevice.Name, "ECS went LOW (zero phase done)")
        } else if ecsRec == 0 {
            logger.Error("driver", masterDevice.Name, "ECS zero NOT received")
            channels.SendAlarm("ECS did not go LOW")
        } else {
            logger.Info("exiting ECS zero check as program stop/reset event received")
        }
        return ecsRec
    }
    return 1
}

// -------------------------------------------------------------------
// Wait for ECS HIGH (ready)
// -------------------------------------------------------------------
func waitForECS(masterDevice MasterDevice) (int, error) {
    operation, err := GetEtherCATOperation("ecs", masterDevice.Device.AddressConfigName)
    if err != nil {
        return 0, err
    }
    driver := GetMotorDriver()
    isECSCheckInProgress.Store(true)
    ecsStat, err := driver.receivedECS(masterDevice, operation, stopECSCheckChan), nil
    isECSCheckInProgress.Store(false)
    return ecsStat, err
}

// -------------------------------------------------------------------
// Wait for ECS LOW (forever, no timeout)
// -------------------------------------------------------------------
func waitForECSZero(masterDevice MasterDevice) (int, error) {
    isECSCheckInProgress.Store(true)
    defer func() { isECSCheckInProgress.Store(false) }()

    logger.Info("Waiting indefinitely for ECS to go LOW...")

    const ecsBit = 1 // bit 0 of 0x4F25
    for {
        val, _ := readDigitalInputs(masterDevice, "ecs")
        ecsHigh := (val & ecsBit) != 0

        if !ecsHigh {
            // Double-check: confirm ECS stays LOW for 5×20ms = 100ms
            stable := true
            for i := 0; i < 5; i++ {
                time.Sleep(20 * time.Millisecond)
                v, _ := readDigitalInputs(masterDevice, "ecs")
                if (v & ecsBit) != 0 {
                    stable = false
                    break
                }
            }
            if stable {
                logger.Info("ECS signal is LOW – continuing execution")
                return 1, nil
            }
        }

        select {
        case <-stopECSCheckChan:
            logger.Warn("ECS zero wait interrupted by stop/reset")
            return 2, nil
        default:
        }

        time.Sleep(50 * time.Millisecond)
    }
}

// -------------------------------------------------------------------
// Send finish signal to controller after motion complete
// -------------------------------------------------------------------
// sendECSFinSignal sends the ECS finish signal to the controller after motion
// completes. This toggles a digital output on the drive (object 0x60FE).
//
// PDO path (TODO — in progress): 0x60FE is already present in the RxPDO
// mapping (0x1600) but its offset is currently discarded by setup_all_rx_pdo.
// To complete PDO conversion, expose OffDigitalOutputs in MasterDevice and
// write the fin-signal bit there instead of via SDO.
//
// Current behaviour: if PDO is active, the SDO path is blocked (SDO writes
// to 0x60FE after ecrt_master_activate require the mailbox). The fin signal
// is skipped and the UI is notified. This is safe — the fin signal is
// informational; missing it does not cause drive faults or motion errors.
//
// If FinishSignal is 0 in driver settings, this function is a no-op.
func sendECSFinSignal(device MasterDevice) error {
    driverSettings := settings.GetDriverSettings(device.Name)
    if driverSettings.FinishSignal == 0 {
        return nil
    }

    if IsPDOActive() {
        // PDO path for Panasonic A6 0x60FE:
        //   0x60FE:01 (OffDigOutMask in domain) = TARGET output values  (1=pin HIGH)
        //   0x60FE:02 (OffDigOutVal  in domain) = EtherCAT ownership mask (1=EtherCAT controls this bit)
        //
        // IMPORTANT: 0x60FE:02 ownership mask must be SET before 0x60FE:01 has any effect.
        // The drive only respects 0x60FE:01 bits where 0x60FE:02 is also 1.
        // We keep ownership mask permanently asserted for the fin signal bit(s).
        //
        // finBit: which physical output pin is the fin signal?
        //   Panasonic A6 SO1 = bit 0 (0x01), SO2 = bit 1 (0x02), SO3 = bit 2 (0x04)
        //   Check drive wiring: which SO terminal connects to the fin-signal relay.
        //   Also check Pr5.04 (SO1 function) = must be 0x00 (Network/Free) not SRDY.
        // Derive finBit from the SDO "finsignal" operation config.
        // The SDO config writes two steps to 0x60FE:01 and 0x60FE:02;
        // the value written to the first step (0x60FE:01 = output value) encodes
        // which physical output bit is the fin signal.
        // We parse it once here to avoid hardcoding the bit.
        finBit := uint32(0x00000001) // fallback: SO1
        if sdoOp, sdoErr := GetEtherCATOperation("finsignal", device.Device.AddressConfigName); sdoErr == nil {
            for _, step := range sdoOp.Steps {
                if step.Action == "write" {
                    if val, parseErr := step.GetValue(); parseErr == nil && val != 0 {
                        finBit = uint32(val)
                        break
                    }
                }
            }
        }
        logger.Info("[PDO] sendECSFinSignal: finBit=", fmt.Sprintf("0x%08X", finBit),
            " (mask→0x60FE:01 at byte 11, ownership→0x60FE:02 at byte 15)")

        notifyDriverStatus("fin_signal", "1", device)
        // Assert: set ownership mask AND output value simultaneously
        // Assert: set output pin HIGH (mask → 0x60FE:01), ownership already permanent
        PDOSetDigitalOutput(masterDevices, finBit, 0xFFFFFFFF)
        logger.Info("[PDO] sendECSFinSignal: fin signal asserted via 0x60FE PDO")
        time.Sleep(time.Duration(driverSettings.ECSFinTiming) * time.Millisecond)
        // Release: clear ONLY the output value (0x60FE:01 bit = LOW).
        // Do NOT change ownership mask (0x60FE:02 stays 0xFFFFFFFF permanently).
        // PDOSetDigitalOutput(mask, val): mask → 0x60FE:01, val → 0x60FE:02 (ownership)
        // To clear output: set mask=0 (pin goes LOW), ownership stays via desiredDigOutVal.
        PDOSetDigitalOutput(masterDevices, 0, 0xFFFFFFFF)
        notifyDriverStatus("fin_signal", "0", device)
        logger.Info("[PDO] sendECSFinSignal: fin signal released")
        return nil
    }

    // SDO path: only safe before ecrt_master_activate.
    operation, _ := GetEtherCATOperation("finsignal", device.Device.AddressConfigName)
    operationEnd, err := GetEtherCATOperation("finsignalend", device.Device.AddressConfigName)
    if err != nil {
        return err
    }

    notifyDriverStatusWithWait("fin_signal", "1", device)
    // Log the SDO steps so we can see what 0x60FE bit pattern the config uses
    for i, step := range operation.Steps {
        logger.Info("[PDO] sendECSFinSignal SDO step", i, "action:", step.Action, "value:", step.Value, "addr:", step.Address, "sub:", step.SubIndex)
    }
    logger.Trace("start sending ecs fin signal ", device.Name)

    for _, step := range operation.Steps {
        if step.Action == "read" {
            SDOUpload2(device.Master, device.Position, step)
        } else {
            SDODownload(device.Master, device.Position, step)
        }
    }

    time.Sleep(time.Duration(driverSettings.ECSFinTiming) * time.Millisecond)

    for _, step := range operationEnd.Steps {
        if step.Action == "read" {
            SDOUpload2(device.Master, device.Position, step)
        } else {
            SDODownload(device.Master, device.Position, step)
        }
    }

    notifyDriverStatusWithWait("fin_signal", "0", device)
    logger.Trace("finish sending ecs fin signal ", device.Name)
    return nil
}

// Debug: print details of the ECS EtherCAT operation mapping
// -------------------------------------------------------------------