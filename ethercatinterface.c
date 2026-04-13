#include <ecrt.h>
#include <stdio.h>
#include "ethercatinterface.h"
//to compile 
// gcc -o libethercatinterface.so -Wall -g -shared -fPIC ethercatinterface.c -I/opt/etherlab/include /opt/etherlab/lib/libethercat.a
ec_master_t *requestMaster(int index) {
    ec_master_t *master0 = NULL;
    master0 = ecrt_request_master(index);
    if(master0==NULL) {
        return NULL;
    }
    // printf("slave count %d", master0->slave_count);
//   int errorCode = 0;
    uint32_t abortCode = 0;

    unsigned long int value = 0xFFFFFFFF;
    size_t resultSize;

    //Get all the params..

    ecrt_master_sdo_upload(master0, 0, 0x60FE,0x01, (unsigned char *)&value, sizeof(value), &resultSize, &abortCode);
    return master0;
}

int sdo_upload(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t *data, /**< Data buffer to download. */
        size_t data_size, /**< Size of the data buffer. */
        uint32_t *abort_code /**< Abort code of the SDO download. */) {
        size_t resultSize=sizeof(data);
            return ecrt_master_sdo_upload(master, slave_position, index,subindex, (unsigned char *)data,data_size, &resultSize, abort_code);
        }

int sdo_download(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t *data, /**< Data buffer to download. */
        size_t data_size, /**< Size of the data buffer. */
        uint32_t *abort_code /**< Abort code of the SDO download. */) {
            //size_t resultSize=sizeof(data);
            return ecrt_master_sdo_download(master, slave_position, index, subindex, (unsigned char *)data, data_size, abort_code);
        }

int drivePosition(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t data /**< Data buffer to download. */) {
            size_t res_size = 0;
            unsigned int stat = data;
            uint32_t abort_code = 0;
            int errorCode=0;
            int i=0;
            for(i=0;i<3;i++) {
                errorCode= ecrt_master_sdo_upload(master, slave_position, index, subindex, (unsigned char *)&stat, sizeof(stat), &res_size, &abort_code);
                if(errorCode>=0) {
                    break;
                }
            }
            return stat;
        }

int sdo_upload2(ec_master_t *master, /**< EtherCAT master. */
        uint16_t slave_position, /**< Slave position. */
        uint16_t index, /**< Index of the SDO. */
        uint8_t subindex, /**< Subindex of the SDO. */
        uint8_t data, /**< Data buffer to download. */
        size_t data_size) {
            size_t res_size = 0;
            unsigned int stat = data;
            uint32_t abort_code = 0;

            ecrt_master_sdo_upload(master, slave_position, index, subindex, (unsigned char *)&stat, sizeof(data_size), &res_size, &abort_code);
            return stat;
        }
        
size_t uint16Size() {
    uint16_t tmp = 2;
    size_t resultSize=sizeof(tmp);
    return resultSize;
}
size_t uint32Size() {
    uint32_t tmp = 2;
    size_t resultSize=sizeof(tmp);
    return resultSize;
}
size_t uint8Size(){
    uint8_t tmp = 2;
    size_t resultSize=sizeof(tmp);
    return resultSize;
}
size_t unintSize() {
    unsigned int tmp = 0xFFFF;
    return sizeof(tmp);
}

size_t int32Size() {
    int32_t tmp=2;
    size_t resultSize = sizeof(tmp);
    return resultSize;
}
size_t int16Size() {
    int16_t tmp=2;
    size_t resultSize = sizeof(tmp);
    return resultSize;
}
size_t int8Size() {
    int8_t tmp=2;
    size_t resultSize = sizeof(tmp);
    return resultSize;
}