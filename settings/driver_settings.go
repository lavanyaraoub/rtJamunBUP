package settings

import (
	"EtherCAT/channels"
	"EtherCAT/helper"
	"encoding/json"
	"io/ioutil"
	"strconv"
    "os"
)

//DriverSettings keeps the settings of the driver
//If the settings file uses values are enclosed in brackets use this approach
//https://stackoverflow.com/a/9573928/955092
//eg. FinishSignal       int       `json:"fin_signal,int"`
//	  HomingOffset       float32   `json:"homing_offset,float32"`
type DriverSettings struct {
	FinishSignal       int          `json:"fin_signal"`
	WorkOffSet         float64      `json:"work_offset,string"`
	JogFeed            int          `json:"jog_feed,string"`
	HomingOffset       float32      `json:"homing_offset,string"`
	HomeDirection      int          `json:"home_dir"`
	GearRation         string       `json:"gear_ratio"`
	ECSFinTiming       int          `json:"timing,string"`
	BackLash           float64      `json:"back_lash,string"`
	ECS                int          `json:"ecs"`
	ClampDeclamp       int          `json:"cl_dl"`
	G55                float64      `json:"g55,string"`
	G54                float64      `json:"g54,string"`
	NOT                int          `json:"not,string"`
	G57                float64      `json:"g57,string"`
	MotorDirection     int          `json:"motor_dir"`
	G58                float64      `json:"g58,string"`
	ClampDeclampTiming int          `json:"cldl_timing,string"`
	POT                int          `json:"pot,string"`
	G56                float64      `json:"g56,string"`
	PitchError         []Float64Str `json:"pitch_error"`
	Mode               string
	FactorBacklash     int
	BinaryPosFeeds     []BinaryPosFeed `json:"binary_pos_feed"`
	LineNumber          string `json:"line_number,omitempty"`
}

type BinaryPosFeed struct {
	Binary    string `json:"binary"`
	Position  string `json:"pos"`
	Direction int16  `json:"dir"`
	FeedRate  int32  `json:"feed_rate"`
}

//Float64Str custom parsing for PitchError, pitch error is coming from client as float string
//this change is based on the post from https://stackoverflow.com/questions/49415573/golang-json-how-do-i-unmarshal-array-of-strings-into-int64
type Float64Str float64

type TextProgramConfig struct {
    IP       string `json:"ip"`
    User     string `json:"user"`
    Password string `json:"password"`
    Path     string `json:"path"`
	JogClockwisePath        string `json:"jogClockwisePath"`
    JogCounterClockwisePath string `json:"jogCounterClockwisePath"`

}

// SaveTextProgramConfig marshals the struct to JSON and saves it to a file
func SaveTextProgramConfig(config TextProgramConfig) error {
    // Convert the struct into a nicely formatted JSON byte slice
    fileData, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return err // Return error if converting to JSON fails
    }

    // Write the JSON data to the textprogram.json file
    return os.WriteFile("/mnt/app/jamun/settings/textprogram.json", fileData, 0644)
}

//for renishaw to load the text
func LoadTextProgramConfig() (TextProgramConfig, error) {
    var config TextProgramConfig

    fileData, err := os.ReadFile("/mnt/app/jamun/settings/textprogram.json")
    if err != nil {
        return config, err
    }

    err = json.Unmarshal(fileData, &config)
    return config, err
}


//MarshalJSON custom Marsh for Float64Str
func (i Float64Str) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatFloat(float64(i), 'f', 3, 64))
}

//UnmarshalJSON custom UnMarsh for Float64Str
func (i *Float64Str) UnmarshalJSON(b []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		value, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*i = Float64Str(value)
		return nil
	}

	// Fallback to number
	return json.Unmarshal(b, (*float64)(i))
}

//TODO add code to get pitch error value
//refer machine_parser.js getAngleWithPitchError line# 359

// type SettingsRoot struct {
// 	DriveSettings DriverSettings `json:"X"`
// }

//SettingsRoot root struct of the settings
type SettingsRoot map[string]DriverSettings

var settingsRoot SettingsRoot

//LoadDriverSettings Load driver settings from settings.json file
func LoadDriverSettings() error {
	path := helper.AppendWDPath("/settings/settings.json")
	settingsFile, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(settingsFile, &settingsRoot)
	//if ECS finish timing is not specified then default to 500ms
	// if driverSettings.ECSFinTiming <= 0 {
	// 	driverSettings.ECSFinTiming = 500000
	// }
	channels.NotifyMotorDriver("SETTINGS_CHANGED", "", "", 0)
	return err
}

func GetAllSettings() map[string]DriverSettings {
	return settingsRoot
}

//GetDriverSettings get loaded driver settings
func GetDriverSettings(driverName string) DriverSettings {
	return settingsRoot[driverName]
}

//SetMode set mode whether running in ABS(absolute) or not
// func SetMode(mode string) {
// 	driverSettings.Mode = mode
// }

//GetWorkOffset get the workoff set configured in settings. workoffsets are like G55, G56 etc
func (ds *DriverSettings) GetWorkOffset() map[string]float64 {
	wrkOffset := make(map[string]float64)
	wrkOffset["G53"] = 0
	wrkOffset["G54"] = ds.G54
	wrkOffset["G55"] = ds.G55
	wrkOffset["G56"] = ds.G56
	wrkOffset["G57"] = ds.G57
	wrkOffset["G58"] = ds.G58
	return wrkOffset
}
