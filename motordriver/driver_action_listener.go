package motordriver

//Listen for any action request on Motor to perform from the client. For e.g. reset, zero reference, etc

import (
	channels "EtherCAT/channels"
	logger "EtherCAT/logger"
	"EtherCAT/motordriver/statusnotifier"
	//"EtherCAT/ren"
	//"time"
	//"fmt"
	"strconv"
)

// rotation_direction is a package-level variable used to store the last jog direction.
// This declaration resolves the "syntax error: unexpected keyword else" issue.
var rotation_direction int 

func initDriverActionListener() {
	logger.Debug("starting driver action listener")
	channels.DriverActionChannel = make(chan channels.DriverAction, 100)
	channels.DriveActionChannelReady()
	//listen for any action to takes on driver.
	go listenDriverAction()
}

func stopDriverActionListener() {
	channels.NotifyMotorDriver("EXIT_DRIVE_LISTENER", "", "", 0)
}

/**
function listen to channel DriverActionChannel. Any one can write actions to this channel and this function
will channel the action to the motor driver.
**/
func listenDriverAction() {
	masterDevices := getMasterDevices()
	device := masterDevices[0]
	for {
		msg := <-channels.DriverActionChannel

		switch msg.Action {
		case channels.RESET:
			performSysReset(true)
			// if !HasDriverConnected() {
			//  InitMaster()
			// }
			// ResetDriver(masterDevices)
		case channels.MANUAL_JOG:
			// Store the direction globally so STOP_JOG can reference it.
			rotation_direction = msg.Direction 
			notifyDriverStatusWithWait("rotation_direction", strconv.Itoa(msg.Direction), *device)
			ManualJog(device, msg.Direction)
		case channels.STOP_JOG:
			// StopJog internally ramps velocity to 0, then waits 250ms for the
			// motor to physically decelerate before disabling jog mode.
			// No ResetDriver/PDOFaultReset here — the drive is NOT faulted after
			// a normal jog stop (statusword bit3=0, confirmed in every stop log).
			// PDOFaultReset takes 500-600ms and is unnecessary; calling it after
			// every jog stop was the root cause of the ~800ms total stop latency.
			StopJog(device)
		case channels.ZERO_REF:
			go moveToZero(device)
		case channels.STEP_MODE_ENABLE:
			logger.Debug("step mode enabled - configureDriver bypassed")
			//configureDriver(*device)
		case channels.STEP_MODE:
			pos, _ := strconv.ParseFloat(msg.Value, 64)
			notifyDriverStatusWithWait("rotation_direction", strconv.Itoa(msg.Direction), *device)
			stepMode(device, pos)
		case channels.SET_RPM:
			rpm, _ := strconv.ParseInt(msg.Value, 0, 32)
			setRpm(device, int(rpm))
		case channels.MOVE_TO_POSITION:
			degree, _ := strconv.ParseFloat(msg.Value, 64)
			//run moveMotorToDegree as go routine, so that this listener can continue listening other events
			//such as emergency etc.
			go moveMotorToDegree(device, degree)
		case channels.START_EXECUTION:
			logger.Trace("program exec started")
			notifyDriverStatus("reset", "", *device)
		case channels.POSITION_MODE:
			notifyDriverStatus("mode", msg.Value, *device)
		case channels.SHORTEST_PATH_ENABLED:
			notifyDriverStatus("shortest_path_enable", msg.Value, *device)
		case channels.EMERGENCY:
			stopECSCheck()
			emergency(*device)
			PowerOffAll(masterDevices)
			statusnotifier.SocketMessage("emergency_done", "emergency completed")
			statusnotifier.Alarm("Software Emergency pressed")
		case channels.PROGRAM_EXEC_COMPLETED:
			logger.Trace("program exec completed")
			notifyDriverStatus("reset", "", *device)
		case channels.FAST_POWER_OFF:
			FastPowerOff(*device)
		case channels.RESET_MULTI_TURN:
			if err := resetMultiTurn(masterDevices); err != nil {
				logger.Error("[RESET_MULTI_TURN] failed:", err)
			}
		case channels.SET_WORK_OFFSET:
			notifyDriverStatusWithWait("workoffset", msg.Value, *device)
		case channels.SETTINGS_CHANGED:
			applyClampIfSettingsChanged()
		case channels.STOP_PROGRAM_EXECUTION:
			// DESIGN: "stop after completing current command"
			//
			// The user expects the motor to finish its current move and THEN stop —
			// not to freeze mid-rotation. This matches the behaviour of the old SDO
			// version: the executor sets its internal stop flag, the running
			// moveMotorToDegree goroutine completes hasTargetReached → sends fin
			// signal → calls doneDriverAction(), then the executor checks its stop
			// flag and does not start the next line.
			//
			// What we must NOT do: cancel the active PDO position move by calling
			// EnablePosPDO(false) + SetTargetPositionPDO(current). That was the bug
			// that caused the motor to freeze mid-move (log showed stop at 284.358°
			// when target was 270°, and 249.159° when target was 0°).
			//
			// What we MUST do:
			//   1. stopECSCheck() — unblock any goroutine waiting in the ECS loop
			//      so it can return to the executor which then reads the stop flag.
			//   2. StopJog if jog is active — a G01 Fxx command enables PDO jog
			//      and must be stopped immediately (continuous velocity, no target).
			//   3. Leave PdoPosEnabled alone — hasTargetReached() is polling bit10;
			//      it will return naturally when the drive reaches its target, then
			//      the executor stops before the next line.
			stopECSCheck()
			if IsPDOActive() && device.PdoReady {
				if device.IsJogEnabled() {
					// Jog (continuous velocity) has no natural end — stop it now.
					_ = StopJog(device)
					_ = device.SetTargetVelocityPDO(0)
					logger.Info("[PDO] STOP_PROGRAM_EXECUTION: jog stopped immediately")
				}
				// Position move: intentionally NOT cancelled.
				// The move completes naturally; executor halts after this line.
				if device.IsPosEnabled() {
					logger.Info("[PDO] STOP_PROGRAM_EXECUTION: position move allowed to complete before stopping")
				}
			}
		case channels.EXIT_DRIVE_LISTENER:
			// 'return' exits the goroutine entirely.
			// 'break' would only exit the switch, leaving the for loop running
			// forever and leaking this goroutine on every reset.
			logger.Debug("stopping driver action listener")
			return
		default:
			logger.Error("listenDriverAction->unrecognized driver action type passed", msg.Action)
		}
	}
}

//doneDriverAction will feedback the command handlers about the completion of a command
//for eg. move to 90 degree, once move completed will feed back the handler that its done.
//So than command handlers can execute the next line of command
func doneDriverAction() {
	channels.NotifyCmdComplete()
}