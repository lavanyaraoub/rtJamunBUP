package main

import (
    "fmt"
    "log"
    "time"
   "net/http/pprof"

    "github.com/goburrow/modbus"
)

func main() {
    handler := modbus.NewTCPClientHandler("192.168.1.10:502")
    handler.Timeout = 5 * time.Second
    handler.SlaveId = 1

    err := handler.Connect()
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer handler.Close()

    client := modbus.NewClient(handler)
    fmt.Println("🔍 Scanning Modbus holding registers for active (non-zero) values...\n")

    // The ILC131 supports up to 7167 registers, but Modbus FC03 limits reads to 125 per call
    startAddr := uint16(0)
    maxRegisters := uint16(7167)
    blockSize := uint16(125)

    for startAddr < maxRegisters {
        toRead := blockSize
        if startAddr+blockSize > maxRegisters {
            toRead = maxRegisters - startAddr
        }

        // Read this block
        results, err := client.ReadHoldingRegisters(startAddr, toRead)
        if err != nil {
            log.Printf("Error reading block %d–%d: %v", startAddr, startAddr+toRead-1, err)
            startAddr += toRead
            continue
        }

        // Parse and show non-zero registers
        for i := uint16(0); i < toRead*2; i += 2 {
            val := uint16(results[i])<<8 | uint16(results[i+1])
            if val != 0 {
                reg := startAddr + i/2
                fmt.Printf("🟢 Register %-4d = %5d (0x%04X)\n", reg, val, val)
            }
        }

        startAddr += toRead
        time.Sleep(100 * time.Millisecond) // short delay between blocks
    }

    fmt.Println("\n✅ Scan complete.")
}
