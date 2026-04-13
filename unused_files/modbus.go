package main

import (
    "fmt"
    "log"
    "github.com/goburrow/modbus"
)

func main() {
    // PLC parameters
    handler := modbus.NewTCPClientHandler("192.168.1.10:502") // Update with your PLC's IP if different
    handler.SlaveId = 1 // Default Modbus Slave ID

    // Connect to the PLC
    err := handler.Connect()
    if err != nil {
        log.Fatalf("Unable to connect to PLC: %v", err)
    }
    defer handler.Close()

    client := modbus.NewClient(handler)

    // S1 and S2 Modbus addresses (zero-based for goburrow library)
    s1Addr := 60182 // %MX1.60183.0 => 60182
    s2Addr := 60183 // %MX1.60184.0 => 60183

    // Read S1 (Coil)
    s1Result, err := client.ReadCoils(uint16(s1Addr), 1)
    if err != nil {
        log.Fatalf("Error reading S1: %v", err)
    }
    s1 := s1Result[0]&1 == 1

    // Read S2 (Coil)
    s2Result, err := client.ReadCoils(uint16(s2Addr), 1)
    if err != nil {
        log.Fatalf("Error reading S2: %v", err)
    }
    s2 := s2Result[0]&1 == 1

    // Output values
    fmt.Printf("S1: %v\n", s1)
    fmt.Printf("S2: %v\n", s2)
}
