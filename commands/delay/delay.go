package main

import (
	h "EtherCAT/commands"
	dt "EtherCAT/datatypes"
	"strconv"
	"time"
)

/*
Introduce a delay before the next command is executed. D command can be used for
creating a delay in part program.

Delay is in seconds
*/

//CommandHandler type def
type CommandHandler struct {
}

//CreateHandler when line starts with R then loop starts
func CreateHandler() h.Handler {
	return CommandHandler{}
}

//Handle generic exec
func (d CommandHandler) Handle(cmd dt.Command, execContext *dt.ExecutionContext) []dt.ExecutionResult {
	delay, err := cmd.GetValueAsInt()
	results := []dt.ExecutionResult{}
	if !execContext.TrialModeActive {
		time.Sleep(time.Duration(delay) * time.Second)
		result := dt.ExecutionResult{Description: "Delay for " + strconv.Itoa(delay) + " seconds"}
		results = append(results, result)
	}
	execContext.Err = err
	execContext.MoveNextLine()
	return results
}

//CommandName name of the command
func (d CommandHandler) CommandName() string {
	return "delay"
}

//Command used to lookup for plugins
var Command CommandHandler
