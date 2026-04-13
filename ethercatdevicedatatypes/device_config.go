package ethercatdevicedatatypes

//Device struct holds the details of the connected device.
type Device struct {
	Name              string `yaml:"name"`
	VendorID          int    `yaml:"vendor-id"`
	ProductCode       int    `yaml:"product-code"`
	Alias             int    `yaml:"alias"`
	ID                int    `yaml:"id"`
	RPMConst          int    `yaml:"rpm-const"`
	DriveXRatio       int    `yaml:"drive-x-ratio"`
	IsReady           bool
	AddressConfigName string  `yaml:"address-config-name"`
	AddressConfigFile string  `yaml:"address-config-file"`
	PotNotThreshold   float64 `yaml:"pot-not-threshold"`
	StopWhenHWPOTNOT  bool    `yaml:"stop-when-hardware-potnot"`
	IOPollingInterval int     `yaml:"io-poll-interval"`
}

//Devices holds the array of device configured
type Devices struct {
	Device []Device `yaml:"devices"`
}

// DeviceConfig holds the Devices Array
type DeviceConfig struct {
	Devices Devices
}
