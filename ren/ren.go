package ren

import (
    "fmt"
    "io"
    "log"
    "time"
    "bufio"
    "os"
    "strings"
    "github.com/pkg/sftp"
    "golang.org/x/crypto/ssh"
    settings "EtherCAT/settings"   // ⭐ NEW IMPORT
)

// --- CONFIGURATION ---
// Converted to var so we can update dynamically from UI
var (
    pcIP                   = "192.168.2.29"
    pcPort                 = 22
    pcUsername             = "administrator"
    pcPassword             = "Ucam@2024#"
    remoteFilePathOnPC     = "C:/Users/maniprakash.n_ucamin/Downloads/mani.txt"
    remoteJogPositionPath  = "C:/Users/maniprakash.n_ucamin/Downloads/jog_positioncw.txt"
    remoteJogPositionPath1 = "C:/Users/maniprakash.n_ucamin/Downloads/jog_positionccw.txt"
    refreshInterval        = 1 * time.Second
)

// ⭐ NEW — Update variables when Save button pressed
func UpdateTextProgramConfig(cfg settings.TextProgramConfig) {
    if cfg.IP != "" {
        pcIP = cfg.IP
    }
    if cfg.User != "" {
        pcUsername = cfg.User
    }
    if cfg.Password != "" {
        pcPassword = cfg.Password
    }
    if cfg.Path != "" {
        remoteFilePathOnPC = cfg.Path
    }
    if cfg.JogClockwisePath != "" {
        remoteJogPositionPath = cfg.JogClockwisePath
    }
    if cfg.JogCounterClockwisePath != "" {
        remoteJogPositionPath1 = cfg.JogCounterClockwisePath
    }

    log.Println("⭐ Renishaw PC configuration updated from UI:")
    log.Println("IP:", pcIP)
    log.Println("User:", pcUsername)
    log.Println("Password:", pcPassword)
    log.Println("Main Path:", remoteFilePathOnPC)
    log.Println("CW Jog Path:", remoteJogPositionPath)
    log.Println("CCW Jog Path:", remoteJogPositionPath1)
}

// ⭐ NEW — Load saved config from JSON on startup
func init() {
    cfg, err := settings.LoadTextProgramConfig()
    if err == nil {
        UpdateTextProgramConfig(cfg)
    }
}

// LatestContent holds the most recent version of mani.txt
var LatestContent string

// ReadRemoteFile reads file once (on demand)
func ReadRemoteFile() (string, error) {
    sshConfig := &ssh.ClientConfig{
        User:            pcUsername,
        Auth:            []ssh.AuthMethod{ssh.Password(pcPassword)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    }

    addr := fmt.Sprintf("%s:%d", pcIP, pcPort)
    sshConn, err := ssh.Dial("tcp", addr, sshConfig)
    if err != nil {
        return "", fmt.Errorf("failed to connect to PC: %w", err)
    }
    defer sshConn.Close()

    sftpClient, err := sftp.NewClient(sshConn)
    if err != nil {
        return "", fmt.Errorf("failed to create SFTP client: %w", err)
    }
    defer sftpClient.Close()

    remoteFile, err := sftpClient.Open(remoteFilePathOnPC)
    if err != nil {
        return "", fmt.Errorf("failed to open remote file: %w", err)
    }
    defer remoteFile.Close()

    content, err := io.ReadAll(remoteFile)
    if err != nil {
        return "", fmt.Errorf("failed to read remote file: %w", err)
    }

    return string(content), nil
}

// StartMonitor runs in background, refreshes LatestContent periodically
func StartMonitor() {
    go func() {
        for {
            content, err := ReadRemoteFile()
            if err != nil {
                log.Println("⚠️ Error reading renishaw.txt from PC:", err)
            } else {
                if content != LatestContent { // detect change
                    log.Println("🔄 renishaw.txt updated on PC, refreshing on Pi")
                    LatestContent = content

                    // Update local G-code files
                    err := WriteLatestToGmCodes("FILES")
                    if err != nil {
                        log.Println("⚠️ Failed to update G-code file:", err)
                    } else {
                        log.Println("✅ Local G-code file updated with latest A- value")
                    }
                }
            }
            time.Sleep(refreshInterval)
        }
    }()
}

// WriteLatestToGmCodes updates a local G-code file with LatestContent
func WriteLatestToGmCodes(localFileName string) error {
    if LatestContent == "" {
        return fmt.Errorf("LatestContent is empty")
    }

    newValue := strings.TrimSpace(LatestContent)
    if newValue == "" {
        return fmt.Errorf("renishaw.txt content is empty or invalid")
    }

    aLine := "A" + newValue + ";"
    localFilePath := "/mnt/app/jamun/gm_codes/FILES"

    file, err := os.Open(localFilePath)
    if err != nil {
        return fmt.Errorf("failed to open local file: %w", err)
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    var updatedLines []string
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "A") {
            line = aLine
        }
        updatedLines = append(updatedLines, line)
    }

    if err := scanner.Err(); err != nil {
        return fmt.Errorf("error reading local file: %w", err)
    }

    outFile, err := os.Create(localFilePath)
    if err != nil {
        return fmt.Errorf("failed to write local file: %w", err)
    }
    defer outFile.Close()

    for _, line := range updatedLines {
        fmt.Fprintln(outFile, line)
    }

    return nil
}

func WriteRemoteFile(data string) error {
    sshConfig := &ssh.ClientConfig{
        User:            pcUsername,
        Auth:            []ssh.AuthMethod{ssh.Password(pcPassword)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    }

    addr := fmt.Sprintf("%s:%d", pcIP, pcPort)
    sshConn, err := ssh.Dial("tcp", addr, sshConfig)
    if err != nil {
        return fmt.Errorf("failed to connect to PC: %w", err)
    }
    defer sshConn.Close()

    sftpClient, err := sftp.NewClient(sshConn)
    if err != nil {
        return fmt.Errorf("failed to create SFTP client: %w", err)
    }
    defer sftpClient.Close()

    remoteFile, err := sftpClient.OpenFile(remoteFilePathOnPC, os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
    if err != nil {
        return fmt.Errorf("failed to open remote file for write: %w", err)
    }
    defer remoteFile.Close()

    _, err = remoteFile.Write([]byte(data))
    if err != nil {
        return fmt.Errorf("failed to write remote file: %w", err)
    }

    return nil
}

func UpdateRTCToPC(rtcValue string) error {
    if strings.TrimSpace(rtcValue) == "" {
        return fmt.Errorf("empty RTC value")
    }

    err := WriteRemoteFile(rtcValue)
    if err != nil {
        log.Println("Failed to update RTC to PC:", err)
        return err
    }

    log.Println("RTC position written to PC mani.txt:", rtcValue)
    return nil
}

func WriteRemoteJogPosition(data string) error {
    sshConfig := &ssh.ClientConfig{
        User:            pcUsername,
        Auth:            []ssh.AuthMethod{ssh.Password(pcPassword)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    }

    addr := fmt.Sprintf("%s:%d", pcIP, pcPort)
    sshConn, err := ssh.Dial("tcp", addr, sshConfig)
    if err != nil {
        return fmt.Errorf("failed to connect to PC: %w", err)
    }
    defer sshConn.Close()

    sftpClient, err := sftp.NewClient(sshConn)
    if err != nil {
        return fmt.Errorf("failed to create SFTP client: %w", err)
    }
    defer sftpClient.Close()

    remoteFile, err := sftpClient.OpenFile(remoteJogPositionPath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
    if err != nil {
        return fmt.Errorf("failed to open jog_position file: %w", err)
    }
    defer remoteFile.Close()

    _, err = remoteFile.Write([]byte(data))
    if err != nil {
        return fmt.Errorf("failed to write jog_position file: %w", err)
    }

    return nil
}

func WriteRemoteJogPosition1(data string) error {
    sshConfig := &ssh.ClientConfig{
        User:            pcUsername,
        Auth:            []ssh.AuthMethod{ssh.Password(pcPassword)},
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
    }

    addr := fmt.Sprintf("%s:%d", pcIP, pcPort)
    sshConn, err := ssh.Dial("tcp", addr, sshConfig)
    if err != nil {
        return fmt.Errorf("failed to connect to PC: %w", err)
    }
    defer sshConn.Close()

    sftpClient, err := sftp.NewClient(sshConn)
    if err != nil {
        return fmt.Errorf("failed to create SFTP client: %w", err)
    }
    defer sftpClient.Close()

    remoteFile, err := sftpClient.OpenFile(remoteJogPositionPath1, os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
    if err != nil {
        return fmt.Errorf("failed to open jog_position file: %w", err)
    }
    defer remoteFile.Close()

    _, err = remoteFile.Write([]byte(data))
    if err != nil {
        return fmt.Errorf("failed to write jog_position file: %w", err)
    }

    return nil
}

func UpdateJogPositionToPC(rtcValue string) error {
    rtcValue = strings.TrimSpace(rtcValue)
    if rtcValue == "" {
        return fmt.Errorf("empty RTC value")
    }

    err := WriteRemoteJogPosition(rtcValue)
    if err != nil {
        log.Println("Failed to update jog position to PC:", err)
        return err
    }

    log.Println("Jog position written to PC jog_position.txt:", rtcValue)
    return nil
}

func UpdateJogPositionToPC1(rtcValue string) error {
    rtcValue = strings.TrimSpace(rtcValue)
    if rtcValue == "" {
        return fmt.Errorf("empty RTC value")
    }

    err := WriteRemoteJogPosition1(rtcValue)
    if err != nil {
        log.Println("Failed to update jog position to PC:", err)
        return err
    }

    log.Println("Jog position written to PC jog_position.txt:", rtcValue)
    return nil
}