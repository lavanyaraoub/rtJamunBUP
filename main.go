package main

/*
#cgo CFLAGS: -g -Wall -I/opt/etherlab/include
#cgo LDFLAGS: /home/pi/gosrc/src/EtherCAT/libethercatinterface.so
*/
import "C"
import (
	socket "EtherCAT/clientcommunication"
	"EtherCAT/constants"
	executors "EtherCAT/executors"
	"EtherCAT/helper"
	"EtherCAT/channels"
	"EtherCAT/hotspot"
	"EtherCAT/licensechecker"
	"EtherCAT/logger"
	 //"runtime"
	motor "EtherCAT/motordriver"
	"EtherCAT/restapi"
	settings "EtherCAT/settings"
	"EtherCAT/systemupdate"
	"EtherCAT/tunnel"
	//"EtherCAT/ren"
	"EtherCAT/webserver"
	"EtherCAT/serialtest"
	"bytes"
	"time"
	"fmt"
	_ "net/http/pprof"
    //"net/http"
    //"log"
	"os"
	"os/exec"
	"github.com/goburrow/modbus"
	"os/signal"
	"strings"
	"EtherCAT/gpiohandle"
	"syscall"
	//"encoding/binary"
	"text/tabwriter"
	
)

//Version of the program
var Version = constants.SystemVersion

//RelChannel release channel of the program
var RelChannel = "v1"

//LogLevel tells what level of logs should be logged
var LogLevel = "TRACE"

var BuildTime string

var wait chan bool

func main() {
	envSetting := settings.GetEnvSettings()
	LogLevel = envSetting.LogLevel
	RelChannel = envSetting.ReleaseChannel
	logger.Init(LogLevel)
    // ---- RS232: restore persisted toggle state on boot ----
    rsData, loadErr := settings.LoadRS232Data()
	if loadErr != nil {
		logger.Error("Failed to load RS232 status from disk:", loadErr)
		} else {
			if rsData == "1" {
				executors.SetRS232Enabled(true)
				logger.Info("RS232 status restored: 1 (ENABLED)")
				} else {
					executors.SetRS232Enabled(false)
					logger.Info("RS232 status restored: 0 (DISABLED)")
				}
			}

	if (len(os.Args)) > 1 {
		sysArg := strings.ToLower(os.Args[1])
		exit := processSytemParm(sysArg)
		if exit {
			return
		}
	}

	logger.Info("-------------------------------------------------------------------------------------")
	logger.Info("Starting Ethercat Driver Controller ...")
	logger.Info("Version:", Version)
	logger.Info("Build time:", BuildTime)
	logger.Info("Release channel:", RelChannel)
	logger.Info("Log level: ", LogLevel)

	// 🔹 Start monitoring mani.txt from PC
   //ren.StartMonitor()

	if envSetting.HotSpotOnStart {
		logger.Info("starting hotspot")
		h := hotspot.NewHotspot()
		h.Create()
	}
	setupCloseHandler()

	_ = settings.LoadDriverSettings()
	logger.Info("loaded driver settings")

	go socket.Start()

	licErr := licensechecker.CheckLicense(true)
	if licErr != nil {
		logger.Error(licErr)
		return
	}
	
	
	
	err := motor.InitMaster()
	if err != nil {
		logger.Error(err)
	}
	
	execErr := executors.Initialize()
	
	if execErr != nil {
		logger.Error(execErr)
		logger.Error("Exiting the system ...........")
		os.Exit(0)
	}


	w := webserver.NewWebUI()
	go w.Start(envSetting.UIVer)

	a := restapi.NewApi()
	go a.Start()

	go runStartScript()
	
	 // 2️⃣ Start RS-232 listener (NON-BLOCKING)
	 serialtest.StartSerialListener()

	//go func() {
	//	log.Println("pprof on :6060")
//
//		http.ListenAndServe("0.0.0.0:6060", nil)
//	}()

//	go func() {
//		for {
//			log.Println("GOROUTINES:", runtime.NumGoroutine())
//			time.Sleep(5 * time.Second)
//		}
//	}()

	    // 🔹 START GPIO HANDLER HERE
	//go startGPIOHandler()

	//go startModbusHandler()

	//wait for done signal from closeHandler
	<-wait
}

func processSytemParm(sysParm string) bool {
	if sysParm == "-h" {
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "-v\t", "Display version and other environment settings")
		fmt.Fprintln(w, "-s\t", "Check for system update and download if any update available")
		fmt.Fprintln(w, "-u\t", "Update the system to latest version")
		fmt.Fprintln(w, "-c\t", "Remove log, license and backup files")
		fmt.Fprintln(w, "-l\t", "Download license file")
		fmt.Fprintln(w, "--hotspot-down\t", "Kill hotspot")
		fmt.Fprintln(w, "--hotspot-up\t", "Create hotspot")
		fmt.Fprintln(w, "--tunnel-up\t", "Create remote tunnel")
		fmt.Fprintln(w, "--tunnel-down\t", "Stop remote tunnel")
		fmt.Fprintln(w, "--pdo-pos\t", "Print drive position using TxPDO (0x6064:0) without altering normal flow")

		w.Flush()
		return true
	} else if sysParm == "-v" {
		s, _ := helper.ReadSerialNumber()
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "Version\t:", Version)
		fmt.Fprintln(w, "Release channel\t:", RelChannel)
		fmt.Fprintln(w, "Loglevel\t:", LogLevel)
		fmt.Fprintln(w, "Serial#\t:", s)
		w.Flush()
		return true
	} else if sysParm == "-s" {
		systemupdate.CheckforUpdates(false)
		return true
	} else if sysParm == "-u" {
		systemupdate.PerformSystemUpdate(false)
		return true
	} else if sysParm == "-l" {
		os.Remove(helper.AppendWDPath("/license.key"))
		os.Remove(helper.AppendWDPath("/license.lic"))
		licErr := licensechecker.CheckLicense(true)
		if licErr != nil {
			logger.Error(licErr)
		}
		return true
	} else if sysParm == "-c" {
		fmt.Println("Starting file cleanup...")
		fmt.Println("Remove log files")
		os.RemoveAll(helper.AppendWDPath("/log"))
		fmt.Println("Remove license file")
		os.Remove(helper.AppendWDPath("/license.key"))
		os.Remove(helper.AppendWDPath("/license.lic"))
		fmt.Println("Remove system backup files")
		os.RemoveAll("/home/pi/jamun_backup")
		fmt.Println("File cleanup completed")
		return true
	} else if sysParm == "--hotspot-down" {
		h := hotspot.NewHotspot()
		h.Kill()
		return true
	} else if sysParm == "--hotspot-up" {
		h := hotspot.NewHotspot()
		h.Create()
		return true
	} else if sysParm == "--tunnel-up" {
		t := tunnel.NewTunnel()
		u, err := t.StartHTTP()
		if err != nil {
			logger.Error(err)
		} else {
			logger.Info("Remote access url", u)
		}
		return true
	} else if sysParm == "--tunnel-down" {
		t := tunnel.NewTunnel()
		t.Stop()
		return true
	} else if sysParm == "--pdo-pos" {
		logger.Info("PDO position is printed automatically during runtime now (integrated mode).")
		return true
	}
	

	return false
}

// setupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func setupCloseHandler() {
	c := make(chan os.Signal, 1) // Buffered channel for signals
	wait = make(chan bool, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		sig := <-c
		logger.Info("Received signal:", sig, "starting graceful shutdown...")

		// PHASE 1 & 2: Walk the Drive State Machine down to a safe state
		// This uses the 3-phase logic in init_master.go to avoid Err88.2
		motor.ShutdownMasters()

		// PHASE 3: Stop the cyclic task and release the IgH Master
		// Note: ShutdownMasters already calls StopPDOCyclic() at the end of its sequence

		logger.Info("Exiting the application")
		logger.Info("-------------------------------------------------------------------------------------")
		
		// Signal main() that we are done with cleanup
		wait <- true
	}()
}

//runStartScript run the start.sh script in the current dir. User can add and action in this script which should
//execute after starting the system
func runStartScript() {
	c := exec.Command("/bin/sh", helper.AppendWDPath("/scripts/start.sh"))
	stderr := &bytes.Buffer{}
	stdout := &bytes.Buffer{}
	c.Stderr = stderr
	c.Stdout = stdout
	if err := c.Run(); err != nil {
		logger.Error(err)
		logger.Error("error opening ui in kiosk mode")
	}
}

// 🔹 new GPIO event logic
func startGPIOHandler() {
    cfg := gpiohandler.Config{
        InputPins:  []int{17, 22,23}, // add pin 22 as input
        OutputPins: []int{},       // outputs if needed later
        OnInputChange: func(pin, state int) {
            switch pin {
            case 17:
                // Jog mode (already working)
                if state == 1 {
                    logger.Info("[GPIO] Start Jog")
				
                    channels.DriverActionChannel <- channels.DriverAction{
                        Action:    "MANUAL_JOG",
                        Direction: 1,
                    }
                } else {
                    logger.Info("[GPIO] Stop Jog")

                    channels.DriverActionChannel <- channels.DriverAction{
                        Action: "STOP_JOG",
                    }
				
                }

            case 22:
                // Zero reference trigger
                if state == 1 {
                    logger.Info("[GPIO] Zero reference triggered by hardware")
                    channels.DriverActionChannel <- channels.DriverAction{
                        Action: "ZERO_REF",
                    }
                }
			case 23:
				if state == 1 {
					channels.WriteCommandExecInput("stop_prog_exec", "")
					executors.ResetExecutingProgram()
					time.Sleep(1 * time.Second)
					motor.RefreshCurrentPosition()
					logger.Info("[GPIO] Execute Program Triggered (GPIO 25 HIGH)")

					// Program file name is fixed as "FILES" (as you said)
					filePath := helper.GetCodeFilePath() + "/FILES"
			
					logger.Info("[GPIO] Executing program file:", filePath)
					err := executors.RunCodeFile(filePath)
					if err != nil {
						logger.Error("GPIO program execution failed:", err)
					}
				} else {
					logger.Info("[GPIO] Program button released (GPIO 25 LOW)")
				}
            }
        },
    }

    handler, err := gpiohandler.New(cfg)
    if err != nil {
        logger.Error("GPIO init failed:", err)
        return
    }
    handler.Start()
}

func startModbusHandler() {

    handler := modbus.NewTCPClientHandler("192.168.1.10:502")
    handler.Timeout = 1 * time.Second
    handler.SlaveId = 1

    if err := handler.Connect(); err != nil {
        logger.Error("Modbus connect failed:", err)
        return
    }
    defer handler.Close()

    client := modbus.NewClient(handler)
    logger.Info("✅ Modbus bit-wise polling started (GPIO emulation)")

    var last uint16 = 0

    for {
        // Read Register 2
        raw, err := client.ReadHoldingRegisters(2, 1)
        if err != nil {
            logger.Error("Modbus read failed:", err)
            time.Sleep(100 * time.Millisecond)
            continue
        }

        value := uint16(raw[0])<<8 | uint16(raw[1])

        // If no change, continue
        if value == last {
            time.Sleep(50 * time.Millisecond)
            continue
        }

        logger.Info("[Modbus] Register2 changed:", value)

        // Edge detection
        changed := value ^ last
        last = value

        // ===============================
        // BIT 0 → JOG  (GPIO 17)
        // ===============================
        if changed&(1<<1) != 0 {
            if value&(1<<1) != 0 {

				logger.Warn("[Modbus] Stopping program before EXECUTION...")
                channels.WriteCommandExecInput("stop_prog_exec", "")
                executors.ResetExecutingProgram()
                time.Sleep(200 * time.Millisecond)


                logger.Info("[Modbus] Start Jog")
                channels.DriverActionChannel <- channels.DriverAction{
                    Action:    "MANUAL_JOG",
                    Direction: 1,
                }

            } else {
                logger.Info("[Modbus] Stop Jog")
                channels.DriverActionChannel <- channels.DriverAction{
                    Action: "STOP_JOG",
                }
            }
        }

        // ===========================================
        // BIT 1 → ZERO REFERENCE  (GPIO 22)
        // ===========================================
        if changed&(1<<3) != 0 {
            if value&(1<<3) != 0 {

				logger.Warn("[Modbus] Stopping program before EXECUTION...")
                channels.WriteCommandExecInput("stop_prog_exec", "")
                executors.ResetExecutingProgram()
                time.Sleep(200 * time.Millisecond)


                logger.Info("[Modbus] ZERO_REF Trigger")
                channels.DriverActionChannel <- channels.DriverAction{
                    Action: "ZERO_REF",
                }
            }
        }

        // ===========================================
        // BIT 2 → EXECUTE PROGRAM  (GPIO 23)
        // ===========================================
        if changed&(1<<4) != 0 {
            if value&(1<<4) != 0 {

                logger.Warn("[Modbus] Stopping program before EXECUTION...")
                channels.WriteCommandExecInput("stop_prog_exec", "")
                executors.ResetExecutingProgram()
                time.Sleep(200 * time.Millisecond)

                motor.RefreshCurrentPosition()

                file := helper.GetCodeFilePath() + "/FILES"
                logger.Info("[Modbus] EXECUTING Program:", file)

                // --- THE FIX: DO NOT BLOCK THE MODBUS LOOP ---
                go func() {
                    if err := executors.RunCodeFile(file); err != nil {
                        logger.Error("Program execution failed:", err)
                    }
                }()
                // ------------------------------------------------

            } else {
                logger.Info("[Modbus] Program button released (bit2 LOW)")
            }
        }

		if changed&(1<<2) != 0 {
			if value&(1<<2) != 0 {
		
				logger.Info("[Modbus] Start Reverse Jog (bit4 HIGH)")
				channels.DriverActionChannel <- channels.DriverAction{
					Action:    "MANUAL_JOG",
					Direction: -1,   // REVERSE DIRECTION
				}
			} else {
				logger.Info("[Modbus] Stop Reverse Jog (bit4 LOW)")
				channels.DriverActionChannel <- channels.DriverAction{
					Action: "STOP_JOG",
				}
			}
		}
		
		

        time.Sleep(50 * time.Millisecond)
    }
}

