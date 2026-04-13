package motordriver

//GetMotorDriver return specialized motor driver, for now returns only A6 Minas driver
func GetMotorDriver() IMotorDriver {
	return &A6Minas{}
}
