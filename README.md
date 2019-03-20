# About

This project is aimed to find which pins exposed by the target device are JTAG
pins. It does so by enumerating throughout the provided pins set and trying to
abuse some JTAG features, such as BYPASS and IDCODE registers.

It is written in Go and supposed to be used under Linux (or any OS which Go
supports) on the device with GPIO subsystem supported by `go-rpio`
(https://github.com/stianeikeland/go-rpio) project. Raspberry Pi 1,2,3 is the
most famous example.

Initially this project was a port of JTAGenum
(https://github.com/cyphunk/JTAGenum/) to Golang. Current version has
implementation mostly ported from another great project JTAGulator
(https://github.com/grandideastudio/jtagulator).

For technical documentation refer to the original project:
https://github.com/cyphunk/JTAGenum/blob/master/README.md and comments in the
source code taken from JTAGulator implementation.

# Changes Comparing to JTAGenum

The goal was to just port JTAGenum Arduino project to Go for the following
reasons:
- shell version simply did not work in my case where this tool helped a lot;
- shell version is *very* slow due to `echo 1 > /sys/...gpio` interface;
- Arduino version requires... Arduino controller;
- Arduino version requires source code modification;
- to practice Golan a bit :-)

After porting was finished it became clear that logic behind is not perfect and
produces unstable results. Thus, implementation of the core functions was taken
from JTAGulator. Once features were tested the source code was adopted to Go
coding style.

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
# go-jtagenum -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command check_loopback
defined pins: map[24:pin3 25:pin4 8:pin5 11:pin9 18:pin1 23:pin2 10:pin7 9:pin8 7:pin6]
================================
Starting loopback check...
================================
```

Perform enumeration:
```
# go-jtagenum -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command scan_bypass
defined pins: map[18:pin1 24:pin3 8:pin5 9:pin8 25:pin4 7:pin6 11:pin9 23:pin2 10:pin7]
================================
Starting scan for pattern 0110011101001101101000010111001001
FOUND!  TCK:pin4 TMS:pin3 TDO:pin2 TDI:pin1, possible nTRST: pin5 pin7 
================================
```

Dump IDCODE:
```
# go-jtagenum -pins '{ "pin1": 18, "pin2": 23, "pin3": 24, "pin4": 25, "pin5": 8, "pin6": 7, "pin7": 10, "pin8": 9, "pin9": 11 }' -command scan_idcode
defined pins: map[23:pin2 8:pin5 7:pin6 24:pin3 9:pin8 11:pin9 18:pin1 10:pin7 25:pin4]
================================
Starting scan for IDCODE...
FOUND!  TCK:pin4 TMS:pin3 TDO:pin2
     devices:
        0x0684617f (mfg: 0x0bf (Broadcom), part: 0x6846, ver: 0x0)
        0x5ba00477 (mfg: 0x23b (Solid State System Co., Ltd.), part: 0xba00, ver: 0x5)
        0x0684617f (mfg: 0x0bf (Broadcom), part: 0x6846, ver: 0x0)
     possible nTRST: pin6 pin8 pin9 pin1 pin5 pin7 
================================
```

Verify determined pins:
```
# go-jtagenum -known-pins '{ "tdi": 18, "tdo": 23, "tms": 24, "tck": 25, "trst": 8 }' -command test_bypass
================================
Starting BYPASS test for pattern 0110011101001101101000010111001001
sent pattern: 0110011101001101101000010111001001
recv pattern: 0110011101001101101000010111001001
match!
================================
```

```
# go-jtagenum -known-pins '{ "tdi": 18, "tdo": 23, "tms": 24, "tck": 25, "trst": 8 }' -command test_idcode
================================
Attempting to retreive IDCODE...
devices:
0x0684617f (mfg: 0x0bf (Broadcom), part: 0x6846, ver: 0x0)
0x5ba00477 (mfg: 0x23b (Solid State System Co., Ltd.), part: 0xba00, ver: 0x5)
0x0684617f (mfg: 0x0bf (Broadcom), part: 0x6846, ver: 0x0)
================================
```

## If Something is Not Clear

If tool's output is not clear or not expected, try the following:
- enable pull-up, toggle `-pullup` switch and run the same commands;
- increase toggle delay (`-delay-tck`) and run the same commands;
- increase reset delay (`-delay-reset`) and run the same commands;
- combine previous.

# TODO

There is a room for improvements and several ideas already came to our minds:
- Special mode to adapt GPIO toggle delay;
- Support partially known JTAG pins configuration;
- Support `ftdi` additionally to `go-rpio` as a way to toggle pins.
