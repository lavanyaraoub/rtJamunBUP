package motordriver

/*
#cgo CFLAGS: -g -Wall -I/opt/etherlab/include -I/home/pi/gosrc/src/EtherCAT
#cgo LDFLAGS: -L/home/pi/gosrc/src/EtherCAT -L/opt/etherlab/lib/ -lethercatinterface -lethercat
#include "ethercatinterface.h"
*/
import "C"

import (
	"fmt"
	"time"
	logger "EtherCAT/logger"
)

// setRpm updates the velocity for both Jog (Mode 3) and Program Execution (Mode 1).
func setRpm(masterDevice *MasterDevice, rpm int) error {
	scaledRPM := masterDevice.Device.RPMConst * rpm

	if !IsPDOActive() || !masterDevice.PdoJogReady {
		return fmt.Errorf("setRpm: PDO not active or RxPDO not ready — " +
			"cannot set velocity setpoint (PDO mandatory in this build)")
	}

	// 1. Update the PDO cache (0x60FF) used exclusively for Manual Jog moves
	masterDevice.desiredTargetVelocity.Store(int32(scaledRPM))
	logger.Info("[PDO] setRpm: velocity setpoint cached for Jog. scaledRPM:", scaledRPM, "driver:", masterDevice.Name)

	// 2. Update Profile Velocity (0x6081) via async SDO for Program Execution (Profile Position mode).
	//
	// WHY WE MUST WAIT FOR ACK BEFORE doneDriverAction():
	//
	//   The executor runs G01 F20 → G90 → G68 → A-90 in ~6ms total.
	//   The EtherCAT CoE mailbox takes 10-20ms to deliver the SDO write to the
	//   drive. If doneDriverAction() is called before the ACK, the first PP move
	//   starts while 0x6081 still holds the startup default (1000000 counts/s).
	//   All subsequent moves are correct because the ACK arrives before the second
	//   command executes — but the first rotation runs at the wrong (slow) speed.
	//
	//   Fix: arm the SDO request then poll until SUCCESS/ERROR before unblocking
	//   the executor. The mailbox round-trip is typically 5-15ms — imperceptible
	//   to the operator but critical for velocity correctness on the first move.
	if masterDevice.PdoVelSdoReady && masterDevice.PdoVelSdoReq != nil {
		armed := false
		for i := 0; i < 10; i++ {
			rc := int(C.trigger_profile_vel_request(masterDevice.PdoVelSdoReq, C.uint32_t(scaledRPM)))
			if rc == 0 {
				armed = true
				break
			}
			if rc == -16 { // EBUSY — previous mailbox still in flight
				time.Sleep(5 * time.Millisecond)
				continue
			}
			logger.Error("[PDO] setRpm: Failed to arm Profile Velocity SDO request, rc=", rc)
			break
		}
		if armed {
			// Poll until the drive ACKs (IgH services the mailbox inside
			// ecrt_master_receive() on every 1ms cyclic tick).
			deadline := time.Now().Add(100 * time.Millisecond)
			for time.Now().Before(deadline) {
				state := int(C.get_profile_vel_state(masterDevice.PdoVelSdoReq))
				if state == 1 { // EC_REQUEST_SUCCESS
					logger.Info("[PDO] setRpm: Profile Velocity (0x6081) ACKed by drive. scaledRPM:", scaledRPM)
					break
				}
				if state == -1 { // EC_REQUEST_ERROR
					logger.Error("[PDO] setRpm: drive returned error for Profile Velocity SDO")
					break
				}
				time.Sleep(1 * time.Millisecond)
			}
		}
	} else {
		logger.Warn("[PDO] setRpm: PdoVelSdoReady is false, Profile Position feed rate will not update dynamically.")
	}

	// Signal executor to advance to the next program line.
	doneDriverAction()
	return nil
}