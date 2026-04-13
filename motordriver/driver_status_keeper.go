package motordriver

import (
	channels "EtherCAT/channels"
	logger "EtherCAT/logger"
	settings "EtherCAT/settings"
	"fmt"
	"strconv"
	"sync/atomic"

	cmap "github.com/orcaman/concurrent-map"
)

type driverCurrentStatus struct {
	currentPosition     float64
	alarm               string
	mode                string //ABS or REL (absolute or relative mode)
	shortestPathEnabled bool
	destinationPosition float64
	direction           int //rotation direction -1 anti clockwise 1 clock wise
	backlash            float64
	workOffset          float64
	potNotExceeded      bool
	potExceeded         bool
	notExceeded         bool
	isMotorRunning      bool //true when motor is rotating
	isProgramMode       bool //true while a program is executing
	backlashInSetting   float64
	isSendingFinSignal  bool
	isDriverOnOff       bool
}

func (d *driverCurrentStatus) reset() {
	d.currentPosition = 0
	d.alarm = ""
	d.mode = "ABS"
	d.shortestPathEnabled = false
	d.destinationPosition = -1
	// d.direction = 1
	d.backlash = 0
	d.workOffset = 0
	d.isMotorRunning = false
}

//EventType type of event which can be notified to driver status keeper
type EventType string




const (
	current_position     EventType = "current_position"
	mode                           = "mode"
	shortest_path_enable           = "shortest_path_enable"
	destination_position           = "destination_position"
)

var driveStatusUpdated chan bool

// waitForDriveStatusUpdate is set by notifyDriverStatusWithWait to signal
// doneDriveStatusUpdate that the caller is blocking on driveStatusUpdated.
// Must be atomic: written by listenDriverAction goroutine, read by
// listenDriverStatus goroutine (via doneDriveStatusUpdate).
var waitForDriveStatusUpdate atomic.Bool

func doneDriveStatusUpdate() {
	if waitForDriveStatusUpdate.Load() {
		driveStatusUpdated <- true
	}
}

// isStatusListening is true while the listenDriverStatus goroutine is running.
// Must be atomic: written by listenDriverStatus goroutine, read by
// notifyDriverStatus and notifyDriverStatusWithWait from other goroutines.
// A stale false-read causes notifyDriverStatusWithWait to deadlock waiting on
// driveStatusUpdated with nobody to send on it.
var isStatusListening atomic.Bool

//driverStatusMap will keep the status of each driver configured, for e.g. driverStatusMap["A"] contains all the
//status of drive A
// var driverStatusMap map[string]driverCurrentStatus
var driverStatusMap cmap.ConcurrentMap

func initDriverStatusKeeperListener() {
	logger.Debug("initialize driver status listener")
	driverStatusMap = cmap.New()

	// driverStatusMap = make(map[string]driverCurrentStatus)
	for _, dev := range masterDevices {
		settings := settings.GetDriverSettings(dev.Name)
		driveStatus := getCurrentDriverStatus(dev.Name)
		driveStatus.backlashInSetting = settings.BackLash
		driveStatus.isMotorRunning = false
		setCurrentDriverStatus(dev.Name, driveStatus)
	}
	startDriverStatusListener()
}

func startDriverStatusListener() {
	logger.Debug("start driver status listener")
	channels.BroadCastDriveStatusChannel = make(chan channels.DriverStatus, 1000)
	driveStatusUpdated = make(chan bool)
	isStatusListening.Store(false)
	go listenDriverStatus()
}

func stopDriveStatusListener() {
	driverStatus := channels.DriverStatus{Event: "exit"}
	channels.BroadCastDriveStatusChannel <- driverStatus
}

// getCurrentDriverStatus returns the current status for the named drive.
// Returns a zero-value status if the drive has not been registered yet.
func getCurrentDriverStatus(driveName string) driverCurrentStatus {
	if stat, ok := driverStatusMap.Get(driveName); !ok {
		return driverCurrentStatus{currentPosition: 0, alarm: "", isMotorRunning: false, isDriverOnOff: false}
	} else {
		return stat.(driverCurrentStatus)
	}
}

func setCurrentDriverStatus(driveName string, currentStatus driverCurrentStatus) {
	driverStatusMap.Set(driveName, currentStatus)
}

func currentDriverPosition(device MasterDevice, currPos float64) {
	driverStatus := channels.DriverStatus{DriveName: device.Name, Data: fmt.Sprintf("%.3f", currPos), Event: "current_position"}
	channels.BroadCastDriveStatusChannel <- driverStatus
}

func notifyDriverStatus(event EventType, data string, device MasterDevice) {
	if !isStatusListening.Load() {
		return
	}
	// NOTE: Do NOT touch waitForDriveStatusUpdate here.
	// Resetting it to false would silently kill any concurrent
	// notifyDriverStatusWithWait caller that is mid-flight waiting
	// on <-driveStatusUpdated — the listener would never send on that
	// channel and the caller would stall indefinitely.
	driverStatus := channels.DriverStatus{DriveName: device.Name, Data: data, Event: string(event)}
	channels.BroadCastDriveStatusChannel <- driverStatus
}

//notifyDriverStatusWithWait callers can call this function if they need to ensure the driver status is updated
//before moving to the next step. For e.g. setting backlash, the caller should move forward only after successfully
//set the backlash other wise the system behave incorrectly
func notifyDriverStatusWithWait(event EventType, data string, device MasterDevice) {
	if !isStatusListening.Load() {
		return
	}
	waitForDriveStatusUpdate.Store(true)
	driverStatus := channels.DriverStatus{DriveName: device.Name, Data: data, Event: string(event)}
	channels.BroadCastDriveStatusChannel <- driverStatus
	<-driveStatusUpdated
	waitForDriveStatusUpdate.Store(false)
}

func listenDriverStatus() {
	isStatusListening.Store(true)
	for {
		msg := <-channels.BroadCastDriveStatusChannel
		switch msg.Event {
		case "current_position":
			pos, _ := strconv.ParseFloat(msg.Data, 64)
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.currentPosition = pos
			setCurrentDriverStatus(msg.DriveName, driverStatus)
		case "pot_not_exceeded":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.potNotExceeded = true
			driverStatus.notExceeded = false
			driverStatus.potExceeded = false
			if msg.Data == "POT" {
				driverStatus.potExceeded = true
			} else {
				driverStatus.notExceeded = true
			}
			setCurrentDriverStatus(msg.DriveName, driverStatus)
		case "mode":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.mode = msg.Data
			logger.Debug("changing driver mode to ", msg.Data)
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriverAction()
			doneDriveStatusUpdate()
		case "shortest_path_enable":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			shortestPathEnbl, _ := strconv.ParseBool(msg.Data)
			driverStatus.shortestPathEnabled = shortestPathEnbl
		
			// IMPORTANT:
			// G68 (shortest path) must NOT override ABS/REL mode.
			// Mode should only be controlled by G90/G91.
			logger.Debug("Shortest path set to ", shortestPathEnbl, " (rotation mode unchanged: ", driverStatus.mode, ")")
		
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriverAction()
			doneDriveStatusUpdate()
		case "destination_position":
			pos, _ := strconv.ParseFloat(msg.Data, 64)
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.destinationPosition = pos
			setCurrentDriverStatus(msg.DriveName, driverStatus)
		case "reset":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.reset()
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			logger.Trace("reset driver status")
		case "rotation_direction":
			driverStatusDir := getCurrentDriverStatus(msg.DriveName)
			dir, _ := strconv.ParseInt(msg.Data, 10, 32)
			if driverStatusDir.direction != int(dir) {
				driverStatusDir.potNotExceeded = false
			}
			driverStatusDir.backlash = 0
			//current dir is CW but new dir is counter clockwise, so apply backlash
			if driverStatusDir.direction > 0 && dir <= 0 {
				driverStatusDir.backlash = driverStatusDir.backlashInSetting
			}
			//driver continue moving to CCW, set backlash to set value.
			//As per Dinesh response on Sept-25-2021 in google chat
			/*
				As per the mechanical change the backlash will be applicable for the first position when direction changes from CW to CCW  but for the electrical calculation, for every position the backlash value should be consider when running in CCW direction. since we are getting feed back from absolute encoder. Calculation: 1. Clockwise position = Destination + pitch error (cumulative value), 2. Counter clockwise position = Destination + Pitch error (Cumulative value) + Backlash.
			*/
			if driverStatusDir.direction <= 0 && dir <= 0 {
				driverStatusDir.backlash = driverStatusDir.backlashInSetting
			}
			// //driver moving to CW from CCW, set backlash to 0
			// if driverStatus.direction < 0 && dir > 0 {
			// 	driverStatus.backlash = 0
			// }
			driverStatusDir.direction = int(dir)
			setCurrentDriverStatus(msg.DriveName, driverStatusDir)
			doneDriveStatusUpdate()
		case "set_backlash":
			backlash, _ := strconv.ParseFloat(msg.Data, 64)
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.backlash = backlash
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriveStatusUpdate()
		case "workoffset":
			workOffset, _ := strconv.ParseFloat(msg.Data, 64)
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverStatus.workOffset = workOffset
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriveStatusUpdate()
		case "program_mode":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			progMode, _ := strconv.ParseBool(msg.Data)
			driverStatus.isProgramMode = progMode
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriveStatusUpdate()
		case "motor_running":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			motorRunning, _ := strconv.ParseBool(msg.Data)
			driverStatus.isMotorRunning = motorRunning
			setCurrentDriverStatus(msg.DriveName, driverStatus)
		case "fin_signal":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			finSignal, _ := strconv.ParseBool(msg.Data)
			driverStatus.isSendingFinSignal = finSignal
			setCurrentDriverStatus(msg.DriveName, driverStatus)
			doneDriveStatusUpdate()
		case "driver_on_off":
			driverStatus := getCurrentDriverStatus(msg.DriveName)
			driverOn, _ := strconv.ParseBool(msg.Data)
			driverStatus.isDriverOnOff = driverOn
			// driverStatus.isMotorRunning = driverOn
			setCurrentDriverStatus(msg.DriveName, driverStatus)
		case "exit":
			logger.Debug("stopping driver status keeper")
			isStatusListening.Store(false)
			return
		}
	}

}