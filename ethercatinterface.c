#include <ecrt.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <errno.h>
#include "ethercatinterface.h"
// to compile:
// gcc -o libethercatinterface.so -Wall -g -shared -fPIC ethercatinterface.c \
//     -I/opt/etherlab/include /opt/etherlab/lib/libethercat.a

ec_master_t *requestMaster(int index)
{
    ec_master_t *master0 = ecrt_request_master(index);
    if (master0 == NULL)
        return NULL;
    uint32_t abortCode = 0;
    unsigned long int value = 0xFFFFFFFF;
    size_t resultSize;
    ecrt_master_sdo_upload(master0, 0, 0x60FE, 0x01,
                           (unsigned char *)&value, sizeof(value),
                           &resultSize, &abortCode);
    return master0;
}

int sdo_upload(ec_master_t *master, uint16_t slave_position,
               uint16_t index, uint8_t subindex,
               uint8_t *data, size_t data_size, uint32_t *abort_code)
{
    size_t resultSize = data_size;
    return ecrt_master_sdo_upload(master, slave_position, index, subindex,
                                  (unsigned char *)data, data_size,
                                  &resultSize, abort_code);
}

int sdo_download(ec_master_t *master, uint16_t slave_position,
                 uint16_t index, uint8_t subindex,
                 uint8_t *data, size_t data_size, uint32_t *abort_code)
{
    return ecrt_master_sdo_download(master, slave_position, index, subindex,
                                    (unsigned char *)data, data_size, abort_code);
}

int drivePosition(ec_master_t *master, uint16_t slave_position,
                  uint16_t index, uint8_t subindex, uint8_t data)
{
    size_t res_size = 0;
    unsigned int stat = data;
    uint32_t abort_code = 0;
    int errorCode = 0;
    for (int i = 0; i < 3; i++)
    {
        errorCode = ecrt_master_sdo_upload(master, slave_position, index, subindex,
                                           (unsigned char *)&stat, sizeof(stat),
                                           &res_size, &abort_code);
        if (errorCode >= 0)
            break;
    }
    return stat;
}

int sdo_upload2(ec_master_t *master, uint16_t slave_position,
                uint16_t index, uint8_t subindex,
                uint8_t data, size_t data_size)
{
    uint32_t stat = 0;
    uint32_t abort_code = 0;
    size_t resultSize = data_size;
    ecrt_master_sdo_upload(master, slave_position, index, subindex,
                           (unsigned char *)&stat, data_size,
                           &resultSize, &abort_code);
    return (int)stat;
}

/* ============================================================
 * PDO MAPPING -- verified from live drive (ethercat pdos -p 0)
 * after power cycle restoring EEPROM defaults:
 *
 * RxPDO 0x1600 (SM2, PhysAddr 0x1400, ControlRegister 0x64, output):
 *   0x6040:00  Controlword          16-bit
 *   0x6060:00  Modes of Operation    8-bit
 *   0x607A:00  Target position      32-bit
 *   0x60FF:00  Target velocity      32-bit
 *   0x60FE:01  Physical outputs mask  32-bit  vendor
 *   0x60FE:02  Physical outputs value 32-bit  vendor
 *   Total: 152 bits = 19 bytes
 *
 * TxPDO 0x1A00 (SM3, PhysAddr 0x1600, ControlRegister 0x20, input):
 *   0x603F:00  Error code           16-bit
 *   0x6041:00  Statusword           16-bit
 *   0x6061:00  Modes of op display   8-bit
 *   0x6064:00  Position actual      32-bit
 *   0x60B9:00  Touch probe status   16-bit
 *   0x60BA:00  Touch probe pos1     32-bit
 *   0x60F4:00  Following error      32-bit
 *   0x4F25:00  Input signal reg     32-bit  vendor
 * ============================================================ */

/* RxPDO 0x1600 â€” 6 entries matching EEPROM default (verified after power cycle).
 * NOTE: 0x4D01:00 and 0x4D00:01 are intentionally NOT included.
 * Adding them forces IgH to rewrite 0x1600 in the drive during PreOp.
 * The Panasonic A6 rejects this â†’ slave stays in PreOp forever.
 * Multiturn reset uses SDO Request objects (async, safe during Op mode). */
static ec_pdo_entry_info_t minas_a6_rx_entries[] = {
    {0x6040, 0x00, 16},
    {0x6060, 0x00, 8},
    {0x607A, 0x00, 32},
    {0x60FF, 0x00, 32},
    {0x60FE, 0x01, 32},
    {0x60FE, 0x02, 32},
};
#define MINAS_RX_ENTRIES (sizeof(minas_a6_rx_entries) / sizeof(minas_a6_rx_entries[0]))

/* TxPDO 0x1A00 â€” 8 entries matching EEPROM default (verified after power cycle) */
static ec_pdo_entry_info_t minas_a6_tx_entries[] = {
    {0x603F, 0x00, 16},
    {0x6041, 0x00, 16},
    {0x6061, 0x00, 8},
    {0x6064, 0x00, 32},
    {0x60B9, 0x00, 16},
    {0x60BA, 0x00, 32},
    {0x60F4, 0x00, 32},
    {0x4F25, 0x00, 32},
};
#define MINAS_TX_ENTRIES (sizeof(minas_a6_tx_entries) / sizeof(minas_a6_tx_entries[0]))

static ec_pdo_info_t minas_a6_pdos[] = {
    {0x1600, MINAS_RX_ENTRIES, minas_a6_rx_entries},
    {0x1A00, MINAS_TX_ENTRIES, minas_a6_tx_entries},
};

static ec_sync_info_t minas_a6_syncs[] = {
    {0, EC_DIR_OUTPUT, 0, NULL, EC_WD_DISABLE},
    {1, EC_DIR_INPUT, 0, NULL, EC_WD_DISABLE},
    {2, EC_DIR_OUTPUT, 1, &minas_a6_pdos[0], EC_WD_ENABLE},
    {3, EC_DIR_INPUT, 1, &minas_a6_pdos[1], EC_WD_DISABLE},
    {0xff}};

int configure_minas_a6_pdos(ec_slave_config_t *sc)
{
    if (!sc)
        return -EINVAL;
    return ecrt_slave_config_pdos(sc, EC_END, minas_a6_syncs);
}

/* ============================================================
 * Hardcoded domain byte offsets â€” kept for reference ONLY.
 * In Option A we will NOT return these; we will return IgH-computed offsets.
 * ============================================================ */
#define OFF_ERR_CODE 0u     /* 0x603F:00  16-bit  TxPDO */
#define OFF_STATUSWORD 2u   /* 0x6041:00  16-bit  TxPDO */
#define OFF_OPMODE_DISP 4u  /* 0x6061:00   8-bit  TxPDO */
#define OFF_POS_ACTUAL 5u   /* 0x6064:00  32-bit  TxPDO */
#define OFF_DIG_INPUTS 19u  /* 0x4F25:00  32-bit  TxPDO */
#define OFF_CONTROLWORD 23u /* 0x6040:00  16-bit  RxPDO */
#define OFF_OPMODE 25u      /* 0x6060:00   8-bit  RxPDO */
#define OFF_TARGET_POS 26u  /* 0x607A:00  32-bit  RxPDO */
#define OFF_TARGET_VEL 30u  /* 0x60FF:00  32-bit  RxPDO */

/* ============================================================
 * Option A: Dynamic PDO offsets (industrial-grade)
 *
 * IgH computes byte offsets when we call ecrt_domain_reg_pdo_entry_list().
 * These can change if mapping/order changes. We cache and return them.
 * ============================================================ */
static unsigned int g_off_cw = 0;
static unsigned int g_off_op = 0;
static unsigned int g_off_tp = 0;
static unsigned int g_off_tv = 0;
static unsigned int g_off_ec = 0;
static unsigned int g_off_sw = 0;
static unsigned int g_off_pa = 0;
static unsigned int g_off_di = 0;
static int g_off_valid = 0;
static unsigned int g_off_fe1 = 0;  /* 0x60FE:01 Digital Output Mask  */
static unsigned int g_off_fe2 = 0;  /* 0x60FE:02 Digital Output Value */
static unsigned int g_off_4d01 = 0; /* 0x4D01:00 special function setting */
static unsigned int g_off_4d00 = 0; /* 0x4D00:01 special start flag */

/* ============================================================
 * setup_domain_sizing â€” registers all PDO entries with the domain.
 * REQUIRED so domain has non-zero size, and we also cache offsets here.
 * ============================================================ */
int setup_domain_sizing(ec_domain_t *domain,
                        uint16_t alias, uint16_t position,
                        uint32_t vendor_id, uint32_t product_code)
{
    unsigned int _cw, _op, _tp, _tv, _fe1, _fe2;
    unsigned int _ec, _sw, _od, _pa, _tps, _tpp, _fea, _di;

    ec_pdo_entry_reg_t regs[] = {
        /* RxPDO entries (6 entries, EEPROM default) */
        {alias, position, vendor_id, product_code, 0x6040, 0x00, &_cw},
        {alias, position, vendor_id, product_code, 0x6060, 0x00, &_op},
        {alias, position, vendor_id, product_code, 0x607A, 0x00, &_tp},
        {alias, position, vendor_id, product_code, 0x60FF, 0x00, &_tv},
        {alias, position, vendor_id, product_code, 0x60FE, 0x01, &_fe1},
        {alias, position, vendor_id, product_code, 0x60FE, 0x02, &_fe2},
        /* TxPDO entries */
        {alias, position, vendor_id, product_code, 0x603F, 0x00, &_ec},
        {alias, position, vendor_id, product_code, 0x6041, 0x00, &_sw},
        {alias, position, vendor_id, product_code, 0x6061, 0x00, &_od},
        {alias, position, vendor_id, product_code, 0x6064, 0x00, &_pa},
        {alias, position, vendor_id, product_code, 0x60B9, 0x00, &_tps},
        {alias, position, vendor_id, product_code, 0x60BA, 0x00, &_tpp},
        {alias, position, vendor_id, product_code, 0x60F4, 0x00, &_fea},
        {alias, position, vendor_id, product_code, 0x4F25, 0x00, &_di},
        {}};

    int rc = ecrt_domain_reg_pdo_entry_list(domain, regs);
    if (rc != 0)
    {
        fprintf(stderr, "[PDO] setup_domain_sizing: registration failed rc=%d (%s)\n",
                rc, strerror(-rc));
        return rc;
    }

    /* Log what IgH computed */
    fprintf(stdout,
            "[PDO] IgH offsets (verify vs hardcoded): "
            "CW=%u Op=%u TPos=%u TVel=%u | EC=%u SW=%u Pos=%u DI=%u\n",
            _cw, _op, _tp, _tv, _ec, _sw, _pa, _di);
    fflush(stdout);

    /* Cache IgH offsets (Option A) */
    g_off_cw = _cw;
    g_off_op = _op;
    g_off_tp = _tp;
    g_off_tv = _tv;
    g_off_ec = _ec;
    g_off_sw = _sw;
    g_off_pa = _pa;
    g_off_di = _di;
    g_off_fe1 = _fe1;
    g_off_fe2 = _fe2;
    g_off_valid = 1;

    return 0;
}

/* ============================================================
 * get_digital_output_offsets â€” returns cached 0x60FE:01/02 byte offsets.
 * Call after setup_domain_sizing(). Used by Go layer to write physical
 * digital outputs (fin signal, brake solenoid) via PDO instead of SDO.
 * ============================================================ */
int get_digital_output_offsets(unsigned int *off_mask, unsigned int *off_val)
{
    if (!g_off_valid)
        return -EINVAL;
    *off_mask = g_off_fe1;
    *off_val = g_off_fe2;
    return 0;
}
/* get_multiturn_reset_offsets is removed â€” 0x4D01/0x4D00 are not in PDO domain.
 * Use async SDO requests (see create_mt_sdo_requests / trigger_mt_reset). */

/* ============================================================
 * Async SDO Request objects for multiturn reset.
 *
 * IgH ec_sdo_request_t allows SDO writes during Op mode:
 *   - Arm the request from any goroutine (ecrt_sdo_request_write)
 *   - IgH services the mailbox inside ecrt_master_receive() each cycle
 *   - State transitions: UNUSED â†’ QUEUED â†’ BUSY â†’ SUCCESS/ERROR
 *   - No blocking, no deadlock, safe while PDO cyclic is running
 *
 * SETUP (call from SetupPDOPosition, before ecrt_master_activate):
 *   create_mt_sdo_requests(sc) â€” registers both request objects
 *
 * TRIGGER (call from any goroutine, including during Op mode):
 *   trigger_mt_reset_step(step, value) â€” arms one request with a value
 *   get_mt_request_state(step)         â€” polls state (0=unused/busy, 1=success, -1=error)
 * ============================================================ */

static ec_sdo_request_t *g_req_mt_func = NULL;  /* 0x4D01:00 U16 */
static ec_sdo_request_t *g_req_mt_start = NULL; /* 0x4D00:01 U32 */

int create_mt_sdo_requests(ec_slave_config_t *sc)
{
    if (!sc)
        return -EINVAL;

    g_req_mt_func = ecrt_slave_config_create_sdo_request(sc, 0x4D01, 0x00, 2);
    if (!g_req_mt_func)
    {
        fprintf(stderr, "[SDO-REQ] Failed to create SDO request for 0x4D01:00\n");
        return -1;
    }
    ecrt_sdo_request_timeout(g_req_mt_func, 500); /* 500ms timeout */

    g_req_mt_start = ecrt_slave_config_create_sdo_request(sc, 0x4D00, 0x01, 4);
    if (!g_req_mt_start)
    {
        fprintf(stderr, "[SDO-REQ] Failed to create SDO request for 0x4D00:01\n");
        return -1;
    }
    ecrt_sdo_request_timeout(g_req_mt_start, 500);

    fprintf(stdout, "[SDO-REQ] Multiturn SDO request objects created\n");
    fflush(stdout);
    return 0;
}

/* step: 0 = write 0x4D01:00 (func select), 1 = write 0x4D00:01 (trigger/clear)
 *
 * Returns:
 *   0       â€” request armed successfully
 *  -EINVAL  â€” request object not created (create_mt_sdo_requests not called)
 *  -EBUSY   â€” slave mailbox still processing prior request; caller should retry
 *
 * NOTE: After a prior step returns SUCCESS, the slave CoE mailbox may still
 * be in a "releasing" state for 1-2 cycles. Callers must wait (poll with retry)
 * rather than treating -EBUSY as a fatal error.
 */
int trigger_mt_request_step(int step, uint32_t value)
{
    ec_sdo_request_t *req = (step == 0) ? g_req_mt_func : g_req_mt_start;
    if (!req)
    {
        fprintf(stderr, "[MT-SDO] trigger_mt_request_step: req[%d] is NULL\n", step);
        return -EINVAL;
    }

    ec_request_state_t state = ecrt_sdo_request_state(req);
    if (state == EC_REQUEST_BUSY)
    {
        /* Slave mailbox not yet free â€” caller should sleep 1-2ms and retry */
        return -EBUSY;
    }

    /* Write value and arm the request */
    if (step == 0)
    {
        EC_WRITE_U16(ecrt_sdo_request_data(req), (uint16_t)(value & 0xFFFF));
    }
    else
    {
        EC_WRITE_U32(ecrt_sdo_request_data(req), value);
    }
    ecrt_sdo_request_write(req);

    fprintf(stdout, "[MT-SDO] step %d armed: value=0x%08X prior_state=%d\n",
            step, value, (int)state);
    fflush(stdout);
    return 0;
}

/* Returns: 1=success, 0=busy/queued/unused, -1=error, -2=not created */
int get_mt_request_state(int step)
{
    ec_sdo_request_t *req = (step == 0) ? g_req_mt_func : g_req_mt_start;
    if (!req)
        return -2;
    ec_request_state_t s = ecrt_sdo_request_state(req);
    if (s == EC_REQUEST_SUCCESS)
        return 1;
    if (s == EC_REQUEST_ERROR)
    {
        fprintf(stderr, "[MT-SDO] step %d request ERROR from drive\n", step);
        fflush(stderr);
        return -1;
    }
    return 0; /* EC_REQUEST_QUEUED, EC_REQUEST_BUSY, or EC_REQUEST_UNUSED */
}
/* ============================================================
 * Size helpers (must match ethercatinterface.h symbols)
 * ============================================================ */
size_t uint16Size() { return sizeof(uint16_t); }
size_t uint32Size() { return sizeof(uint32_t); }
size_t uint8Size() { return sizeof(uint8_t); }
size_t unintSize() { return sizeof(unsigned int); }
size_t int32Size() { return sizeof(int32_t); }
size_t int16Size() { return sizeof(int16_t); }
size_t int8Size() { return sizeof(int8_t); }

/* ============================================================
 * PDO offset setup functions â€” Option A returns CACHED IgH offsets.
 * (Go must call setup_domain_sizing() first; your code already does.)
 * ============================================================ */
int setup_pos_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                  uint32_t vendor_id, uint32_t product_code,
                  unsigned int *off_pos)
{
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_pos = g_off_pa;
    return 0;
}

int setup_statusword_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                         uint32_t vendor_id, uint32_t product_code,
                         unsigned int *off_status)
{
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_status = g_off_sw;
    return 0;
}

int setup_error_code_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                         uint32_t vendor_id, uint32_t product_code,
                         unsigned int *off_error)
{
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_error = g_off_ec;
    return 0;
}

int setup_velocity_actual_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                              uint32_t vendor_id, uint32_t product_code,
                              unsigned int *off_velocity)
{
    /* Not mapped in current PDO set (TxPDO has no velocity actual).
       Kept for backward compatibility with existing Go calls. */
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_velocity = g_off_pa; /* legacy behavior: return pos offset */
    return 0;
}

int setup_digital_inputs_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                             uint32_t vendor_id, uint32_t product_code,
                             unsigned int *off_digital_inputs)
{
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_digital_inputs = g_off_di;
    return 0;
}

int setup_target_torque_pdo(ec_domain_t *domain, uint16_t alias, uint16_t position,
                            uint32_t vendor_id, uint32_t product_code,
                            unsigned int *off_target_torque)
{
    /* Not mapped in current PDO set. Kept for compatibility. */
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_target_torque = g_off_cw; /* legacy behavior */
    return 0;
}

int setup_all_rx_pdo(ec_domain_t *domain,
                     uint16_t alias, uint16_t position,
                     uint32_t vendor_id, uint32_t product_code,
                     unsigned int *off_controlword, unsigned int *off_opmode,
                     unsigned int *off_target_pos, unsigned int *off_target_vel)
{
    (void)domain;
    (void)alias;
    (void)position;
    (void)vendor_id;
    (void)product_code;
    if (!g_off_valid)
        return -EINVAL;
    *off_controlword = g_off_cw;
    *off_opmode = g_off_op;
    *off_target_pos = g_off_tp;
    *off_target_vel = g_off_tv;
    return 0;
}

/* ============================================================
 * PDO read/write helpers (must match ethercatinterface.h symbols)
 * ============================================================ */
int32_t read_s32(uint8_t *d, unsigned int o) { return EC_READ_S32(d + o); }
uint16_t read_u16(uint8_t *d, unsigned int o) { return EC_READ_U16(d + o); }
uint32_t read_u32(uint8_t *d, unsigned int o) { return EC_READ_U32(d + o); }
int8_t read_s8(uint8_t *d, unsigned int o) { return EC_READ_S8(d + o); }
int16_t read_s16(uint8_t *d, unsigned int o) { return EC_READ_S16(d + o); }

void write_u16(uint8_t *d, unsigned int o, uint16_t v) { EC_WRITE_U16(d + o, v); }
void write_s32(uint8_t *d, unsigned int o, int32_t v) { EC_WRITE_S32(d + o, v); }
void write_u32(uint8_t *d, unsigned int o, uint32_t v) { EC_WRITE_U32(d + o, v); }
void write_s8(uint8_t *d, unsigned int o, int8_t v) { EC_WRITE_S8(d + o, v); }
void write_s16(uint8_t *d, unsigned int o, int16_t v) { EC_WRITE_S16(d + o, v); }

const char *ec_strerror(int err) { return strerror(err); }
/* ============================================================
 * Async SDO Request for Profile Velocity (0x6081)
 * Used to update the feed rate dynamically during Profile Position moves.
 * ============================================================ */
void *create_profile_vel_sdo_request(ec_slave_config_t *sc)
{
    if (!sc)
        return NULL;
    ec_sdo_request_t *req = ecrt_slave_config_create_sdo_request(sc, 0x6081, 0x00, 4);
    if (req)
    {
        ecrt_sdo_request_timeout(req, 500);
    }
    return (void *)req;
}

int trigger_profile_vel_request(void *req_ptr, uint32_t value)
{
    if (!req_ptr)
        return -EINVAL;
    ec_sdo_request_t *req = (ec_sdo_request_t *)req_ptr;

    // If the mailbox is still processing a previous feed-rate update, tell Go to retry
    if (ecrt_sdo_request_state(req) == EC_REQUEST_BUSY)
        return -16; /* EBUSY */

    EC_WRITE_U32(ecrt_sdo_request_data(req), value);
    ecrt_sdo_request_write(req);
    return 0;
}

/* Returns: 1=success, 0=busy/queued/unused, -1=error, -2=NULL pointer */
int get_profile_vel_state(void *req_ptr)
{
    if (!req_ptr)
        return -2;
    ec_sdo_request_t *req = (ec_sdo_request_t *)req_ptr;
    ec_request_state_t s = ecrt_sdo_request_state(req);
    if (s == EC_REQUEST_SUCCESS)
        return 1;
    if (s == EC_REQUEST_ERROR)
        return -1;
    return 0; /* EC_REQUEST_QUEUED or EC_REQUEST_BUSY */
}