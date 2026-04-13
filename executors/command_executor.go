package executors

import (
	channels "EtherCAT/channels"
	h "EtherCAT/commands"
	parsers "EtherCAT/configparser"
	dt "EtherCAT/datatypes"
	"EtherCAT/licensechecker"
	logger "EtherCAT/logger"
	"EtherCAT/settings"
	motor "EtherCAT/motordriver"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"strconv"
	 "sync/atomic"
	"encoding/json"
	"time"
)

var funcMap map[string]h.Handler
var yamlConfig dt.YamlConfig
var hasYmlLoaded bool
var execContext dt.ExecutionContext
var RS232Enabled atomic.Bool

const lastLineFile = "/mnt/app/jamun/settings/last_line.txt"

func SetRS232Enabled(enable bool) {
    RS232Enabled.Store(enable)
}

func IsRS232Enabled() bool {
    return RS232Enabled.Load()
}

//Initialize executors to load plugins and parse execution yaml file.
func Initialize() error {
	//Load command plugins from plugins folder
	var err error
	funcMap, err = loadCommandPlugins()
	if err != nil {
		return err
	}
	//yml config of the function/command mapping
	_, err = loadYmlConfig()
	if err != nil {
		channels.SendAlarm(err.Error())
		return err
	}

	execContext.CommandMaps = yamlConfig.Execution
	execContext.ExecutionMode = "continuous"
	listenCommandExecInput(&execContext)

	// Only send "No Alarms" if no drive fault has been detected yet.
	//
	// ROOT CAUSE OF ALARM BEING INVISIBLE ON GUI:
	//
	// InitMaster() (called before Initialize()) starts the PDO cyclic task and
	// the error poller. If the drive has a stale fault from a previous session
	// (e.g. Err80.4 "ESM unauthorized request error protection"), the poller
	// fires statusnotifier.Alarm("ESM unauthorized request error protection")
	// within ~400ms of PDO activation.
	//
	// Initialize() runs ~800ms after PDO activation — AFTER the fault alarm has
	// already been sent to the UI. The unconditional SendAlarm("No Alarms") here
	// overwrites the fault alarm on every connected client, making the user think
	// everything is fine when the drive is actually faulted.
	//
	// Fix: only send "No Alarms" if the current alarm state is still "No Alarms"
	// (i.e. no fault was detected during InitMaster). If a fault alarm is active,
	// preserve it so the user sees it when they open the web page.
	if motor.GetCurrentAlarm() == "No Alarms" {
		channels.SendAlarm("No Alarms")
	} else {
		logger.Info("[EXECUTOR] Skipping 'No Alarms' — active alarm:", motor.GetCurrentAlarm())
	}
	UpdateLastLineFromJSON()
	lastLine, err := readLastLine()
    if err != nil {
		logger.Error("Failed to read last_line.txt:", err)
    } else {
		execContext.NextLineWhenStopped = lastLine 
		if lastLine > 0 {
			logger.Info("Successfully read last_line.txt. Resuming from line: ", lastLine)
			execContext.NextLineWhenStopped = lastLine
        }
    }
	execContext.Reset()
	return nil
}

func loadYmlConfig() (dt.YamlConfig, error) {
	if hasYmlLoaded == false {
		var err error
		yamlConfig, err = parsers.ParseExececutionConfigYML()
		if err != nil {
			return yamlConfig, err
		}
		hasYmlLoaded = true
	}
	return yamlConfig, nil
}

func validateLicenseBeforeRun() error {
	licErr := licensechecker.CheckLicense(false)
	if licErr != nil {
		channels.SendAlarm(licErr.Error())
		return licErr
	}
	return nil
}

func setDriveSettings() {
	allSettings := settings.GetAllSettings()
	execdriveSettings := make(map[string]dt.DriveSetting)
	for k, v := range allSettings {
		setting := dt.DriveSetting{POTLimit: v.POT, NOTLimit: v.NOT, ConfiguredWorkOffset: v.GetWorkOffset()}
		execdriveSettings[k] = setting
	}
	execContext.DriveSettings = execdriveSettings
}

func CompileProgram(fileName string) error {
	commands, err := createCommands(fileName)
	if err != nil {
		return err
	}
	errVerify := canExecuteGivenCommands(commands)
	if errVerify != nil {
		return errVerify
	}
	return nil
}

func RunCodeFile(fileName string) error {
	file := filepath.Base(fileName)
	logger.Info("executing program file", file)

	channels.SendAlarm("No Alarms")
	if licErr := validateLicenseBeforeRun(); licErr != nil {
		return licErr
	}
	execContext.PrepareExecutingFile(fileName)
	execContext.ExecutingFilePath = fileName
	commands, err := createCommands(fileName)
	if err != nil {
		return err
	}

	execContext.Commands = commands

	setDriveSettings()
	drvSettings := settings.GetDriverSettings("A")
	execContext.ECSEnabled = drvSettings.ECS
	execContext.StopExecution = false
	panicAfter = 0

	if execContext.NextLineWhenStopped <= 0 {
		channels.NotifyMotorDriver(channels.START_EXECUTION, "", "", 0)
		ResetExecutingProgram()
		UpdateLastLineFromJSON()
		lastLine, err := readLastLine()
		if err != nil {
			logger.Error("Failed to read last_line.txt:", err)
		} else {
			execContext.NextLineWhenStopped = lastLine
			if lastLine > 0 {
				logger.Info("Successfully read last_line.txt. Resuming from line: ", lastLine)
				execContext.NextLineWhenStopped = lastLine
			}
		}
		errVerify := canExecuteGivenCommands(commands)
		if errVerify != nil {
			channels.SendAlarm(errVerify.Error())
			return errVerify
		}
	}
	
	logger.Trace("executing commands from", execContext.NextLineWhenStopped)
	execErr := executeCommands(commands, execContext.NextLineWhenStopped)
	if !execContext.StopExecution {
		channels.NotifyMotorDriver(channels.PROGRAM_EXEC_COMPLETED, "", "A", 0)
		ResetExecutingProgram()
	}
	channels.NotifyUIProgramCompleted()
	return execErr
}

func ResetExecutingProgram() {
	execContext.StopExecution = true
	execContext.Reset()
	err := os.WriteFile(lastLineFile, []byte("0"), 0644)
	if err != nil {
		logger.Error("Failed to save execution state to last_line.txt:", err)
	}
	logger.Info("PROGRAM RESET IS INITIATED.")
}

func canExecuteGivenCommands(commands []dt.Command) error {
	execContext.Reset()
	execContext.ActivateTrialMode()
	logger.Info("running in trial mode to verify the commands")
	execVerifyErr := executeCommands(commands, 0)
	if execVerifyErr != nil {
		return execVerifyErr
	}
	logger.Info("trial run completed, commands ok.")
	execContext.DeActivateTrialMode()
	UpdateLastLineFromJSON()
	lastLine, err := readLastLine()
    if err != nil {
		logger.Error("Failed to read last_line.txt:", err)
    } else {
		execContext.NextLineWhenStopped = lastLine
		if lastLine > 0 {
			logger.Info("Successfully read last_line.txt. Resuming from line: ", lastLine)
			execContext.NextLineWhenStopped = lastLine
        }
    }
	execContext.Reset()
	return nil
}

func saveLastLine(lineNumber int) {
	lineStr := strconv.Itoa(lineNumber)
	err := os.WriteFile(lastLineFile, []byte(lineStr), 0644)
	if err != nil {
		logger.Error("Failed to save execution state to last_line.txt:", err)
	}
}

func readLastLine() (int, error) {
    content, err := os.ReadFile(lastLineFile)
    if err != nil {
        if os.IsNotExist(err) {
            return 0, nil
        }
        return 0, err
    }

    lineStr := string(content)
    lineNumber, err := strconv.Atoi(lineStr)
    if err != nil {
        return 0, err
    }
    return lineNumber, nil
}

// 

func executeCommands(commands []dt.Command, nextCommandIndex int) error {
    if execContext.StopExecution {
        logger.Info("Execution stopped. Unwinding command calls...")
        return nil
    }

    if nextCommandIndex < 0 || nextCommandIndex >= len(commands) {
        logger.Trace("command execution completed", nextCommandIndex)
        return nil
    }

    if !execContext.TrialModeActive {
        saveLastLine(nextCommandIndex)
    }

    waitForNextBlock, err := executeCommand(commands[nextCommandIndex], &execContext, nextCommandIndex)
    if err != nil {
        return err
    }

    // Only reload from disk when RS232 mode is active — in that mode an external
    // tool may rewrite the file between commands. In standard (non-RS232) mode the
    // file never changes during execution, so re-reading it on every command is
    // pure filesystem overhead (was causing 12 disk reads per program loop).
    if IsRS232Enabled() && execContext.ExecutingFilePath != "" {
        updatedCommands, err := createCommands(execContext.ExecutingFilePath)
        if err == nil {
            commands = updatedCommands
            execContext.Commands = updatedCommands
            logger.Debug("Program file reloaded, total commands:", len(commands))
        } else {
            logger.Warn("Could not reload updated program file:", err)
        }
    }

    if waitForNextBlock && !execContext.TrialModeActive {
		if IsRS232Enabled() {
			// 1. Explicitly save the current index before waiting for the new file update
			nextIdx := nextCommandIndex + 1
            execContext.NextLineWhenStopped = nextIdx
            saveLastLine(nextIdx)
			logger.Info("[RS232] Blocking execution: waiting for file update...")
        
           // 2. Blocks the routine until the file is modified (external RS232 command writes new file)
           waitForProgramFileUpdate(execContext.ExecutingFilePath)
        
           // 3. FORCE reload of the commands so the NEXT iteration uses the NEW file content
           updatedCommands, err := createCommands(execContext.ExecutingFilePath)
           if err == nil {
               commands = updatedCommands
               execContext.Commands = updatedCommands
               // Sync the context pointer to ensure recursion picks up the new command list
               execContext.NextCmdLineToExec = nextCommandIndex + 1
               logger.Debug("Program file reloaded after RS232 update")
           }
        } else {
            logger.Debug("[EXECUTOR] RS232 disabled → standard wait")
            motor.RefreshCurrentPosition()
            execContext.NextLineWhenStopped = nextCommandIndex + 1
            execContext.WaitExecuteNextCommand()
        }
    }

    if execContext.StopExecution {
        if execContext.HasResetted {
            execContext.NextLineWhenStopped = 0
        } else if execContext.WaitingForECS {
            execContext.NextLineWhenStopped = nextCommandIndex
        } else {
            execContext.NextLineWhenStopped = nextCommandIndex + 1
        }

        logger.Debug("Stopping execution, next resume line will be #", execContext.NextLineWhenStopped)

        if !execContext.TrialModeActive {
            saveLastLine(execContext.NextLineWhenStopped)
        }
        return nil
    }

    return executeCommands(commands, execContext.NextCmdLineToExec)
}

func executeCommand(cmd dt.Command, execContext *dt.ExecutionContext, currentCmdIndex int) (bool, error) {
	commandToExec := yamlConfig.Execution.GetCommand(cmd.Cmd)
	cmd.Description = commandToExec.Description
	execContext.CurrentExecCommandLine = currentCmdIndex
	funcToExec := funcMap[commandToExec.Func]
	err := isAValidHandler(funcToExec, cmd.Cmd)
	if err != nil {
		return false, err
	}
	if funcToExec != nil {
		if !execContext.TrialModeActive {
			logger.Info("executing line:", cmd.CodeLineNumber, "command:", cmd.Cmd)
			//for i := 0; i < 4; i++ {
			channels.SendLineNumber(cmd.CodeLineNumber)
				//time.Sleep(time.Duration(10) * time.Millisecond)
			//}
		}
		cmd.ConsiderInBlockExecution = commandToExec.ConsiderInBlockExecution
		results := funcToExec.Handle(cmd, execContext)
		if execContext.Err != nil {
			logger.Error("Error executing command " + cmd.Cmd)
			logger.Error(execContext.Err)
			return false, execContext.Err
		}
		execLineErr := executeInLineParamFunction(funcToExec, cmd, execContext, results)
		if execLineErr != nil {
			return false, execLineErr
		}
	}
	waitForNextBlock := false
	if commandToExec.ConsiderInBlockExecution == 1 {
		waitForNextBlock = true
	}
	return waitForNextBlock, nil
}

func isAValidHandler(funcToExec h.Handler, funcName string) error {
	if funcName == "invalidCommand" {
		return errors.New("Unable to find command processor for " + funcName)
	}
	if funcToExec == nil || (reflect.ValueOf(funcToExec).Kind() == reflect.Ptr && reflect.ValueOf(funcToExec).IsNil()) {
		return errors.New("Unable to find command processor for " + funcName)
	}
	return nil
}

var panicAfter int

func executeInLineParamFunction(funcToExec h.Handler, cmd dt.Command, execContext *dt.ExecutionContext, returnedResults []dt.ExecutionResult) error {
	if len(returnedResults) <= 0 {
		return nil
	}

	for _, result := range returnedResults {
		if result.ShouldExecute {
			paramToExec := funcMap[result.Cmd.Func]
			if paramToExec != nil {
				childResults := paramToExec.Handle(result.Cmd, execContext)
				if execContext.Err != nil {
					logger.Error("Error executing command " + cmd.Cmd)
					logger.Error(execContext.Err)
					return execContext.Err
				}
				if result.Cmd.ConsiderInBlockExecution == 1 {
					execContext.WaitExecuteNextCommand()
				}
				return executeInLineParamFunction(paramToExec, result.Cmd, execContext, childResults)
			}
		}
	}
	return nil
}

func ResumeExecution() error {
	if execContext.NextLineWhenStopped == 0 {
		return nil
	}

	execContext.StopExecution = false
	logger.Info("resuming execution from line ", execContext.NextLineWhenStopped)
	return executeCommands(execContext.Commands, execContext.NextLineWhenStopped)
}

func UpdateLastLineFromJSON() {
    type UserLine struct {
        UserLine string `json:"user_line"`
    }

    data, err := os.ReadFile("/mnt/app/jamun/settings/userline.json")
    if err != nil {
        logger.Error("RefreshLineFromJSON: failed to read userline.json:", err)
        return
    }

    var ul UserLine
    if err := json.Unmarshal(data, &ul); err != nil {
        logger.Error("RefreshLineFromJSON: failed to parse userline.json:", err)
        return
    }

    cleanLine := strings.TrimSpace(ul.UserLine)
    if cleanLine == "" {
        logger.Warn("RefreshLineFromJSON: empty user_line in JSON")
        return
    }

    if err := os.WriteFile(lastLineFile, []byte(cleanLine), 0644); err != nil {
        logger.Error("RefreshLineFromJSON: failed to update last_line.txt:", err)
        return
    }

    lastLine, err := readLastLine()
    if err != nil {
        logger.Error("RefreshLineFromJSON: failed to re-read last_line.txt:", err)
        return
    }

    execContext.NextLineWhenStopped = lastLine
    logger.Info("🔄 Refreshed execContext.NextLineWhenStopped from userline.json:", lastLine)
}

func waitForProgramFileUpdate(filePath string) {
    info, err := os.Stat(filePath)
    if err != nil {
        logger.Warn("Could not stat file for updates:", err)
        return
    }
    lastMod := info.ModTime()

    for {
        time.Sleep(200 * time.Millisecond)
        info, err := os.Stat(filePath)
        if err != nil {
            logger.Warn("Could not stat file for updates:", err)
            return
        }
        if info.ModTime().After(lastMod) {
            logger.Info("Detected program file update:", filePath)
            break
        }
    }
}