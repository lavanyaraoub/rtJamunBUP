#ifndef ETHERCATINTERFACE_H
#define ETHERCATINTERFACE_H

#include "ecrt.h"
#include <stdint.h>
#include <stdlib.h>

ec_master_t *requestMaster(int index);

int sdo_upload(ec_master_t *master, uint16_t slave_position, uint16_t index, uint8_t subindex,
               uint8_t *data, size_t data_size, uint32_t *abort_code);
int sdo_download(ec_master_t *master, uint16_t slave_position, uint16_t index, uint8_t subindex,
                 uint8_t *data, size_t data_size, uint32_t *abort_code);
int drivePosition(ec_master_t *master, uint16_t slave_position, uint16_t index, uint8_t subindex, uint8_t data);
int sdo_upload2(ec_master_t *master, uint16_t slave_position, uint16_t index, uint8_t subindex, uint8_t data, size_t data_size);

/*
 * PDO configuration (verified against: sudo ethercat pdos -p 0):
 *
 * RxPDO 0x1600 (SM2, PhysAddr 0x1400, ControlRegister 0x64, output, 19 bytes)
 *   0x6040:00  Controlword          (16-bit)
 *   0x6060:00  OpMode               ( 8-bit)
 *   0x607A:00  Target pos           (32-bit)
 *   0x60FF:00  Target vel           (32-bit)
 *   0x60FE:01  Phys outputs mask    (32-bit)  vendor, subindex 0x01
 *   0x60FE:02  Phys outputs value   (32-bit)  vendor, subindex 0x02
 *   Total: 6 entries = 152 bits = 19 bytes
 *   NOTE: 0x4D00:01 and 0x4D01:00 are NOT in this PDO â€” use async SDO requests instead
 *
 * TxPDO 0x1A00 (SM3, PhysAddr 0x1600, ControlRegister 0x20, input, 23 bytes)
 *   0x603F:00  Error code           (16-bit)
 *   0x6041:00  Statusword           (16-bit)
 *   0x6061:00  OpMode disp          ( 8-bit)
 *   0x6064:00  Pos actual           (32-bit)
 *   0x60B9:00  Touch stat           (16-bit)
 *   0x60BA:00  Touch pos1           (32-bit)
 *   0x60F4:00  Follow err           (32-bit)
 *   0x4F25:00  Input signal reg     (32-bit)  vendor-specific (replaces 0x60FD)
 *   [Gap 0 bit -- implicit padding]
 */
int configure_minas_a6_pdos(ec_slave_config_t *sc);

size_t uint16Size();
size_t uint32Size();
size_t uint8Size();
size_t unintSize();
size_t int32Size();
size_t int16Size();
size_t int8Size();

/* ---- Domain sizing (MUST be called once before activate) ---- */
int setup_domain_sizing(ec_domain_t *domain,
                        uint16_t alias, uint16_t position,
                        uint32_t vendor_id, uint32_t product_code);

/*
 * get_digital_output_offsets â€” retrieves cached 0x60FE:01/02 domain byte offsets.
 * Must be called after setup_domain_sizing() succeeds.
 * off_mask â†’ 0x60FE:01 (Digital Output Mask)
 * off_val  â†’ 0x60FE:02 (Digital Output Value)
 */
int get_digital_output_offsets(unsigned int *off_mask, unsigned int *off_val);

/*
 * Async SDO Request API for multiturn reset during Op mode.
 *
 * These replace the old PDO-domain approach (which caused PreOp lockup)
 * and the old blocking ecrt_master_sdo_download() approach (deadlock during Op).
 *
 * IgH services ec_sdo_request_t mailbox transactions inside ecrt_master_receive()
 * each cycle â€” no blocking, no deadlock, safe to trigger from any goroutine.
 *
 * SETUP: call create_mt_sdo_requests(sc) from SetupPDOPosition (before activate).
 * TRIGGER: call trigger_mt_request_step() / get_mt_request_state() at any time.
 *
 * Multiturn reset sequence:
 *   1. trigger_mt_request_step(0, 0x0031)      â€” set 0x4D01:00 = function code
 *   2. poll get_mt_request_state(0) == 1        â€” wait for ACK
 *   3. trigger_mt_request_step(1, 0x00000200)   â€” pulse 0x4D00:01 trigger
 *   4. poll get_mt_request_state(1) == 1
 *   5. trigger_mt_request_step(1, 0x00000000)   â€” clear trigger
 *   6. poll get_mt_request_state(1) == 1
 */
int create_mt_sdo_requests(ec_slave_config_t *sc);
int trigger_mt_request_step(int step, uint32_t value);
int get_mt_request_state(int step);

/* ---- TxPDO (feedback) registration helpers ---- */
int setup_pos_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                  uint32_t vendor_id, uint32_t product_code, unsigned int *off_pos);
int setup_statusword_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                         uint32_t vendor_id, uint32_t product_code, unsigned int *off_status);
int setup_error_code_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                         uint32_t vendor_id, uint32_t product_code, unsigned int *off_error);
int setup_velocity_actual_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                              uint32_t vendor_id, uint32_t product_code, unsigned int *off_velocity);
int setup_digital_inputs_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                             uint32_t vendor_id, uint32_t product_code, unsigned int *off_digital_inputs);
int setup_target_torque_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                            uint32_t vendor_id, uint32_t product_code, unsigned int *off_target_torque);

/*
 * setup_all_rx_pdo - registers ALL RxPDO entries from 0x1600 in ONE call.
 * Exposes only offsets for 0x6040/0x6060/0x607A/0x60FF.
 */
int setup_all_rx_pdo(ec_domain_t *domain,
                     uint16_t alias, uint16_t position,
                     uint32_t vendor_id, uint32_t product_code,
                     unsigned int *off_controlword,
                     unsigned int *off_opmode,
                     unsigned int *off_target_pos,
                     unsigned int *off_target_vel);

int32_t read_s32(uint8_t *domain_pd, unsigned int offset);
int16_t read_s16(uint8_t *domain_pd, unsigned int offset);
uint16_t read_u16(uint8_t *domain_pd, unsigned int offset);
uint32_t read_u32(uint8_t *domain_pd, unsigned int offset);
int8_t read_s8(uint8_t *domain_pd, unsigned int offset);

void write_u16(uint8_t *domain_pd, unsigned int offset, uint16_t value);
void write_s16(uint8_t *domain_pd, unsigned int offset, int16_t value);
void write_s32(uint8_t *domain_pd, unsigned int offset, int32_t value);
void write_u32(uint8_t *domain_pd, unsigned int offset, uint32_t value);
void write_s8(uint8_t *domain_pd, unsigned int offset, int8_t value);
/* ---- Async SDO for Profile Velocity (0x6081) ---- */
void *create_profile_vel_sdo_request(ec_slave_config_t *sc);
int trigger_profile_vel_request(void *req_ptr, uint32_t value);
int get_profile_vel_state(void *req_ptr); /* 1=success  0=busy  -1=error  -2=null */

const char *ec_strerror(int err);

#endif /* ETHERCATINTERFACE_H */