package motordriver

import "EtherCAT/logger"

// breakBit is the 0x60FE bit that controls the brake solenoid digital output.
// Bit 1 of 0x60FE:02 drives OUT2 (brake solenoid wire on this machine).
// Adjust if your wiring uses a different bit.
const breakBit = uint32(0x00000002)

// breakOn switches on the brake solenoid (output signal).
//
// PDO path (when cyclic task is active): atomically sets the brake bit in
// 0x60FE:01/02 via PDOSetDigitalOutput — no mailbox, no race with cyclic task.
//
// SDO fallback (before ecrt_master_activate): uses the "break_on" operation
// from the EtherCAT config, which SDO-writes 0x60FE directly.
func breakOn(masterDevice MasterDevice) error {
	logger.Debug("break on")
	if IsPDOActive() {
		if masterDevice.PdoDigOutReady {
			PDOSetDigitalOutput(masterDevices, breakBit, 0xFFFFFFFF) // assert: set output HIGH, ownership permanent
			logger.Info("[PDO] breakOn: brake asserted via 0x60FE PDO")
			return nil
		}
		logger.Warn("[PDO] breakOn: PdoDigOutReady=false, falling back to SDO (may be slow)")
	}
	operation, err := GetEtherCATOperation("break_on", masterDevice.Device.AddressConfigName)
	if err != nil {
		return err
	}
	for _, step := range operation.Steps {
		SDODownload(masterDevice.Master, masterDevice.Position, step)
	}
	return nil
}

// breakOff switches off the brake solenoid (output signal).
//
// PDO path (when cyclic task is active): clears the brake bit in 0x60FE:01/02
// via PDOSetDigitalOutput — no mailbox, no race with cyclic task.
//
// SDO fallback: uses the "break_off" operation from EtherCAT config.
func breakOff(masterDevice MasterDevice) error {
	logger.Debug("break off")
	if IsPDOActive() {
		if masterDevice.PdoDigOutReady {
			PDOSetDigitalOutput(masterDevices, 0, 0xFFFFFFFF) // release: clear output, ownership stays
			logger.Info("[PDO] breakOff: brake released via 0x60FE PDO")
			return nil
		}
		logger.Warn("[PDO] breakOff: PdoDigOutReady=false, falling back to SDO (may be slow)")
	}
	operation, err := GetEtherCATOperation("break_off", masterDevice.Device.AddressConfigName)
	if err != nil {
		return err
	}
	for _, step := range operation.Steps {
		SDODownload(masterDevice.Master, masterDevice.Position, step)
	}
	return nil
}