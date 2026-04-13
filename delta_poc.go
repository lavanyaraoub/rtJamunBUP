package main

/*
#cgo CFLAGS: -I/opt/etherlab/include
#cgo LDFLAGS: -L/opt/etherlab/lib -lethercat
#include "ecrt.h"
#include <stdio.h>

// 1. Helper functions for macros and state
static inline int32_t read_s32(uint8_t *ptr, unsigned int offset) {
    return EC_READ_S32(ptr + offset);
}

static inline uint16_t read_u16(uint8_t *ptr, unsigned int offset) {
    return EC_READ_U16(ptr + offset);
}

static inline uint8_t get_al_state(ec_master_t *master) {
    ec_master_state_t state;
    ecrt_master_state(master, &state);
    return state.al_states; 
}

// 2. Mapping definitions - Removed 'static' to avoid linker issues
ec_pdo_entry_info_t delta_entries[] = {
    {0x6040, 0x00, 16},
    {0x607a, 0x00, 32},
    {0x6041, 0x00, 16},
    {0x6064, 0x00, 32},
};

ec_pdo_info_t delta_pdos[] = {
    {0x1601, 2, delta_entries + 0},
    {0x1a01, 2, delta_entries + 2},
};

ec_sync_info_t delta_syncs[] = {
    {0, EC_DIR_OUTPUT, 0, NULL, EC_WD_DISABLE},
    {1, EC_DIR_INPUT,  0, NULL, EC_WD_DISABLE},
    {2, EC_DIR_OUTPUT, 1, delta_pdos + 0, EC_WD_ENABLE},
    {3, EC_DIR_INPUT,  1, delta_pdos + 1, EC_WD_DISABLE},
    {0xff}
};

// Data Offsets
unsigned int off_control;
unsigned int off_target;
unsigned int off_status;
unsigned int off_actual;

const ec_pdo_entry_reg_t domain_regs[] = {
    {0, 0, 0x000001dd, 0x10305070, 0x6040, 0x00, &off_control},
    {0, 0, 0x000001dd, 0x10305070, 0x607a, 0x00, &off_target},
    {0, 0, 0x000001dd, 0x10305070, 0x6041, 0x00, &off_status},
    {0, 0, 0x000001dd, 0x10305070, 0x6064, 0x00, &off_actual},
    {}
};

// Helper to get the pointer to the sync info
static inline ec_sync_info_t* get_delta_syncs() {
    return delta_syncs;
}

// Helper to get the pointer to the registration list
static inline const ec_pdo_entry_reg_t* get_domain_regs() {
    return domain_regs;
}

*/
import "C"
import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("Starting Delta EtherCAT PoC...")

	master := C.ecrt_request_master(0)
	if master == nil {
		panic("Failed to request master")
	}
	defer C.ecrt_release_master(master)

	domain := C.ecrt_master_create_domain(master)
	if domain == nil {
		panic("Failed to create domain")
	}

	sc := C.ecrt_master_slave_config(master, 0, 0, 0x000001dd, 0x10305070)
	if sc == nil {
		panic("Failed to get slave config")
	}

	// Use the helper function to pass the pointer
	if C.ecrt_slave_config_pdos(sc, C.EC_END, C.get_delta_syncs()) != 0 {
		panic("Failed to configure PDOs")
	}

	// Use the helper function for registration
	if C.ecrt_domain_reg_pdo_entry_list(domain, C.get_domain_regs()) != 0 {
		panic("PDO entry registration failed")
	}

	if C.ecrt_master_activate(master) != 0 {
		panic("Failed to activate master")
	}

	domainPD := C.ecrt_domain_data(domain)
	
	ticker := time.NewTicker(2 * time.Millisecond)
	logTicker := time.NewTicker(1 * time.Second)
	
	fmt.Println("PoC Running. Monitor your terminal...")

	for {
		select {
		case <-ticker.C:
			C.ecrt_master_receive(master)
			C.ecrt_domain_process(domain)

			var ds C.ec_domain_state_t
			C.ecrt_domain_state(domain, &ds)

			if ds.wc_state == 2 {
				rawPos := int32(C.read_s32(domainPD, C.off_actual))
				status := uint16(C.read_u16(domainPD, C.off_status))
				
				select {
				case <-logTicker.C:
					alState := C.get_al_state(master)
					fmt.Printf("[STATUS] AL State: %d | Pos: %d | Statusword: 0x%04X | WC: %d\n", 
						alState, rawPos, status, ds.wc_state)
				default:
				}
			}

			C.ecrt_domain_queue(domain)
			C.ecrt_master_send(master)
		}
	}
}