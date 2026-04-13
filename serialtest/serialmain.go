package serialtest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tarm/serial"

	"EtherCAT/channels"
	executors "EtherCAT/executors"
	"EtherCAT/helper"
	"EtherCAT/logger"
	motor "EtherCAT/motordriver"
)

const (
	serialDevice = "/dev/ttyUSB0"
	baudRate     = 9600
	filesPath    = "/mnt/app/jamun/gm_codes/FILES"
)

// ---- Execution control flags ----
var executionInProgress atomic.Bool
var fileUpdatedDuringExecution atomic.Bool

// ------------------------------------------------------------

func StartSerialListener() {
	cfg := &serial.Config{
		Name:        serialDevice,
		Baud:        baudRate,
		ReadTimeout: time.Millisecond * 200,
	}

	port, err := serial.OpenPort(cfg)
	if err != nil {
		logger.Error("Failed to open RS-232:", err)
		return
	}
	defer port.Close()

	logger.Info("RS-232 FILES updater + deferred executor started")

	buf := make([]byte, 256)
	frame := make([]byte, 0, 128)

	for {
		n, err := port.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		for i := 0; i < n; i++ {
			b := buf[i]

			// Ignore ISO handshake chatter
			if b == 0x12 {
				continue
			}

			// Strip parity bit (ISO-safe)
			clean := b & 0x7F

			// End-of-message detection (your controller is ending messages with CR/LF/0x14)
			if clean == '\n' || clean == '\r' || clean == 0x14 {
				cmd := strings.TrimSpace(string(frame))
				frame = frame[:0]

				if cmd != "" {
					handleRS232Command(cmd)
				}
				continue
			}

			frame = append(frame, clean)
		}
	}
}

// ------------------------------------------------------------
// RS232 command handler
// ------------------------------------------------------------

func handleRS232Command(cmd string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}

	parts := parseBatchCommands(cmd)

	// Readable logs (what came from controller + how we split it)
	logger.Info("[RS232] Batch received:", cmd)
	logger.Info("[RS232] Parsed commands:", strings.Join(parts, " | "))

	// Update FILES for each parsed command
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		norm := normalizeControllerToken(p)
		if norm == "" {
			continue
		}

		logger.Info("[RS232] Command received:", norm)

		if err := updateFILESFromRS232(norm); err != nil {
			logger.Error("[RS232] FILES update failed:", err)
			continue
		}

		logger.Info("[RS232] FILES updated with:", norm)
	}

	// Execution decision (unchanged behavior)
	if executionInProgress.Load() {
		fileUpdatedDuringExecution.Store(true)
		logger.Warn("[RS232] Execution in progress, update deferred")
		return
	}

	executionInProgress.Store(true)
	go executeFilesProgramOnce()
}

// ------------------------------------------------------------
// BATCH SPLIT + NORMALIZATION
// ------------------------------------------------------------

// parseBatchCommands supports these controller styles:
//
// 1) Concatenated: "G01F20G91G68A90"   -> ["G01F20","G91","G68","A90"]
// 2) Space-batch:  "G01 F20 G91 G68 A90;" -> ["G01 F20","G91","G68","A90"]
// 3) Comma-batch:  "go1f20,g91,g68,a90"   -> ["go1f20","g91","g68","a90"]
// 4) Multi-; batch: "G01 F20;G91;A90;"    -> ["G01 F20","G91","A90"]
func parseBatchCommands(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// If multiple commands are already ';' separated, split those first
	if strings.Count(s, ";") > 1 {
		raw := strings.Split(s, ";")
		out := make([]string, 0, len(raw))
		for _, r := range raw {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		return out
	}

	// Remove a single trailing ';' (batch terminator)
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimSpace(s)

	// Comma-batch: "go1f20,g91,g68,a90"
	if strings.Contains(s, ",") {
		raw := strings.Split(s, ",")
		out := make([]string, 0, len(raw))
		for _, r := range raw {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		return out
	}

	// Space-batch: "G01 F20 G91 G68 A90"
	if strings.Contains(s, " ") || strings.Contains(s, "\t") {
		toks := strings.Fields(s)
		if len(toks) == 0 {
			return nil
		}

		isCmdStart := func(t string) bool {
			if t == "" {
				return false
			}
			u := strings.ToUpper(strings.TrimSuffix(t, ";"))
			switch u[0] {
			case 'G', 'M', 'A', 'B', 'X', 'Y', 'Z', 'D':
				return true
			default:
				return false
			}
		}

		isParam := func(t string) bool {
			if t == "" {
				return false
			}
			u := strings.ToUpper(strings.TrimSuffix(t, ";"))
			switch u[0] {
			case 'F', 'P', 'I', 'J', 'K', 'R', 'X', 'Y', 'Z':
				return true
			default:
				return false
			}
		}

		var out []string
		var cur []string

		for _, t := range toks {
			if isCmdStart(t) {
				if len(cur) > 0 {
					out = append(out, strings.Join(cur, " "))
					cur = cur[:0]
				}
				cur = append(cur, t)
				continue
			}

			if len(cur) > 0 && isParam(t) {
				cur = append(cur, t)
				continue
			}

			if len(cur) > 0 {
				cur = append(cur, t)
			}
		}

		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
		}
		return out
	}

	// ✅ Concatenated: "G01F20G91G68A90"
	return splitConcatenatedBatch(s)
}

// Splits concatenated controller strings like:
// "G01F20G91G68A90" -> ["G01F20","G91","G68","A90"]
func splitConcatenatedBatch(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	u := strings.ToUpper(s)

	// New command starts at:
	// - any 'G' or 'M'
	// - axis vars you care about: A/B/D
	// Note: We do NOT use X/Y/Z here because those are usually parameters of G01.
	isStart := func(c byte) bool {
		switch c {
		case 'G', 'M', 'A', 'B', 'D':
			return true
		default:
			return false
		}
	}

	var out []string
	i := 0

	for i < len(u) {
		// Skip whitespace/junk
		if u[i] == ' ' || u[i] == '\t' {
			i++
			continue
		}

		if !isStart(u[i]) {
			i++
			continue
		}

		start := i
		i++ // consume letter

		// Consume digits for G/M number
		if u[start] == 'G' || u[start] == 'M' {
			for i < len(u) && u[i] >= '0' && u[i] <= '9' {
				i++
			}
			// Consume until next start (parameters like F20 will be included here)
			for i < len(u) && !isStart(u[i]) {
				i++
			}
		} else {
			// Axis var like A90/B20/D3: consume until next start
			for i < len(u) && !isStart(u[i]) {
				i++
			}
		}

		token := strings.TrimSpace(s[start:i])
		if token != "" {
			out = append(out, token)
		}
	}

	return out
}

// Converts one parsed token into a proper FILES line.
// Examples:
//   "G01F20" -> "G01 F20;"
//   "G91"    -> "G91;"
//   "A90"    -> "A90;"
//   "45"     -> "A45;"   (legacy numeric-only)
func normalizeControllerToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Legacy numeric-only => A<number>;
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return fmt.Sprintf("A%g;", v)
	}

	// remove trailing ';' (we add it back)
	s = strings.TrimSuffix(s, ";")

	// commas inside token -> spaces (just in case)
	s = strings.ReplaceAll(s, ",", " ")

	// uppercase
	s = strings.ToUpper(s)

	// handle GO1 -> G01 (letter O instead of 0)
	if strings.HasPrefix(s, "GO") && len(s) >= 3 {
		s = "G0" + s[2:]
	}

	// Insert spaces before param letters for G/M tokens (G01F20 -> G01 F20)
	if len(s) > 0 && (s[0] == 'G' || s[0] == 'M') {
		s = insertSpacesBeforeLetters(s)
	}

	// collapse spaces
	s = strings.Join(strings.Fields(s), " ")

	// ensure terminator
	if !strings.HasSuffix(s, ";") {
		s += ";"
	}
	return s
}

func insertSpacesBeforeLetters(s string) string {
	var out []rune
	var prev rune
	for i, r := range s {
		if i > 0 {
			isLetter := (r >= 'A' && r <= 'Z')
			isPrevDigit := (prev >= '0' && prev <= '9')
			if isLetter && isPrevDigit {
				out = append(out, ' ')
			}
		}
		out = append(out, r)
		prev = r
	}
	return string(out)
}

// ------------------------------------------------------------
// FILE UPDATE LOGIC
// ------------------------------------------------------------

func isAxisKey(k string) bool {
	if k == "" {
		return false
	}
	switch k[0] {
	case 'A', 'B', 'X', 'Y', 'Z', 'D':
		return true
	default:
		return false
	}
}

func findInsertBeforeEnd(lines []string) int {
	for i, line := range lines {
		lt := strings.TrimSpace(line)
		if strings.HasPrefix(lt, "M99") || strings.HasPrefix(lt, "M30") {
			return i
		}
	}
	return len(lines)
}

// updateFILESFromRS232 updates gm_codes/FILES according to rules:
//
// - Axis letters (A/B/X/Y/Z/D): match by letter only (A90 replaces any A...)
// - G/M: exact token match (G01 replaces G01 ...)
// - Special: if controller sends G91 and file has G90 -> replace G90 with G91 (and vice versa)
// - If not found: insert BEFORE M99/M30 (never after)
func updateFILESFromRS232(oneCmd string) error {
	oneCmd = strings.TrimSpace(oneCmd)
	if oneCmd == "" {
		return fmt.Errorf("empty command")
	}
	if !strings.HasSuffix(oneCmd, ";") {
		oneCmd += ";"
	}

	content, err := os.ReadFile(filesPath)
	if err != nil {
		return fmt.Errorf("failed to read FILES: %w", err)
	}
	lines := strings.Split(string(content), "\n")

	cmdNoSemi := strings.TrimSuffix(oneCmd, ";")
	toks := strings.Fields(cmdNoSemi)
	if len(toks) == 0 {
		return fmt.Errorf("invalid command: %q", oneCmd)
	}
	cmdKey := toks[0]

	replaced := false

	for i, line := range lines {
		lt := strings.TrimSpace(line)
		if lt == "" {
			continue
		}

		// Axis: match by letter only
		if isAxisKey(cmdKey) {
			if strings.HasPrefix(lt, string(cmdKey[0])) {
				lines[i] = oneCmd
				replaced = true
				break
			}
			continue
		}

		// Special modal: G90/G91 replace each other
		if cmdKey == "G90" || cmdKey == "G91" {
			if strings.HasPrefix(lt, "G90") || strings.HasPrefix(lt, "G91") {
				lines[i] = oneCmd
				replaced = true
				break
			}
			continue
		}

		// Normal: exact token match
		if strings.HasPrefix(lt, cmdKey) {
			lines[i] = oneCmd
			replaced = true
			break
		}
	}

	// Not found: insert before M99/M30
	if !replaced {
		idx := findInsertBeforeEnd(lines)
		lines = append(lines[:idx], append([]string{oneCmd}, lines[idx:]...)...)
	}

	// Atomic write
	tmpPath := filesPath + ".tmp"
	data := strings.Join(lines, "\n")
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}

	if err := os.WriteFile(tmpPath, []byte(data), 0644); err != nil {
		return fmt.Errorf("failed to write temp FILES: %w", err)
	}
	if err := os.Rename(tmpPath, filesPath); err != nil {
		return fmt.Errorf("failed to replace FILES: %w", err)
	}

	return nil
}

// ------------------------------------------------------------
// EXECUTION LOGIC (ONE-SHOT, DEFERRED)
// ------------------------------------------------------------

func executeFilesProgramOnce() {
	defer executionInProgress.Store(false)

	logger.Warn("[RS232] Preparing program execution")

	// Stop any residual execution
	channels.WriteCommandExecInput("stop_prog_exec", "")
	executors.ResetExecutingProgram()
	time.Sleep(200 * time.Millisecond)

	// Sync motor state
	motor.RefreshCurrentPosition()

	// ⏳ Execution delay (ADD HERE)
	time.Sleep(2 * time.Second)


	file := helper.GetCodeFilePath() + "/FILES"
	logger.Info("[RS232] EXECUTING Program:", file)

	// BLOCKS until:
	// - ECS satisfied
	// - program finishes
	// - PROGRAM_EXEC_COMPLETED issued
	if err := executors.RunCodeFile(file); err != nil {
		logger.Error("[RS232] Program execution failed:", err)
	} else {
		logger.Info("[RS232] Program execution completed")
	}

	// 🔁 If FILES changed during execution, run once more
	if fileUpdatedDuringExecution.Load() {
		logger.Info("[RS232] Detected FILES update during execution, executing latest program")

		fileUpdatedDuringExecution.Store(false)
		executionInProgress.Store(true)
		go executeFilesProgramOnce()
	}
}
