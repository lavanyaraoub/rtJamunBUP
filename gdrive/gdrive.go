package gdrive

import (
    "EtherCAT/helper"
    "EtherCAT/logger"
    "EtherCAT/settings"
    "context"
    "fmt"
    "os"
    "time"

    "golang.org/x/oauth2/google"
    "google.golang.org/api/drive/v3"
    "google.golang.org/api/option"
)

// UploadLogFileToDrive uploads the given log file to the configured GDrive folder
func UploadLogFileToDrive(logPath string) error {
    env := settings.GetEnvSettings()

    if env.GDriveFolderID == "" {
        return fmt.Errorf("gdrive folder id not configured")
    }
    if env.DeviceID == "" {
        return fmt.Errorf("device id not configured")
    }

    // Service account key JSON placed in /configs/gdrive-service-account.json
    keyPath := helper.AppendWDPath("/configs/remote-logs-479604-35c0d4e2fe37.json")
    jsonKey, err := os.ReadFile(keyPath)
    if err != nil {
        return fmt.Errorf("cannot read service account key: %w", err)
    }

    ctx := context.Background()

    // Use Drive API with a service account
    conf, err := google.JWTConfigFromJSON(jsonKey, drive.DriveFileScope)
    if err != nil {
        return fmt.Errorf("unable to parse service account JSON: %w", err)
    }
    client := conf.Client(ctx)

    srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
    if err != nil {
        return fmt.Errorf("unable to create drive service: %w", err)
    }

    f, err := os.Open(logPath)
    if err != nil {
        return fmt.Errorf("cannot open log file: %w", err)
    }
    defer f.Close()

    // File name in Drive: <device_id>-log-YYYY-MM-DD_HH-MM-SS.txt
    now := time.Now().Format("2006-01-02_15-04-05")
    fileName := fmt.Sprintf("%s-log-%s.txt", env.DeviceID, now)

    fileMetadata := &drive.File{
        Name:    fileName,
        Parents: []string{env.GDriveFolderID},
    }

    logger.Info("Uploading log file to Google Drive as ", fileName)

    _, err = srv.Files.Create(fileMetadata).Media(f).Do()
    if err != nil {
        return fmt.Errorf("drive upload failed: %w", err)
    }

    logger.Info("Log file uploaded to Google Drive successfully")
    return nil
}