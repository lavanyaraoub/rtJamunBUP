#ifndef ETHERCATINTERFACE_H
#define ETHERCATINTERFACE_H
#include "ecrt.h"
typedef struct MasterOut  {
    const char * result;
} AMasterOut ;
ec_master_t *requestMaster(int index);
int sdo_upload(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t *data, /**< Data buffer to download. */
        size_t data_size, /**< Size of the data buffer. */
        uint32_t *abort_code /**< Abort code of the SDO download. */);

int sdo_download(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t *data, /**< Data buffer to download. */
        size_t data_size, /**< Size of the data buffer. */
        uint32_t *abort_code /**< Abort code of the SDO download. */);

int drivePosition(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t data /**< Data buffer to download. */);

int sdo_upload2(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t data, /**< Data buffer to download. */
        size_t data_size);        

size_t uint16Size();
size_t uint32Size();
size_t uint8Size();
size_t unintSize();
size_t int32Size();
size_t int16Size();
size_t int8Size();
#endif /* !ETHERCATINTERFACE_H */