package main

import (
    "fmt"
    "log"
    "time"

    "github.com/goburrow/modbus"
)

func main() {
    // 1️⃣ Configure Modbus TCP handler
    handler := modbus.NewTCPClientHandler("192.168.1.10:502")
    handler.Timeout = 5 * time.Second
    handler.SlaveId = 1 // ILC131 unit ID, typically 1

    // 2️⃣ Connect once and reuse the connection
    err := handler.Connect()
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer handler.Close()

    // 3️⃣ Create Modbus client
    client := modbus.NewClient(handler)

    fmt.Println("🔌 Connected — polling register 2 every 500 ms...\n")

    // 4️⃣ Poll loop
    for {
        // Read ONE holding register starting at address 2
        results, err := client.ReadHoldingRegisters(2, 1)
        if err != nil {
            log.Printf("❌ Read error: %v", err)
            time.Sleep(2 * time.Second)
            continue
        }

        // Convert bytes → uint16
        value := uint16(results[0])<<8 | uint16(results[1])

        // Display current value and bits
        fmt.Printf("[%s] Reg 2 = %5d (0x%04X) → Bits: %016b\n",
            time.Now().Format("15:04:05.000"), value, value, value)

        time.Sleep(1000 * time.Millisecond) // 0.5 s interval
    }
}