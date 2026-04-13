package motordriver

var speclDriver IMotorDriver

func pollIOStat(avilableDevices []*MasterDevice) {
	speclDriver = GetMotorDriver()
	speclDriver.pollIOStat(avilableDevices)
}

func stopPollIOStat() {
	speclDriver.stopPollIOStat()
}
