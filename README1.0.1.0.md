# Introduction 
Master driver controller for Ethercat drivers.

A general purpose driver which can control different ethercat systems.

## Setup Development Environment
### Install golang
```sh
cd /home/pi
wget https://go.dev/dl/go1.17.6.linux-armv6l.tar.gz
sudo tar -C ./ -xzf go1.17.6.linux-armv6l.tar.gz
mkdir ./gosrc
nano ~/.profile 
##and add the below lines to the file
PATH=$PATH:/home/pi/go/bin
GOPATH=/home/pi/gosrc

source ~/.profile
```

Set LDLibrary path
```sh
nano ~/.profile
##and add the below line to the file
export LD_LIBRARY_PATH=/home/pi/gosrc/EtherCAT:$LD_LIBRARY_PATH
source ~/.profile
```
Without the above setup application will not be able to find the libethercatinterface.so file. This file is required to compile the application.
`make c` command will create this so file.

## Building the system
### Packaging
```sh
make package VERSION=x.x.x
```
Make file will compile plugins and main app and package them under ./release directory. VERSION should be the current version of the application.

### Just compilation
```sh
$ make all
```
Will build the main program as well as the commands plugins. This step is really required if any package referenced in commands get modified.

### Run main program
Can use two approaches
```sh
make rb
## or
make run
## or
go run main.go
```

### How to compile the ethercatinterface.c?
ethercatinterface.c is act as an abstraction layer to communicate between Igh Ethercat and this system.

```sh
make c
```

## Commands
In this system commands are plugins. Commands are the handlers of G&M codes. The system doesnt know what are the G&M codes this system can handle. This also isolate any errors from happening to main system from modifying any commands. At run time the system will gather all the commands based on the configuration in `execution.yml` file
```yaml
        - cmd: D**
          func: delay
          description: delay n second
          considerInBlockExecution: 0
```
* cmd: tells which command should execute by this plugin
* func: the command plugin which can handle this code
* description: optional, a small description about the plugin
* considerInBlockExecution: if 1 then execution will halt if the programs are running in single block. In continuous mode this will not have any effect
## Environment settings
All the environment settings are configured in ./configs/envconfig.yaml

* mode: can be release or debug, if the mode is debug all the log messages will send to the console. In release mode logs will logged to log.log file.
* log_level: TRACE, DEBUG, INFO. In Trace logging all logs including trace logs will be captued. If Debug then trace logs will be ignored and capture only debug and info logs. If Info then ignore debug and trace log entries.



## Scan wifi networks using command line
```sh
sudo iwlist wlan0 scanning | grep ESSID
```

## Open chrome browser in full screen
```
sudo nano /etc/xdg/lxsession/LXDE-pi/autostart
update
/usr/bin/chromium-browser --kiosk --disable-restore-session-state http://localhost:8000
```
now the browser will start with the webpage at startup.

## Setting up in a new system
Write the customized ethercat rpi image to the card
> *Never update the Raspbian os using `sudo apt-get upgrade`. Ethercat driver can work only with some specific version of OS for now.*



Once rpi starts, open a shell and do the below steps

```sh
ip link show

1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
2: eth0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc mq state DOWN mode DEFAULT group default qlen 1000
    link/ether dc:a6:32:6a:8d:73 brd ff:ff:ff:ff:ff:ff
3: wlan0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP mode DORMANT group default qlen 1000
    link/ether dc:a6:32:6a:8d:74 brd ff:ff:ff:ff:ff:ff
```
>Copy the value next to link/ether of eth0, e.g dc:a6:32:6a:8d:73

Update the mac address to `/etc/ethercat.conf` and `/opt/etherlab/etc/ethercat.conf`. If this step is missed then Ethercat driver will not be able to detect the slave devices.


```
sudo nano /etc/ethercat.conf
# update MASTER_DEVICE with mac address copied earlier
sample
MASTER0_DEVICE="dc:a6:32:d4:31:3e"

#MASTER1_DEVICE =

#MASTER0_BACKUP=

DEVICE_MODULES="generic"

```
One more place needs updation of mac address
```
$ sudo nano /opt/etherlab/etc/ethercat.conf
and update the MASTER0_DEVICE with the mac address
```

### View Serial number to register for licensing
```
$ cat /proc/cpuinfo
```
>Note down the serial number and feed it via the license creator program

### Start/Stop Jamun system
By default jamun system will start when RPi powered on
```sh
sudo systemctl start jamun.service
sudo systemctl stop jamun.service
```

### Know the version of the system
Go to the installed folder of Jamun, by default the system is installed in `/home/pi/jamun`
```sh
cd /home/pi/jamun
./jamun -v
```
System will display the details like below and exit without proceeding further.
```
Version: 1.0.1.0
Release channel: v1
Loglevel: TRACE
```
### Start/Stop Ethercat driver
By default Ethercat driver starts when RPi powered on
```sh
sudo systemctl stop ethercat
sudo systemctl start ethercat
```

## Copy file from rpi
```
scp pi@192.168.1.31:/home/pi/ftp/jamun_v1.0.3.0.tar.gz D:\tmp\jamun_v1.0.3.0.tar.gz
```

## FTP credentials
```
username: jamunbin
pwd: 1986
```

### Test Ethercat driver can detect masters
```
$ ethercat master

should show the details like below

Master0
  Phase: Operation
  Active: no
  Slaves: 1
  Ethernet devices:
    Main: dc:a6:32:b7:33:1f (attached)
- - - 
- - -
- - -
```
## Troubleshooting
1. Ethercat driver unable to detect the motor driver then 
    - get the mac address of rpi using `ifconfig` command
    - copy the mac address of eth0, e.g. ether `dc:a6:32:6a:8d:73`  txqueuelen 1000  (Ethernet)
    - check mac address updated correctly in `/etc/ethercat.conf` and `/opt/etherlab/etc/ethercat.conf` file
    - driver connected to rpi via ethernet cable. 
    - restart driver and HMI (RPi)

    Jamun system only works if the `ethercat master` command successfully detect a motor driver.

2. File too short error when loading command plugins

    log.log file will have an entry like below
    ```
    level=error msg="[plugin.Open(\"/home/pi/jamun/commands/g69.so\"): /home/pi/jamun/commands/g69.so: file too short]"
    ```
    This can happen after a system update or restore from backup after a failed system update.

    Issue command `ls -al ~/jamun/commands` and we can see the total bytes of the file will be 0

    Jamun will take the backup of system to `/home/pi/jamun_backup/`. Find the latest backup directory say `2021-03-03-17-53-07`. Copy the command plugin which is flagged as error manually to the jamun directory as shown below

    ```
    cp /home/pi/jamun_backup/2021-03-03-17-53-07/commands/g69.so /home/pi/jamun/commands/g69.so
    ```


## Igh Ethercat documentation
url: https://www.etherlab.org/download/ethercat/ethercat-1.5.2.pdf