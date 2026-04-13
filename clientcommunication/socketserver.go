package clientcommunication

import (
	channels "EtherCAT/channels"
	executors "EtherCAT/executors"
	"EtherCAT/helper"
	logger "EtherCAT/logger"
	"EtherCAT/systemupdate"
	settings "EtherCAT/settings"
	"fmt"
	"net/http"
	"strings"

	gosocketio "github.com/graarh/golang-socketio"
	"github.com/graarh/golang-socketio/transport"
)

//ConnectedClientList keep tracks of all the clients connected
type ConnectedClientList struct {
	Clients []Client
}

//Client keeps client details connected via socket
type Client struct {
	Channel *gosocketio.Channel
	ID      string
}

var connectedClients ConnectedClientList

func init() {
	channels.BroadCastUIChannel = make(chan channels.SocketMessage, 100)
}

var rs232State int = 0 // 0 = OFF, 1 = ON(Socketserver rs232 state change)

// RS232 status payload contract for UI: { Data: "0" } or { Data: "1" }
type rs232Status struct {
	Data string `json:"Data"`
}

//Start for socket connection from client
func Start() error {

	server := gosocketio.NewServer(transport.GetDefaultWebsocketTransport())

	// ---- RS232: restore persisted state on backend boot ----
	data, err := settings.LoadRS232Data()
	if err != nil {
		logger.Error("Failed to load RS232 status from disk:", err)
	} else {
		if data == "1" {
			rs232State = 1
			executors.RS232Enabled.Store(true)
		} else {
			rs232State = 0
			executors.RS232Enabled.Store(false)
		}
		logger.Info("RS232 status restored:", data)
	}

	socketEventsCreator(server)
	serveMux := http.NewServeMux()
	serveMux.Handle("/socket.io/", server)
	//go routine waiting for any sort of messages that needs to transmit to ui
	go uiBradcastMessageListner()

	logger.Info("starting socket.io server listening at port 9090...")
	err = http.ListenAndServe(":9090", serveMux)
	if err != nil {
		logger.Error(err)
	}
	logger.Info("started socket.io server listening at port 9090")
	return err
}

func socketEventsCreator(server *gosocketio.Server) {
	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel) {
		logger.Debug("new client connected, client id:", c.Id())
		client := Client{Channel: c, ID: c.Id()}
		connectedClients.Clients = append(connectedClients.Clients, client)
		channels.SendAlarm("No Alarms")

		// ---- RS232: push current status to UI on connect ----
		cur := "0"
		if executors.RS232Enabled.Load() {
			cur = "1"
		}
		c.Emit("rs232_status", rs232Status{Data: cur})
	})

	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		logger.Debug("client dis-connected, client id:", c.Id())
		removeClient(c.Id())
	})

	server.On("jog_mode", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("jog_mode event received from client")
		direction := 1
		if msg.Direction <= 0 {
			direction = -1
		}
		if msg.Action == 1 {
			logger.Debug("start jogging")
			driverAction := channels.DriverAction{Action: "MANUAL_JOG", Direction: direction}
			channels.DriverActionChannel <- driverAction
		} else {
			logger.Debug("stop jogging")
			driverAction := channels.DriverAction{Action: "STOP_JOG"}
			channels.DriverActionChannel <- driverAction
		}
	})

	server.On("reset", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("Reset initiated")
		channels.NotifyMotorDriver("RESET", "", "", 0)
		channels.SendAlarm("No Alarms")
	})

	server.On("goToZero", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("Zero referenced enabled")
		channels.NotifyMotorDriver("ZERO_REF", "", "", 0)
	})

	server.On("enable_step_mode", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("step mode enabled")
		channels.NotifyMotorDriver("STEP_MODE_ENABLE", "", "", 0)
	})

	server.On("step_mode", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("running in step mode, with position to add", msg.Position2)
		direction := 1
		if msg.Direction <= 0 {
			direction = -1
		}
		channels.NotifyMotorDriver("STEP_MODE", fmt.Sprintf("%f", msg.Position2), "", direction)
	})

	server.On("execute", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("execute the program")
		executeProgram(msg.FileName)
	})

	server.On("emergency", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("emergency activated")
		channels.NotifyMotorDriver("EMERGENCY", "", "", 0)
		channels.WriteCommandExecInput("stop_prog_exec", "")
	})

	server.On("set_program_mode", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("set program mode")
		if msg.Data == "single" {
			channels.WriteCommandExecInput("command_exec_mode", "single")
		} else {
			channels.WriteCommandExecInput("command_exec_mode", "continuous")
		}
	})

	server.On("exec_next_line", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("execute next line")
		channels.WriteCommandExecInput("move_next_line", "1")
	})

	server.On("stop_execution", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("stop executing program")
		channels.WriteCommandExecInput("stop_prog_exec", "")
		channels.WriteCommandExecInput("move_next_line", "1")
	})

	server.On("resetMultiTurn", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("reset multiturn requested by user")
		channels.NotifyMotorDriver("RESET_MULTI_TURN", "", "", 0)
	})

	server.On("perform_system_update", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("system update requested from user")
		go systemupdate.PerformSystemUpdate(true)
	})

	server.On("check_system_update", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("check for system update requested by user")
		go systemupdate.CheckforUpdates(true)
	})

	server.On("save_line_number", func(c *gosocketio.Channel, msg channels.SocketMessage) {
		logger.Debug("save_line_number event received from client")

		if msg.UserLine == "" {
			logger.Warn("no user_line provided in message")
			return
		}

		err := settings.SaveLineNumber(msg.UserLine)

		if err != nil {
			logger.Error("failed to save line number:", err)
		} else {
			logger.Info("line number saved to userline.json:", msg.UserLine)
			executors.UpdateLastLineFromJSON()
		}
	})

	server.On("save-text-program", func(c *gosocketio.Channel, config settings.TextProgramConfig) {
		logger.Info("save-text-program event received from client")

		err := settings.SaveTextProgramConfig(config)
		if err != nil {
			logger.Error("failed to save text program config:", err)
		} else {
			logger.Info("text program config saved successfully to textprogram.json")

		}
	})

	server.On("get-text-program-config", func(c *gosocketio.Channel) {
		logger.Info("get-text-program-config event received from client")

		config, err := settings.LoadTextProgramConfig()
		if err != nil {
			logger.Error("failed to load text program config:", err)
			c.Emit("text-program-config", nil)
			return
		}

		c.Emit("text-program-config", config)
	})

	// ---- RS232: UI asks for current status ----
	server.On("get_rs232_status", func(c *gosocketio.Channel) {
		cur := "0"
		if executors.RS232Enabled.Load() {
			cur = "1"
		}
		c.Emit("rs232_status", rs232Status{Data: cur})
	})

	server.On("rs232_toggle", func(c *gosocketio.Channel, msg channels.SocketMessage) {

		// sanitize: accept only "0" or "1"
		data := msg.Data
		if data != "0" && data != "1" {
			data = "0"
		}

		if data == "1" {
			rs232State = 1
			executors.RS232Enabled.Store(true)
			logger.Info("RS232 state updated to: 1 (ENABLED)")
		} else {
			rs232State = 0
			executors.RS232Enabled.Store(false)
			logger.Info("RS232 state updated to: 0 (DISABLED)")
		}

		// Persist to disk so reboot remembers
		if err := settings.SaveRS232Data(data); err != nil {
			logger.Error("Failed to save RS232 status to disk:", err)
		}

		// Broadcast updated status to all connected clients
		for _, cl := range connectedClients.Clients {
			cl.Channel.Emit("rs232_status", rs232Status{Data: data})
		}
	})
}

func executeProgram(fileName string) {
	err := executors.RunCodeFile(helper.GetCodeFilePath() + "/" + fileName)
	if err != nil {
		logger.Error(err)
		channels.SendAlarm(err.Error())
	}
}

func uiBradcastMessageListner() {
	for {
		msg := <-channels.BroadCastUIChannel
		if len(connectedClients.Clients) >= 0 {
			if msg.Alarm == "" {
				go Send(msg)
			} else {
				go sendAlarm(msg)
			}
		}
	}
}

func removeClient(id string) {
	for i, client := range connectedClients.Clients {
		if client.ID == id {
			connectedClients.Clients = append(connectedClients.Clients[:i], connectedClients.Clients[i+1:]...)
			logger.Debug("removed disconnected client from collection", id)
			break
		}
	}
}

func Send(message channels.SocketMessage) {
	for _, client := range connectedClients.Clients {
		client.Channel.Emit(message.Event, message)
	}
}

func sendAlarm(message channels.SocketMessage) {
	if !strings.Contains(message.Alarm, "No Alarms") {
		channels.WriteCommandExecInput("stop_prog_exec", "")
	}
	logger.Trace("send alarm to ui", message.Alarm)
	for _, client := range connectedClients.Clients {
		client.Channel.Emit(message.Event, message.Alarm)
	}
}

/*
	custom events listen by ui client
	--------------------------------------
	reset_alert
	pos_data: destination postion
	sent_file_cont: sent the file content of program
	reset_done
	destination_position  (tcpserver.js line# 415 ui_clients[i].emit("destination_position",{"pos":p+(mp.factor_backlash*mp.drive_backlash)-pe_val});)
	alarm_error
	FINSIGNAL
	alarms
	ref_complete  (updateClients("ref_complete",{'ref':'complete'});)
	ETH_DOWN  updateClients('ETH_DOWN', "Ethercat communication down...");
	ETH_UP  updateClients('ETH_UP', "Ethercat communication up...");

	custom events listen by server
	--------------------------------------
	set_program_mode
	program
	status
	stop_execution
	updateSettings
	reset
	resetMultiTurn
	emergency
	execute
	get_file_cont
	line_number
	line_complete
	enable_step_mode
	step_mode
	jog_mode
	start_homing
	stop_homing
	homing_complete
	pos_data
	destination_position
	exec_next_line
	enable_ecs
	disable_ecs
	goToZero
	pot_hard_limit
	not_hard_limit
*/
