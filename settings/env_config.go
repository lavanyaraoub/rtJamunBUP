package settings

import (
	"EtherCAT/helper"

	"github.com/spf13/viper"
)

//EnvironmentSettings holds the environment settings of the application
type EnvironmentSettings struct {
	Version        string
	Mode           string
	ReleaseChannel string
	LogLevel       string
	UIVer          string
	HotSpotOnStart bool
	TunnelAccess   string
	DeviceID        string
    GDriveFolderID  string

}

var envSettings = EnvironmentSettings{}

//GetEnvSettings return a struct contains the environment settings of the system
func GetEnvSettings() EnvironmentSettings {
	if (EnvironmentSettings{}) == envSettings {
		viper.SetConfigType("yaml")
		viper.SetConfigName("envconfig")
		viper.AddConfigPath(helper.AppendWDPath("/configs"))
		viper.ReadInConfig()
		envSettings.Version = viper.GetString("version")
		envSettings.Mode = viper.GetString("mode")
		envSettings.ReleaseChannel = viper.GetString("rel_channel")
		envSettings.LogLevel = viper.GetString("log_level")
		envSettings.UIVer = viper.GetString("ui")
		envSettings.HotSpotOnStart = viper.GetBool("hot_spot_on_startup")
		envSettings.TunnelAccess = viper.GetString("tunnel_access")
        //new code for loggs and google drive.
		envSettings.DeviceID = viper.GetString("device_id") 
        envSettings.GDriveFolderID = viper.GetString("gdrive_folder_id")
	}
	return envSettings
}

// //GetEnvConfig read the environment config
// func GetEnvConfig(key string) string {
// 	viper.SetConfigType("yaml")
// 	viper.SetConfigName("envconfig")
// 	viper.AddConfigPath("./configs")
// 	viper.ReadInConfig()
// 	return viper.GetString(key)
// }
