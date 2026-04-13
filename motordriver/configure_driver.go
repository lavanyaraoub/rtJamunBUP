package motordriver

import logger "EtherCAT/logger"

//configureDriver configure the driver which sets Drive mode, profile acceleration etc.
func configureDriver(device MasterDevice) error {
	operation, err := GetEtherCATOperation("configure", device.Device.AddressConfigName) //ethercatAddress.GetOperation("configure")
	if err != nil {
		return err
	}
	logger.Info("configuring master driver: ", device.Name)
	for _, step := range operation.Steps {
		err := SDODownload(device.Master, device.Position, step)
		if err != nil {
			return err
		}
	}
	return nil
}
