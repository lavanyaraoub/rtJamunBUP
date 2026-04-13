package motordriver

/*
#cgo CFLAGS: -g -Wall -I/opt/etherlab/include -I/home/pi/gosrc/src/EtherCAT
#cgo LDFLAGS: -L/home/pi/gosrc/src/EtherCAT -L/opt/etherlab/lib/ -lethercatinterface -lethercat
#include "ecrt.h"
#include "ethercatinterface.h"
#include <string.h>
*/
import "C"

import (
	"errors"
	"fmt"
)

// SetupPDOPosition registers all PDO entries (RxPDO 0x1600 + TxPDO) into a domain.
// Offsets are hardcoded in ethercatinterface.c (OFF_* constants). Does NOT activate.
func SetupPDOPosition(dev *MasterDevice) error {
	if dev == nil || dev.Master == nil {
		return errors.New("SetupPDOPosition: nil master device")
	}

	// ---- Create domain ----
	domain := C.ecrt_master_create_domain(dev.Master)
	if domain == nil {
		return errors.New("SetupPDOPosition: failed to create domain")
	}

	// ---- Get slave config ----
	// Note: first arg is alias, second is ring-position (device ID).
	// Make sure dev.Device.Alias and dev.Device.ID match your hardware.
	sc := C.ecrt_master_slave_config(
		dev.Master,
		C.ushort(dev.Device.Alias),
		C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID),
		C.uint(dev.Device.ProductCode),
	)
	if sc == nil {
		return errors.New("SetupPDOPosition: ecrt_master_slave_config failed " +
			"(check alias/position/vendor/product match your hardware)")
	}

	// ---- Apply PDO mapping 0x1600 / 0x1A00 ----
	// This tells the IgH master which PDOs to use before activation.
	if rc := C.configure_minas_a6_pdos(sc); rc != 0 {
		return fmt.Errorf("SetupPDOPosition: configure_minas_a6_pdos failed: %v",
			C.GoString(C.strerror(-rc)))
	}

	// Size the domain: registers all 14 PDO entries so IgH creates FMMU configs.
	// Without this, ecrt_domain_data() returns nil. Returned offsets are discarded.
	if rc := C.setup_domain_sizing(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
	); rc != 0 {
		return fmt.Errorf("SetupPDOPosition: setup_domain_sizing failed: %d", int(rc))
	}

	// ---- Retrieve 0x60FE:01/02 digital output offsets ----
	// These are RxPDO entries (master → drive) used to toggle physical digital
	// outputs (fin signal, brake solenoid) via PDO instead of SDO.
	// setup_domain_sizing caches them; we fetch and store here.
	var offDigMask, offDigVal C.uint
	if rc := C.get_digital_output_offsets((*C.uint)(&offDigMask), (*C.uint)(&offDigVal)); rc == 0 {
		dev.OffDigOutMask = offDigMask
		dev.OffDigOutVal = offDigVal
		dev.PdoDigOutReady = true
		// Pre-assert the EtherCAT ownership mask (0x60FE:02) for ALL output bits.
		// On Panasonic A6, 0x60FE:02 must have bits set before the drive will
		// respect 0x60FE:01 values from EtherCAT. We set full ownership so any
		// output bit we write to 0x60FE:01 will actually activate the pin.
		// The actual output values (0x60FE:01) start at 0 (all pins LOW/off).
		// This is stored in desiredDigOutVal; the cyclic task writes it every cycle.
		dev.desiredDigOutVal.Store(0xFFFFFFFF)  // 0x60FE:02: EtherCAT owns ALL output bits
		dev.desiredDigOutMask.Store(0x00000000) // 0x60FE:01: all outputs LOW initially
		fmt.Printf("[PDO] Digital output offsets — Mask(0x60FE:01):%d Val(0x60FE:02):%d\n",
			uint(offDigMask), uint(offDigVal))
	} else {
		dev.PdoDigOutReady = false
		fmt.Printf("[WARN] SetupPDOPosition: get_digital_output_offsets failed: %d\n", int(rc))
	}
	// ---- Create async SDO request objects for multiturn reset ----
	// ec_sdo_request_t objects are registered here (pre-activate) and can
	// be triggered at any time during Op mode — IgH services the mailbox
	// inside ecrt_master_receive() each cycle. No blocking, no deadlock.
	// This replaces the old PDO domain approach (caused PreOp lockup) and
	// the old blocking ecrt_master_sdo_download approach (deadlock during Op).
	if rc := C.create_mt_sdo_requests(sc); rc == 0 {
		dev.MTSdoReady = true
		fmt.Println("[PDO] Multiturn SDO request objects registered (0x4D01:00, 0x4D00:01)")
	} else {
		dev.MTSdoReady = false
		dev.PdoMTReady = false
		fmt.Printf("[WARN] SetupPDOPosition: create_mt_sdo_requests failed rc=%d — multiturn reset unavailable during Op\n", int(rc))
	}
	// ---- ADD THIS BLOCK: Create async SDO request for Profile Velocity (0x6081) ----
	reqPtr := C.create_profile_vel_sdo_request(sc)
	if reqPtr != nil {
		dev.PdoVelSdoReq = reqPtr
		dev.PdoVelSdoReady = true
		fmt.Println("[PDO] Profile Velocity SDO request object registered (0x6081:00)")
	} else {
		dev.PdoVelSdoReady = false
		fmt.Printf("[WARN] SetupPDOPosition: create_profile_vel_sdo_request failed\n")
	}

	// ================================================================
	// Register TxPDO entries (drive → master, feedback)
	// All objects below are in 0x1A00 per the ESI.
	// ================================================================

	// 0x6064:0  Position actual value
	var offPos C.uint
	if rc := C.setup_pos_pdo(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
		(*C.uint)(&offPos),
	); rc != 0 {
		return fmt.Errorf("SetupPDOPosition: setup_pos_pdo (0x6064) failed: %d", int(rc))
	}
	dev.OffPos = offPos
	dev.PdoReady = true

	// 0x6041:0  Statusword
	var offStatus C.uint
	if rc := C.setup_statusword_pdo(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
		(*C.uint)(&offStatus),
	); rc == 0 {
		dev.OffStatus = offStatus
		dev.PdoStatusReady = true
	} else {
		dev.PdoStatusReady = false
		fmt.Printf("[WARN] SetupPDOPosition: setup_statusword_pdo (0x6041) failed: %d\n", int(rc))
	}

	// 0x603F:0  Error code
	var offError C.uint
	if rc := C.setup_error_code_pdo(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
		(*C.uint)(&offError),
	); rc == 0 {
		dev.OffErrorCode = offError
		dev.PdoErrorReady = true
	} else {
		dev.PdoErrorReady = false
		fmt.Printf("[WARN] SetupPDOPosition: setup_error_code_pdo (0x603F) failed: %d\n", int(rc))
	}

	// 0x4F25:0  Input signal register (vendor-specific) — used for ECS, POT, NOT, clamp signals.
	// NOTE: Physical drive maps 0x4F25:00 in TxPDO 0x1A00, NOT 0x60FD.
	var offDI C.uint
	if rc := C.setup_digital_inputs_pdo(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
		(*C.uint)(&offDI),
	); rc == 0 {
		dev.OffDigitalInputs = offDI
		dev.PdoDIReady = true
	} else {
		dev.PdoDIReady = false
		fmt.Printf("[WARN] SetupPDOPosition: setup_digital_inputs_pdo (0x4F25) failed: %d\n", int(rc))
	}

	// Register all 6 RxPDO entries (0x6040/0x6060/0x607A/0x60FF/0x60FE:01/02).
	// 0x4D00/0x4D01 not included — multiturn reset uses async SDO requests.
	var offCW, offOp, offTargetPos, offTargetVel C.uint
	if rc := C.setup_all_rx_pdo(
		domain,
		C.ushort(dev.Device.Alias), C.ushort(dev.Device.ID),
		C.uint(dev.Device.VendorID), C.uint(dev.Device.ProductCode),
		(*C.uint)(&offCW),
		(*C.uint)(&offOp),
		(*C.uint)(&offTargetPos),
		(*C.uint)(&offTargetVel),
	); rc == 0 {
		dev.OffControlWord = offCW
		dev.OffOpMode = offOp
		dev.OffTargetPos = offTargetPos
		dev.OffTargetVel = offTargetVel
		dev.PdoJogReady = true
		dev.PdoPosReady = true
		dev.PdoRxReady = true
	} else {
		// RxPDO failed: log detail, degrade gracefully to SDO fallback.
		errMsg := fmt.Sprintf("SetupPDOPosition: setup_all_rx_pdo failed (rc=%d). "+
			"Jog and position PDO modes disabled. Check PDO mapping on drive.", int(rc))
		fmt.Println("[ERROR]", errMsg)
		dev.PdoJogReady = false
		dev.PdoPosReady = false
		dev.PdoRxReady = false
	}

	// Store domain; DomainPD is assigned after ecrt_master_activate in StartPDOCyclic.
	dev.Domain = domain

	fmt.Printf("[PDO] Setup complete — PdoReady=%v PdoJogReady=%v PdoPosReady=%v "+
		"PdoStatusReady=%v PdoErrorReady=%v PdoDIReady=%v\n",
		dev.PdoReady, dev.PdoJogReady, dev.PdoPosReady,
		dev.PdoStatusReady, dev.PdoErrorReady, dev.PdoDIReady)
	fmt.Printf("[PDO] Offsets — CW:%d OpMode:%d TargetPos:%d TargetVel:%d "+
		"Status:%d Pos:%d ErrCode:%d DI:%d DigMask:%d DigVal:%d\n",
		dev.OffControlWord, dev.OffOpMode, dev.OffTargetPos, dev.OffTargetVel,
		dev.OffStatus, dev.OffPos, dev.OffErrorCode, dev.OffDigitalInputs,
		dev.OffDigOutMask, dev.OffDigOutVal)

	return nil
}