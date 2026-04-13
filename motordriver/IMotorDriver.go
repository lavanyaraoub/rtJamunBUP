package motordriver

import (
	ethercatDevice "EtherCAT/ethercatdevicedatatypes"
)

//This is an interface definition to create functions which cant be implemented generically
//for e.g. this interface can be implemented for minias A6 and other drivers.

//IMotorDriver interface which can be implemented for specialized purpose
type IMotorDriver interface {
	hasTargetReached(masterDevice MasterDevice, action int, immediate int, operation ethercatDevice.Operation) error
	potNotEnabled(masterDevice MasterDevice) (bool, error)
	readDeclampSignal(masterDevice MasterDevice, declampTiming int) (bool, error)
	readClampSignal(masterDevice MasterDevice, clampTiming int) (bool, error)
	receivedECS(masterDevice MasterDevice, operation ethercatDevice.Operation, stopECSChat chan bool) int
	receivedECSZero(masterDevice MasterDevice, operation ethercatDevice.Operation, stopECSChat chan bool) int
	sendFinishSignal(masterDevice MasterDevice, operation ethercatDevice.Operation) error
	pollIOStat(avilableDevices []*MasterDevice)
	stopPollIOStat()
}
