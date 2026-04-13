package executors

import (
	channels "EtherCAT/channels"
	dt "EtherCAT/datatypes"
	logger "EtherCAT/logger"
)

/*
This listener will listen to input commands to command executors when in the middle of executing a program
For e.g. user can change the execution to single or continuous. Send emergency stop or stop execution of the program etc.
*/

func listenCommandExecInput(execContext *dt.ExecutionContext) {
	channels.CommandExecInputChannel = make(chan channels.CommandExecInput, 10)
	go func(execContextToModify *dt.ExecutionContext) {
		for {
			msg := <-channels.CommandExecInputChannel
			switch msg.InputType {
			case "command_exec_mode":
				execContextToModify.ExecutionMode = msg.Data
				logger.Trace("change command exec mode to ", msg.Data)
			case "move_next_line":
				// execContextToModify.WaitExecuteNextCmd = false
				channels.NotifySingleModeComplete()
				logger.Trace("move to next line commanded from ui")
			case "stop_prog_exec":
				logger.Trace("stop_prog_exec received")
				execContextToModify.StopExecution = true
				channels.NotifyCmdComplete()
				driverAction := channels.DriverAction{Action: "STOP_PROGRAM_EXECUTION"}
				channels.DriverActionChannel <- driverAction
			case "reset":
				execContextToModify.HasResetted = true
				execContextToModify.Reset()
				channels.NotifyCmdComplete()
			case "waiting_for_ecs":
				execContextToModify.WaitingForECS = true
			case "ecs_done":
				execContextToModify.WaitingForECS = false
			
			}
		}
	}(execContext)
}
