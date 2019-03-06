# About

This project is a port of JTAGenum (https://github.com/cyphunk/JTAGenum/) to the
Golang. It is supposed to be used under Linux (or any OS which Go supports) on
the device with GPIO subsystem supported by `go-rpio`
(https://github.com/stianeikeland/go-rpio) project. Raspberry Pi 1,2,3 is the
most famous example.

For technical documentation refer to the original project:
https://github.com/cyphunk/JTAGenum/blob/master/README.md

Source code modification may be needed but is not required.

# Changes Comparing to JTAGenum

The goal was to just port JTAGenum Arduino project to Golang for the following
reasons:
- shell version simply did not work in my case where this tool helped a lot;
- shell version is *very* slow due to `echo 1 > /sys/...gpio` interface;
- Arduino version requires... Arduino controller;
- Arduino version requires source code modification;
- to practice Golan a bit :-)

Changes:
- Algorithms, variables names, functions are kept the same as much as possible;
- Pins configuration and other parameters are provided via command line options;
- Introduced GPIO toggle delay (a kind of frequency) which is not required for
  Arduino or shell versions as they are too slow.

# Usage

## Hardware Part

Investigate your target and try to determine JTAG pins in hardware way. This
will help to analyse this tool's output.

Do the required wiring to connect JTAG pins with GPIOs on your board (which runs
this tool).

Write-down GPIO pin numbers (as OS understands them) and give each number unique
identifier.

## Software Part

Again, for technical documentation refer to the original project:
https://github.com/cyphunk/JTAGenum/blob/master/README.md

Prepare pins configuration in JSON format, the following example is
self-descriptive:
```
{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }`
```

Check for loops:
```
# go run jtagenum.go -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command loopback_check
defined pins: map[7:pin6 10:pin7 11:pin9 18:pin1 25:pin4 23:pin2 24:pin3 8:pin5 9:pin8]
================================
Starting loopback check...
active  tdo:pin2 tdi:pin1 bits toggled:31
================================
```

Perform enumeration:
```
# go run jtagenum.go -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command scan
defined pins: map[18:pin1 23:pin2 10:pin7 11:pin9 25:pin4 7:pin6 24:pin3 8:pin5 9:pin8]
================================
Starting scan for pattern 0110011101001101101000010111001001
active  ntrst:pin5 tck:pin4 tms:pin6 tdo:pin2 tdi:pin3 bits toggled:5
active  ntrst:pin5 tck:pin4 tms:pin3 tdo:pin2 tdi:pin6 bits toggled:6
active  ntrst:pin5 tck:pin4 tms:pin3 tdo:pin2 tdi:pin8 bits toggled:6
active  ntrst:pin5 tck:pin4 tms:pin3 tdo:pin2 tdi:pin1 bits toggled:20
active  ntrst:pin5 tck:pin4 tms:pin8 tdo:pin2 tdi:pin3 bits toggled:5
active  ntrst:pin5 tck:pin4 tms:pin1 tdo:pin2 tdi:pin3 bits toggled:5
active  ntrst:pin5 tck:pin4 tms:pin9 tdo:pin2 tdi:pin3 bits toggled:5
================================
```

Dump IDCODE:
```
# go run jtagenum.go -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command scan_idcode
================================
Starting scan for IDCODE...
(assumes IDCODE default DR)
 ntrst:pin5 tck:pin4 tms:pin3 tdo:pin2 tdi:pin1  devices: 3
  0x68[redacted]
  0x5b[redacted]
  0x68[redacted]
================================
```

## If Something is Not Clear

If tool's output is not clear or not expected, try the following:
- enable pull-up, toggle `-pullup` switch and run the same commands;
- increase toggle delay (`-delay-toggle`) and run the same commands;
- increase TAP setup delay (`-delay-tap`) and run the same commands;
- combine previous.

# TODO

There is a room for improvements and several ideas already came to our minds:
- Parsing IDCODE for (at least) vendor name;
- Special mode to adapt GPIO toggle delay;
- Support partially known JTAG pins configuration;
- Support `ftdi` additionally to `go-rpio` as a way to toggle pins.
