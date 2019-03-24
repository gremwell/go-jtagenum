// The original code (JTAGEnum) is licensed under "license for this code is whatever you
// want it to be" so we re-licensed it as GPLv3.
// However, later this tool was significantly rewritten using ideas and pieces
// of code from JTAGulator project (http://www.jtagulator.com) which is licensed
// under "Creative Commons Attribution 3.0". We believe that we still comply
// this license as we "attribute the work to the original author".

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

// Pattern used for tests
// Use something random when trying find JTAG lines
const PATTERN = "0110011101001101101000010111001001"

// Maximum number of devices allowed in a single JTAG chain
const MAX_DEV_NR = 32

// Minimum length of instruction register per IEEE Std. 1149.1
const MIN_IR_LEN = 2

// Maximum length of instruction register
const MAX_IR_LEN = 32

// Maximum total length of JTAG chain w/ IR selected
const MAX_IR_CHAIN_LEN = MAX_DEV_NR * MAX_IR_LEN

// Maximum length of data register
const MAX_DR_LEN = 1024

const TAP_RESET = "111110"
const TAP_SHIFTDR = "100"
const TAP_SHIFTIR = "1100"

type JtagPin byte
type JtagPinState byte

const (
	StateHigh JtagPinState = 1
	StateLow  JtagPinState = 0
)

type JtagPins struct {
	TDI  JtagPin `json:"tdi"`
	TDO  JtagPin `json:"tdo"`
	TCK  JtagPin `json:"tck"`
	TMS  JtagPin `json:"tms"`
	TRST JtagPin `json:"trst"`
}

type Jtag struct {
	PinNames map[JtagPin]string

	AllPins []JtagPin

	KnownPins JtagPins

	// pins which will be used by methods
	// not putting them into struct just to save some space on typing
	TDI  JtagPin
	TDO  JtagPin
	TCK  JtagPin
	TMS  JtagPin
	TRST JtagPin

	IGNOREPIN JtagPin

	DELAY_TCK   uint
	DELAY_RESET uint
	PULLUP      bool

	drv JtagPinDriver
}

type JtagPinDriver interface {
	initDriver()
	closeDriver()
	pinWrite(JtagPin, JtagPinState)
	pinRead(JtagPin) JtagPinState
	pinOutput(JtagPin)
	pinInput(JtagPin)
	pinPullUp(JtagPin)
	pinPullOff(JtagPin)
}

func delay(us uint) {
	time.Sleep(time.Duration(us) * time.Microsecond)
}

// constructor to create Jtag instance with proper defaults
func NewJtag() Jtag {
	jtag := Jtag{}
	jtag.IGNOREPIN = JtagPin(0xFF)
	jtag.DELAY_TCK = 10
	jtag.DELAY_RESET = 10 * 1000
	jtag.PULLUP = false
	return jtag
}

func (J *Jtag) setJtagDriver(driver JtagPinDriver) {
	J.drv = driver
	J.drv.initDriver()
}

func (J *Jtag) closeJtag() {
	J.drv.closeDriver()
}

func (J *Jtag) pinWriteDelay(pin JtagPin, state JtagPinState) {
	J.drv.pinWrite(pin, state)
	delay(J.DELAY_TCK)
}

func (J *Jtag) pulseTCK(cnt int) {
	for i := 0; i < cnt; i += 1 {
		J.pinWriteDelay(J.TCK, StateHigh)
		J.pinWriteDelay(J.TCK, StateLow)
	}
}

func (J *Jtag) pulseTMS(sTMS JtagPinState) {
	J.drv.pinWrite(J.TMS, sTMS)
	J.pulseTCK(1)
}

func (J *Jtag) setTapState(tapState string) {
	for _, ts := range tapState {
		if ts == '1' {
			J.drv.pinWrite(J.TMS, StateHigh)
		} else {
			J.drv.pinWrite(J.TMS, StateLow)
		}
		J.pulseTCK(1)
	}
}

// initialize pins to a default state.
func (J *Jtag) initPins() {
	// we (probably) have all pins set in AllPins array
	// however, some of them could be explicitly specified as J.TDI, J.TCK, etc
	// thus, we must to initialize all pins to failsafe defaults and specific
	// pins to the apropriate values according to their function
	allPins := J.AllPins
	if len(allPins) == 0 {
		allPins = []JtagPin{J.TCK, J.TMS, J.TDI, J.TDO, J.TRST}
	}

	for _, pin := range allPins {
		if pin == J.IGNOREPIN {
			continue
		}
		J.drv.pinOutput(pin)
		J.drv.pinWrite(pin, StateHigh)
		if J.PULLUP == true {
			J.drv.pinPullUp(pin)
		} else {
			J.drv.pinPullOff(pin)
		}
	}

	if J.TDO != J.IGNOREPIN {
		J.drv.pinInput(J.TDO)
	}

	// set known clock state
	if J.TCK != J.IGNOREPIN {
		J.drv.pinWrite(J.TCK, StateLow)
	}
}

func (J *Jtag) printPins() {
	if J.TRST != J.IGNOREPIN {
		fmt.Printf(" nTRST:%s", J.PinNames[J.TRST])
	}
	if J.TCK != J.IGNOREPIN {
		fmt.Printf(" TCK:%s", J.PinNames[J.TCK])
	}
	if J.TMS != J.IGNOREPIN {
		fmt.Printf(" TMS:%s", J.PinNames[J.TMS])
	}
	if J.TDO != J.IGNOREPIN {
		fmt.Printf(" TDO:%s", J.PinNames[J.TDO])
	}
	if J.TDI != J.IGNOREPIN {
		fmt.Printf(" TDI:%s", J.PinNames[J.TDI])
	}
}

// This method shifts data into the target's Data Register (DR).
// The return value is the value read from the DR.
// TAP must be in Run-Test-Idle state before being called.
// Leaves the TAP in the Run-Test-Idle state.
func (J *Jtag) sendData(pattern []byte) []byte {
	J.setTapState(TAP_SHIFTDR)

	ret := []byte{}
	for i, s := range pattern {
		if s == '1' {
			J.drv.pinWrite(J.TDI, StateHigh)
		} else {
			J.drv.pinWrite(J.TDI, StateLow)
		}
		if J.drv.pinRead(J.TDO) == StateHigh {
			ret = append(ret, '1')
		} else {
			ret = append(ret, '0')
		}
		if i == len(pattern)-1 {
			// Go to Exit1
			J.drv.pinWrite(J.TMS, StateHigh)
		}
		J.pulseTCK(1)
	}

	// Go to Update DR, new data in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	return ret
}

// This method loads the supplied instruction into the target's Instruction Register (IR).
// The return value is the value read from the IR.
// TAP must be in Run-Test-Idle state before being called.
// Leaves the TAP in the Run-Test-Idle state.
func (J *Jtag) sendInstruction(instruction []byte) []byte {
	J.setTapState(TAP_SHIFTIR)

	ret := []byte{}
	for i, s := range instruction {
		if s == '1' {
			J.drv.pinWrite(J.TDI, StateHigh)
		} else {
			J.drv.pinWrite(J.TDI, StateLow)
		}
		if J.drv.pinRead(J.TDO) == StateHigh {
			ret = append(ret, '1')
		} else {
			ret = append(ret, '0')
		}
		if i == len(instruction)-1 {
			// Go to Exit1
			J.drv.pinWrite(J.TMS, StateHigh)
		}
		J.pulseTCK(1)
	}

	// Go to Update IR, new instruction in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	return ret
}

// Run a Bypass through every device in the chain.
// Leaves the TAP in the Run-Test-Idle state.
// pattern -- value to shift into TDI
// returns value received from TDO
func (J *Jtag) sendRecvBypassPattern(devCnt int, pattern []byte) []byte {
	J.setTapState(TAP_RESET)
	J.setTapState(TAP_SHIFTIR)

	// Force all devices in the chain (if they exist) into BYPASS mode using opcode of all 1s
	J.drv.pinWrite(J.TDI, StateHigh)
	J.pulseTCK(devCnt * MAX_IR_LEN)

	// Go to Exit1 IR
	J.pulseTMS(StateHigh)

	// Go to Update IR, new instruction in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	// append some bits to compensate the number of devices on bus
	patternExt := pattern
	for i := 0; i < devCnt; i += 1 {
		patternExt = append(patternExt, '0')
	}
	return J.sendData(patternExt)
}

// Performs a blind interrogation to determine how many devices are connected in the JTAG chain.
// In BYPASS mode, data shifted into TDI is received on TDO delayed by one clock cycle. We can
// force all devices into BYPASS mode, shift known data into TDI, and count how many clock
// cycles it takes for us to see it on TDO.
// Leaves the TAP in the Run-Test-Idle state.
func (J *Jtag) detectDevices() int {
	J.setTapState(TAP_RESET)
	J.setTapState(TAP_SHIFTIR)

	// Force all devices in the chain (if they exist) into BYPASS mode using opcode of all 1s
	J.drv.pinWrite(J.TDI, StateHigh)
	J.pulseTCK(MAX_IR_CHAIN_LEN - 1)

	// Go to Exit1 IR
	J.pulseTMS(StateHigh)

	// Go to Update IR, new instruction in effect
	J.pulseTMS(StateHigh)

	// Go to Select DR Scan
	J.pulseTMS(StateHigh)

	// Go to Capture DR Scan
	J.pulseTMS(StateLow)

	// Go to Shift DR Scan
	J.pulseTMS(StateLow)

	// Send 1s to fill DRs of all devices in the chain (In BYPASS mode, DR length = 1 bit)
	J.pulseTCK(MAX_DEV_NR)

	// We are now in BYPASS mode with all DR set
	// Send in a 0 on TDI and count until we see it on TDO
	J.drv.pinWrite(J.TDI, StateLow)
	devCnt := 0
	for devCnt = 0; devCnt < MAX_DEV_NR; devCnt += 1 {
		if J.drv.pinRead(J.TDO) == StateLow {
			// If we have received our 0, it has propagated through the entire chain (one clock cycle per device in the chain)
			break
		}
		J.pulseTCK(1)
	}

	if devCnt > MAX_DEV_NR-1 {
		devCnt = 0
	}

	// Go to Exit1 DR
	J.pulseTMS(StateHigh)

	// Go to Update DR, new data in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	return devCnt
}

// Performs an interrogation to determine the instruction register length of the target device.
// Limited in length to MAX_IR_LEN.
// Assumes a single device in the JTAG chain.
// Leaves the TAP in the Run-Test-Idle state.
// Returns length of the instruction register
func (J *Jtag) detectIrLength() uint32 {
	// Reset TAP to Run-Test-Idle
	J.setTapState(TAP_RESET)
	// Go to Shift IR
	J.setTapState(TAP_SHIFTIR)

	// Flush the IR
	J.drv.pinWrite(J.TCK, StateLow)
	// Since the length is unknown, send lots of 0s
	J.pulseTCK(MAX_IR_LEN - 1)

	// Once we are sure that the IR is filled with 0s
	// Send in a 1 on TDI and count until we see it on TDO
	J.drv.pinWrite(J.TDI, StateHigh)
	num := uint32(0)
	for num = 0; num < MAX_IR_LEN; num += 1 {
		// If we have received our 1, it has propagated through the entire instruction register
		if J.drv.pinRead(J.TDO) == StateHigh {
			break
		}
		J.pulseTCK(1)
	}

	// If no 1 is received, then we are unable to determine IR length
	if (num > MAX_IR_LEN-1) || (num < MIN_IR_LEN) {
		num = 0
	}

	// Go to Exit1 DR
	J.pulseTMS(StateHigh)

	// Go to Update DR, new data in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	return num
}

// Performs an interrogation to determine the data register length of the target device.
// The selected data register will vary depending on the the instruction.
// Limited in length to MAX_DR_LEN.
// Assumes a single device in the JTAG chain.
// Leaves the TAP in the Run-Test-Idle state.
// opcode -- opcode/instruction to be sent to TAP
// returns length of the data register
func (J *Jtag) detectDrLength(opcode uint32) uint32 {
	// Determine length of TAP IR
	len := J.detectIrLength()
	// Send instruction/opcode (only len bits)
	opcodeStr := []byte{}
	for i := uint32(0); i < len; i += 1 {
		if (opcode | (1 << i)) != 0 {
			opcodeStr = append(opcodeStr, '1')
		} else {
			opcodeStr = append(opcodeStr, '0')
		}
	}
	J.sendInstruction(opcodeStr)
	// Go to Shift IR
	J.setTapState(TAP_SHIFTIR)

	// At this point, a specific DR will be selected, so we can now determine its length.
	// Flush the DR
	J.drv.pinWrite(J.TDI, StateLow)
	J.pulseTCK(MAX_DR_LEN - 1)

	// Once we are sure that the DR is filled with 0s
	// Send in a 1 on TDI and count until we see it on TDO
	J.drv.pinWrite(J.TDI, StateHigh)
	num := uint32(0)
	for num = 0; num < MAX_DR_LEN; num += 1 {
		// If we have received our 1, it has propagated through the entire data register
		if J.drv.pinRead(J.TDO) == StateHigh {
			break
		}
		J.pulseTCK(1)
	}

	// If no 1 is received, then we are unable to determine DR length
	if num > MAX_IR_LEN-1 {
		num = 0
	}

	// Go to Exit1 DR
	J.pulseTMS(StateHigh)

	// Go to Update DR, new data in effect
	J.pulseTMS(StateHigh)

	// Go to Run-Test-Idle
	J.pulseTMS(StateLow)

	return num
}

func (J *Jtag) scanBypass(pattern string) {
	fmt.Println("================================")
	fmt.Printf("Starting scan for pattern %s\n", pattern)
	defer fmt.Println("================================")

	for _, tck := range J.AllPins {
		for _, tms := range J.AllPins {
			if tms == tck {
				continue
			}
			for _, tdo := range J.AllPins {
				if tdo == tck || tdo == tms {
					continue
				}
				for _, tdi := range J.AllPins {
					if tdi == tck || tdi == tms || tdi == tdo {
						continue
					}

					J.TDI = tdi
					J.TDO = tdo
					J.TMS = tms
					J.TCK = tck
					J.TRST = J.IGNOREPIN

					J.initPins()

					devCnt := J.detectDevices()
					if devCnt == 0 || devCnt > MAX_DEV_NR {
						continue
					}

					bitsRecv := J.sendRecvBypassPattern(devCnt, []byte(pattern))
					// we need only last len(pattern) bits
					patternRecv := string(bitsRecv[devCnt:])

					if patternRecv == pattern {
						fmt.Print("FOUND! ")
						J.printPins()

						fmt.Print(", possible nTRST: ")

						// Now try to determine if the TRST# pin is being used on the target
						for _, trst := range J.AllPins {
							if trst == tdi || trst == tdo || trst == tms || trst == tck {
								continue
							}

							J.TRST = trst

							// do reset
							J.drv.pinWrite(J.TRST, StateLow)
							// Give target time to react
							delay(J.DELAY_RESET)

							devCntNew := J.detectDevices()
							// If the new value doesn't match what we already have, then the current pin may be a reset line.
							if devCntNew != devCnt {
								fmt.Printf("%s ", J.PinNames[J.TRST])
							}

							// Bring the current pin HIGH when done
							J.drv.pinWrite(J.TRST, StateHigh)
						}
						fmt.Println("")
					} else {
						fmt.Print("active, ")
						J.printPins()
						fmt.Printf(", wrong data received (%s)\n", patternRecv)
						fmt.Println("       try adjusting frequency, delays, pullup, check hardware connectivity")
					}
				}
			}
		}
	}
}

func (J *Jtag) testBypass(pattern string) {
	fmt.Println("================================")
	fmt.Printf("Starting BYPASS test for pattern %s\n", pattern)
	defer fmt.Println("================================")

	J.TCK = J.KnownPins.TCK
	J.TDO = J.KnownPins.TDO
	J.TDI = J.KnownPins.TDI
	J.TMS = J.KnownPins.TMS
	J.TRST = J.KnownPins.TRST

	J.initPins()

	devCnt := J.detectDevices()
	if devCnt == 0 || devCnt >= MAX_DEV_NR-1 {
		fmt.Println("no devices found")
		return
	}

	bitsRecv := J.sendRecvBypassPattern(devCnt, []byte(pattern))
	// we need only last len(pattern) bits
	patternRecv := string(bitsRecv[devCnt:])

	fmt.Printf("sent pattern: %s\n", pattern)
	fmt.Printf("recv pattern: %s\n", patternRecv)

	if patternRecv == pattern {
		fmt.Println("match!")
	} else {
		fmt.Println("no match")
	}
}

// Retrieves the JTAG device ID from each device in the chain.
// Leaves the TAP in the Run-Test-Idle state.
// The Device Identification register (if it exists) should be immediately available
// in the DR after power-up of the target device or after TAP reset.
// argument -- number of devices in JTAG chain
// returns array of idcodes obtained (they still need verification)
func (J *Jtag) getIdcodes(devCnt int) []uint32 {
	// Reset TAP to Run-Test-Idle
	J.setTapState(TAP_RESET)
	// Go to Shift DR
	J.setTapState(TAP_SHIFTDR)

	idcodes := []uint32{}

	// For each device in the chain...
	for i := 0; i < devCnt; i += 1 {
		// Receive 32-bit value from DR (should be IDCODE if exists), leaves the TAP in Exit1 DR
		idcode := uint32(0)
		for k := 0; k < 32; k += 1 {
			if J.drv.pinRead(J.TDO) == StateHigh {
				idcode |= (1 << uint(k))
			}
			if k == 31 {
				// Go to Exit1
				J.drv.pinWrite(J.TMS, StateHigh)
			}
			J.pulseTCK(1)
		}

		idcodes = append(idcodes, idcode)

		// Go to Pause DR
		J.pulseTMS(StateLow)

		// Go to Exit2 DR
		J.pulseTMS(StateHigh)

		// Go to Shift DR
		J.pulseTMS(StateLow)
	}

	// Reset TAP to Run-Test-Idle
	J.setTapState(TAP_RESET)

	return idcodes
}

func (J *Jtag) scanIdcode() {
	fmt.Println("================================")
	fmt.Println("Starting scan for IDCODE...")
	defer fmt.Println("================================")

	for _, tck := range J.AllPins {
		for _, tms := range J.AllPins {
			if tms == tck {
				continue
			}
			for _, tdo := range J.AllPins {
				if tdo == tck || tdo == tms {
					continue
				}

				J.TCK = tck
				J.TMS = tms
				J.TDO = tdo
				J.TDI = J.IGNOREPIN
				J.TRST = J.IGNOREPIN

				J.initPins()

				// Try to get the 1st Device ID in the chain (if it exists) by reading the DR
				idcodes := J.getIdcodes(1)

				// Ignore if received Device ID is 0xFFFFFFFF or if bit 0 != 1
				if idcodes[0] != 0xFFFFFFFF && (idcodes[0]%2) != 0 {
					fmt.Print("FOUND! ")
					J.printPins()
					fmt.Println("")

					// Since we might not know how many devices are in the chain, try the maximum allowable number and verify the results afterwards
					idcodes = J.getIdcodes(MAX_DEV_NR)

					fmt.Println("     devices:")
					for _, idcode := range idcodes {
						if idcode != 0xFFFFFFFF && (idcode%2) != 0 {
							fmt.Printf("        %s\n", describeIdcode(idcode))
						}
					}

					fmt.Print("     possible nTRST: ")

					// Now try to determine if the TRST# pin is being used on the target
					for _, trst := range J.AllPins {
						if trst == tck || trst == tms || trst == tdo {
							continue
						}

						J.TRST = trst

						// do reset
						J.drv.pinWrite(J.TRST, StateLow)
						// Give target time to react
						delay(J.DELAY_RESET)

						// Try to get Device ID again by reading the DR (1st in the chain)
						idcodesNew := J.getIdcodes(1)
						// If the new value doesn't match what we already have, then the current pin may be a reset line.
						if len(idcodesNew) != len(idcodes) || (idcodesNew[0] != idcodes[0]) {
							fmt.Printf("%s ", J.PinNames[J.TRST])
						}

						// Bring the current pin HIGH when done
						J.drv.pinWrite(J.TRST, StateHigh)
					}
					fmt.Println("")
				}
			}
		}
	}
}

// Check for pins that pass pattern[] between tdi and tdo
// regardless of JTAG TAP state (tms, tck ignored).
// TDO, TDI pairs that match indicate possible shorts between
// pins. Pins that do not match but are active might indicate
// that the patch cable used is not shielded well enough. Run
// the test again without the cable connected between controller
// and target. Run with the verbose flag to examine closely.
func (J *Jtag) checkLoopback(pattern string) {
	fmt.Println("================================")
	fmt.Println("Starting loopback check...")
	defer fmt.Println("================================")

	for _, tdo := range J.AllPins {
		for _, tdi := range J.AllPins {
			if tdi == tdo {
				continue
			}

			J.TDI = tdi
			J.TDO = tdo
			J.TRST = J.IGNOREPIN
			J.TCK = J.IGNOREPIN
			J.TMS = J.IGNOREPIN

			J.initPins()

			recv := []byte{}
			for _, s := range pattern {
				if s == '1' {
					J.drv.pinWrite(J.TDI, StateHigh)
				} else {
					J.drv.pinWrite(J.TDI, StateLow)
				}
				if J.drv.pinRead(J.TDO) == StateHigh {
					recv = append(recv, '1')
				} else {
					recv = append(recv, '0')
				}
			}

			if string(recv) == pattern {
				fmt.Printf("possible short detected between %s and %s\n", J.PinNames[J.TDO], J.PinNames[J.TDI])
			} else {
				for i := 1; i < len(recv); i += 1 {
					if recv[i] != recv[0] {
						fmt.Printf("possible interconnection (check cable) detected between %s and %s\n", J.PinNames[J.TDO], J.PinNames[J.TDI])
						return
					}
				}
			}
		}
	}
}

func (J *Jtag) testIdcode() {
	fmt.Println("================================")
	fmt.Println("Attempting to retreive IDCODE...")
	defer fmt.Println("================================")

	J.TDI = J.KnownPins.TDI
	J.TDO = J.KnownPins.TDO
	J.TCK = J.KnownPins.TCK
	J.TMS = J.KnownPins.TMS
	J.TRST = J.KnownPins.TRST

	J.initPins()

	// Since we might not know how many devices are in the chain, try the maximum allowable number and verify the results afterwards
	idcodes := J.getIdcodes(MAX_DEV_NR)

	fmt.Println("devices:")

	// For each device in the chain...
	for _, idcode := range idcodes {
		// Ignore if received Device ID is 0xFFFFFFFF or if bit 0 != 1
		if idcode != 0xFFFFFFFF && (idcode%2) != 0 {
			fmt.Println(describeIdcode(idcode))
		}
	}
}

func (J *Jtag) discoverOpcode() {
	fmt.Println("================================")
	fmt.Println("Attempting to retreive IDCODE...")
	defer fmt.Println("================================")

	J.TDI = J.KnownPins.TDI
	J.TDO = J.KnownPins.TDO
	J.TCK = J.KnownPins.TCK
	J.TMS = J.KnownPins.TMS
	J.TRST = J.KnownPins.TRST

	J.initPins()

	// Get number of devices in the chain
	devCnt := J.detectDevices()
	if devCnt == 0 {
		fmt.Println("no devices in chain")
		return
	} else if devCnt > 1 {
		fmt.Println("more than one device in chain")
		return
	}

	irlen := J.detectIrLength()
	fmt.Print("IR length: ")
	if irlen == 0 {
		fmt.Println("N/A")
		return
	} else {
		fmt.Println(irlen)
	}

	opcodeMax := uint32((1 << irlen) - 1)
	fmt.Printf("Possible instructions: %d\n", opcodeMax)

	// For every possible instruction...
	for opcode := uint32(0); opcode < opcodeMax; opcode += 1 {
		// Get the DR length
		drlen := J.detectDrLength(opcode)
		// ignore 1-bit instructions
		if drlen > 1 {
			// Display the result
			fmt.Printf("%s\n", describeIrDr(irlen, opcode, drlen))
		}
	}

	// Reset TAP to Run-Test-Idle
	J.setTapState(TAP_RESET)
}

func (J *Jtag) boundaryScan() {
	fmt.Println("================================")
	fmt.Println("Starting boundary scan...")
	defer fmt.Println("================================")

	J.TDI = J.KnownPins.TDI
	J.TDO = J.KnownPins.TDO
	J.TCK = J.KnownPins.TCK
	J.TMS = J.KnownPins.TMS
	J.TRST = J.KnownPins.TRST

	J.initPins()

	// Get number of devices in the chain
	devCnt := J.detectDevices()
	if devCnt == 0 {
		fmt.Println("no devices in chain")
		return
	} else if devCnt > 1 {
		fmt.Println("more than one device in chain, not supported")
		return
	}

	// Determine length of TAP IR
	irLen := J.detectIrLength()
	// IR registers must be IR_LEN wide:
	irSample := []byte{'1', '0', '1'}
	if irLen > uint32(len(irSample)) {
		for i := uint32(0); i < irLen-3; i += 1 {
			irSample = append(irSample, '0')
		}
	}

	// send instruction and go to ShiftDR
	J.sendInstruction(irSample)

	// Tell TAP to go to shiftout of selected data register (DR)
	// is determined by the instruction we sent, in our case
	// SAMPLE/boundary scan
	for i := 0; i < 2000; i += 1 {
		// no need to set TMS. It's set to the '0' state to
		// force a Shift DR by the TAP
		if J.drv.pinRead(J.TDO) == StateHigh {
			fmt.Print('1')
		} else {
			fmt.Print('0')
		}
		J.pulseTCK(1)
		if i%32 == 31 {
			fmt.Print(" ")
		}
		if i%128 == 127 {
			fmt.Println("")
		}
	}
	fmt.Println("")

	// Reset TAP to Run-Test-Idle
	J.setTapState(TAP_RESET)
}

func describeIdcode(idcode uint32) string {
	mfg := (idcode & 0xffe) >> 1
	part := (idcode & 0xffff000) >> 12
	ver := (idcode & 0xf0000000) >> 28
	bank := (idcode & 0xf00) >> 8
	id := (idcode & 0xfe) >> 1
	mfgName := Jep106Manufacturer(bank, id)

	return fmt.Sprintf("0x%08x (mfg: 0x%3.3x (%s), part: 0x%4.4x, ver: 0x%1.1x)",
		idcode, mfg, mfgName, part, ver)
}

func describeIrDr(irlen, opcode, drlen uint32) string {
	ret := ""
	// Display current instruction
	ret += "IR: "

	// ...as binary characters (0/1)
	for i := uint32(0); i < irlen; i += 1 {
		if (opcode & (1 << i)) == 0 {
			ret += "0 "
		} else {
			ret += "1 "
		}
	}

	// ...as hexadecimal
	mask := (1 << 4 * ((irlen + 4) / 4)) - 1
	ret += fmt.Sprintf("(0x%08x)", opcode&mask)

	// Display DR length as a decimal value
	ret += fmt.Sprintf(" -> DR: %d", drlen)

	return ret
}

func main() {
	jtag := NewJtag()
	defer jtag.closeJtag()

	flag.UintVar(&(jtag.DELAY_TCK), "delay-tck", 10,
		"delay after TCK toggle in microseconds (a kind of frequency)")
	flag.UintVar(&(jtag.DELAY_RESET), "delay-reset", 10*1000,
		"delay of reset pulse on TRST pin in microseconds")
	flag.BoolVar(&(jtag.PULLUP), "pullup", false,
		"make pins pulled-up, compare results for both cases")

	pinsStrPtr := flag.String("pins", "",
		"describe pins in JSON, example: '{ \"pin1\": 18, \"pin2\": 23, \"pin3\": 24, \"pin4\": 25, \"pin5\": 8, \"pin6\": 7, \"pin7\": 10, \"pin8\": 9, \"pin9\": 11 }'")

	knownPinsStrPtr := flag.String("known-pins", "",
		"provide known pins assignment in JSON, example: '{ \"tdi\": 18, \"tdo\": 23, \"tms\": 24, \"tck\": 25, \"trst\": 8 }'")

	cmdPtr := flag.String("command", "", "action to perform: <check_loopback|scan_bypass|test_bypass|scan_idcode|test_idcode|boundary_scan|discover_opcode>")

	drvPtr := flag.String("driver", "rpio", "drive GPIO via: <rpio|gpiod>")
	gpiodChip := uint(0)
	flag.UintVar(&(gpiodChip), "gpiochip", 0,
		"GPIO chip number to take pins from one of /dev/gpiochipX, used by 'gpiod' driver")

	flag.Parse()

	if len(*cmdPtr) == 0 {
		fmt.Println("provide command")
		return
	}

	jtag.PinNames = make(map[JtagPin]string, 0)
	jtag.KnownPins = JtagPins{}

	switch *cmdPtr {
	default:
		fmt.Println("invalid command")
		return
	case "check_loopback", "scan_bypass", "scan_idcode":
		if len(*pinsStrPtr) == 0 {
			fmt.Println("provide pins description")
			return
		}

		var pinsJson map[string]interface{}
		if err := json.Unmarshal([]byte(*pinsStrPtr), &pinsJson); err != nil {
			panic(err)
		}

		for key, value := range pinsJson {
			// the following will fail with panic if input is garbage
			jtag.PinNames[JtagPin(int(value.(float64)))] = key
		}

		for k := range jtag.PinNames {
			jtag.AllPins = append(jtag.AllPins, k)
		}

		fmt.Printf("defined pins: %v\n", jtag.PinNames)
	case "test_bypass", "boundary_scan", "test_idcode", "discover_opcode":
		if len(*knownPinsStrPtr) == 0 {
			fmt.Printf("provide known pins description for %s command\n", *cmdPtr)
			return
		}

		if err := json.Unmarshal([]byte(*knownPinsStrPtr), &jtag.KnownPins); err != nil {
			panic(err)
		}
	}

	switch *drvPtr {
	default:
		drv := &JtagPinDriverRpio{}
		jtag.setJtagDriver(drv)
	case "rpio":
		drv := &JtagPinDriverRpio{}
		jtag.setJtagDriver(drv)
	case "gpiod":
		drv := &JtagPinDriverGpiod{}
		jtag.setJtagDriver(drv)
	}

	switch *cmdPtr {
	default:
		fmt.Println("invalid command")
		return
	case "check_loopback":
		jtag.checkLoopback(PATTERN)
	case "scan_bypass":
		jtag.scanBypass(PATTERN)
	case "test_bypass":
		jtag.testBypass(PATTERN)
	case "scan_idcode":
		jtag.scanIdcode()
	case "test_idcode":
		jtag.testIdcode()
	case "boundary_scan":
		jtag.boundaryScan()
	case "discover_opcode":
		jtag.discoverOpcode()
	}
}
