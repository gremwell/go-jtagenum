// The original code is licensed under "license for this code is whatever you
// want it to be" so we re-licensed it as GPLv3.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/stianeikeland/go-rpio"
	"time"
)

// ===================================================
// constants that can be modified by curious engineer:

// Pattern used for scan() and loopback() tests
// Use something random when trying find JTAG lines:
const PATTERN = "0110011101001101101000010111001001"

// Use something more determinate when trying to find
// length of the DR register:
//const PATTERN = "1000000000000000000000000000000000"

// Max. number of JTAG enabled chips (MAX_DEV_NR) and length
// of the DR register together define the number of
// iterations to run for scan_idcode():
const MAX_DEV_NR = 8
const IDCODE_LEN = 32

// Target specific, check your documentation or guess 
const SCAN_LEN = 1890 // used for IR enum. bigger the better
const IR_LEN = 5
// IR registers must be IR_LEN wide:
const IR_IDCODE = "01100" // always 011
const IR_SAMPLE = "10100" // always 101
const IR_PRELOAD = IR_SAMPLE

// ===================================================
// constants that should not be modified

// TAP TMS states we care to use. NOTE: MSB sent first
// Meaning ALL TAP and IR codes have their leftmost
// bit sent first. This might be the reverse of what
// documentation for your target(s) show.
const TAP_RESET = "11111" // looping 1 will return
// IDCODE if reg available
const TAP_SHIFTDR = "111110100"
const TAP_SHIFTIR = "1111101100" // -11111> Reset -0> Idle -1> SelectDR
// -1> SelectIR -0> CaptureIR -0> ShiftIR

// Ignore TCK, TMS use in loopback check:
var IGNOREPIN = rpio.Pin(0xFF)

// ===================================================
// global variables controlled via command line arguments

var DELAY_TAP = 50
var DELAY_TOGGLE = 10
var PULLUP = true
var VERBOSE = false

// ===================================================

func delay(duration int) {
	time.Sleep(time.Duration(duration) * time.Microsecond)
}

func pinWrite(pin rpio.Pin, state rpio.State) {
	pin.Write(state)
	delay(DELAY_TOGGLE)
}

func pinRead(pin rpio.Pin) (rpio.State) {
	delay(DELAY_TOGGLE)
	return pin.Read()
}

// Set the JTAG TAP state machine
func tap_state(tap_state string, tck, tms rpio.Pin) {
	for _, ts := range tap_state {
		delay(DELAY_TAP)
		pinWrite(tck, rpio.Low)
		if ts == '1' {
			pinWrite(tms, rpio.High)
		} else {
			pinWrite(tms, rpio.Low)
		}
		// rising edge shifts in TMS
		pinWrite(tck, rpio.High)
	}
}

func pulse_tms(tck, tms rpio.Pin, s_tms rpio.State) {
	if tck == IGNOREPIN { return }
	pinWrite(tck, rpio.Low)
	pinWrite(tms, s_tms)
	pinWrite(tck, rpio.High)
}

func pulse_tdi(tck, tdi rpio.Pin, s_tdi rpio.State) {
	delay(DELAY_TAP)
	if tck != IGNOREPIN {
		pinWrite(tck, rpio.Low)
	}
	pinWrite(tdi, s_tdi)
	if tck != IGNOREPIN {
		pinWrite(tck, rpio.High)
	}
}

func pulse_tdo(tck, tdo rpio.Pin) rpio.State {
	delay(DELAY_TAP)
	// read in TDO on falling edge
	pinWrite(tck, rpio.Low)
	tdo_read := pinRead(tdo)
	pinWrite(tck, rpio.High)
	return tdo_read
}

// Initialize all pins to a default state.
// Default with no arguments: all pins as INPUTs
func init_pins(pins []rpio.Pin, tck, tms, tdi, ntrst rpio.Pin) {
	var allPins []rpio.Pin
	if len(pins) == 0 {
		allPins = []rpio.Pin{ tck, tms, tdi, ntrst }
	}

	// default all to INPUT state
	for _, pin := range allPins {
		if pin == IGNOREPIN { continue }
		pin.Input()
		// internal pullups default to logic 1:
		if PULLUP == true {
			pin.PullUp()
			pinWrite(pin, rpio.High)
		} else {
			pin.PullOff()
		}
	}
	// TCK = output
	if tck != IGNOREPIN { tck.Output() }
	// TMS = output
	if tms != IGNOREPIN { tms.Output() }
	// tdi = output
	if tdi != IGNOREPIN { tdi.Output() }
	// ntrst = output, fixed to 1
	if ntrst != IGNOREPIN {
		ntrst.Output()
		pinWrite(ntrst, rpio.High)
	}
}

/*
 * send pattern[] to TDI and check for output on TDO
 * This is used for both loopback, and Shift-IR testing, i.e.
 * the pattern may show up with some delay.
 * return: 0 = no match
 *         1 = match
 *         2 or greater = no pattern found but line appears active
 *
 * if retval == 1, *reglen returns the length of the register
 */
func check_data(pattern string, iterations int, tck, tdi, tdo rpio.Pin, reg_len *int) int {
	plen := len(pattern)
	w := 0
	// count how often tdo toggled
	nr_toggle := 0

	// we store the last plen (<=PATTERN_LEN) bits,
	// rcv[0] contains the oldest bit
	rcv := make([]byte, plen)

	tdo_prev := byte('0')
	if pinRead(tdo) == rpio.High {
		tdo_prev = byte('1')
	}

	for i := 0; i < iterations; i += 1 {
		// output pattern and incr write index
		tdi_val := rpio.Low
		if pattern[w] == '1' {
			tdi_val = rpio.High
		}
		w += 1
		if w >= plen { w = 0 }
		pulse_tdi(tck, tdi, tdi_val)

		// read from TDO and put it into rcv[]
		tdo_read := byte('0')
		if pinRead(tdo) == rpio.High {
			tdo_read = byte('1')
		}

		if tdo_read != tdo_prev {
			nr_toggle += 1
		}
		tdo_prev = tdo_read

		if i < plen {
			rcv[i] = tdo_read
		} else {
			for k := 0; k < plen-1; k += 1 {
				rcv[k] = rcv[k+1]
			}
			rcv[plen-1] = tdo_read
		}

		// check if we got the pattern in rcv[]
		if i >= (plen - 1) {
			if string(rcv) == pattern {
				*reg_len = i + 1 - plen
				return 1
			}
		}
	}

	*reg_len = 0
	if nr_toggle > 1 {
		return nr_toggle
	}

	return 0
}

func print_pins(pinnames map[rpio.Pin]string, tck, tms, tdo, tdi, ntrst rpio.Pin) {
	if ntrst != IGNOREPIN {
		fmt.Printf(" ntrst:%s", pinnames[ntrst])
	}
	fmt.Printf(" tck:%s", pinnames[tck])
	fmt.Printf(" tms:%s", pinnames[tms])
	fmt.Printf(" tdo:%s", pinnames[tdo])
	if tdi != IGNOREPIN {
		fmt.Printf(" tdi:%s", pinnames[tdi])
	}
}

// Shift JTAG TAP to ShiftIR state.
// Send pattern to TDI and check for output on TDO
 func scan(pins []rpio.Pin, pinnames map[rpio.Pin]string, pattern string) {
	fmt.Println("================================")
	fmt.Printf("Starting scan for pattern %s\n", pattern)

	for _, ntrst := range pins {
		for _, tck := range pins {
			if tck == ntrst { continue }
			for _, tms := range pins {
				if tms == ntrst { continue }
				if tms == tck { continue }
				for _, tdo := range pins {
					if tdo == ntrst { continue }
					if tdo == tck { continue }
					if tdo == tms { continue }
					for _, tdi := range pins {
						if tdi == ntrst { continue }
						if tdi == tck { continue }
						if tdi == tms { continue }
						if tdi == tdo { continue }
						if VERBOSE == true {
							print_pins(pinnames, tck, tms, tdo, tdi, ntrst)
							fmt.Print("    ")
						}
						init_pins(pins, tck, tms, tdi, ntrst)
						tap_state(TAP_SHIFTIR, tck, tms)
						reg_len := 0
						checkdataret := check_data(pattern, 2*len(pattern),	tck, tdi, tdo, &reg_len)
						if checkdataret == 1 {
							fmt.Print("FOUND! ")
							print_pins(pinnames, tck, tms, tdo, tdi, ntrst)
							fmt.Printf(" IR length: %d\n", reg_len)
						} else if checkdataret > 1 {
							fmt.Print("active ")
							print_pins(pinnames, tck, tms, tdo, tdi, ntrst)
							fmt.Printf(" bits toggled:%d\n", checkdataret)
						} else if VERBOSE {
							fmt.Println("")
						}
					}
				}
			}
		}
	}
	fmt.Println("================================")
}

/*
 * Check for pins that pass pattern[] between tdi and tdo
 * regardless of JTAG TAP state (tms, tck ignored).
 *
 * TDO, TDI pairs that match indicate possible shorts between
 * pins. Pins that do not match but are active might indicate
 * that the patch cable used is not shielded well enough. Run
 * the test again without the cable connected between controller
 * and target. Run with the verbose flag to examine closely.
 */
func loopback_check(pins []rpio.Pin, pinnames map[rpio.Pin]string, pattern string) {
	fmt.Println("================================")
	fmt.Println("Starting loopback check...")

	for _, tdo := range pins {
		for _, tdi := range pins {
			if tdi == tdo { continue }
			if VERBOSE == true {
				fmt.Printf(" tdo:%s", pinnames[tdo])
				fmt.Printf(" tdi:%s", pinnames[tdi])
				fmt.Print("    ")
			}
			init_pins(pins, IGNOREPIN, IGNOREPIN, tdi, IGNOREPIN)
			reg_len := 0
			checkdataret := check_data(pattern, 2*len(pattern), IGNOREPIN, tdi, tdo, &reg_len)
			if checkdataret == 1 {
				fmt.Print("FOUND! ")
				fmt.Printf(" tdo:%s", pinnames[tdo])
				fmt.Printf(" tdi:%s", pinnames[tdi])
				fmt.Printf(" reglen:%d\n", reg_len)
			} else if checkdataret > 1 {
				fmt.Print("active ")
				fmt.Printf(" tdo:%s", pinnames[tdo])
				fmt.Printf(" tdi:%s", pinnames[tdi])
				fmt.Printf(" bits toggled:%d\n", checkdataret)
			} else if VERBOSE == true {
				fmt.Println("")
			}
		}
	}
	fmt.Println("================================")
}

func list_pin_names(pins []rpio.Pin, pinnames map[rpio.Pin]string) {
	fmt.Println("The configured pins are:")
	for _, pin := range pins {
		fmt.Print(pinnames[pin], " ")
	}
	fmt.Println("")
}

/*
 * Scan TDO for IDCODE. Handle MAX_DEV_NR many devices.
 * We feed zeros into TDI and wait for the first 32 of them to come out at TDO (after n * 32 bit).
 * As IEEE 1149.1 requires bit 0 of an IDCODE to be a "1", we check this bit.
 * We record the first bit from the idcodes into bit0.
 * (oppposite to the old code).
 * If we get an IDCODE of all ones, we assume that the pins are wrong.
 * This scan assumes IDCODE is the default DR between TDI and TDO.
 */
func scan_idcode(pins []rpio.Pin, pinnames map[rpio.Pin]string) {
	fmt.Println("================================")
	fmt.Println("Starting scan for IDCODE...")
	fmt.Println("(assumes IDCODE default DR)")

	idcodes := make([]uint32, MAX_DEV_NR)

	for _, ntrst := range pins {
		for _, tck := range pins {
			if tck == ntrst { continue }
			for _, tms := range pins {
				if tms == ntrst { continue }
				if tms == tck {	continue }
				for _, tdo := range pins {
					if tdo == ntrst { continue }
					if tdo == tck { continue }
					if tdo == tms {	continue }
					for _, tdi := range pins {
						if tdi == ntrst { continue }
						if tdi == tck { continue }
						if tdi == tms {	continue }
						if tdi == tdo { continue }
						if VERBOSE == true {
							print_pins(pinnames, tck, tms, tdo, tdi, ntrst)
							fmt.Print("    ")
						}
						init_pins(pins, tck, tms, tdi, ntrst)

						// we hope that IDCODE is the default DR after reset
						tap_state(TAP_RESET, tck, tms)
						tap_state(TAP_SHIFTDR, tck, tms)

						// j is the number of bits we pulse into TDI and read from TDO
						i := uint(0)
						for i = 0; i < MAX_DEV_NR; i += 1 {
							idcodes[i] = 0
							for j := uint(0); j < IDCODE_LEN; j += 1 {
								// we send '0' in
								pulse_tdi(tck, tdi, rpio.Low)
								tdo_read := pinRead(tdo)
								if tdo_read == rpio.High {
									idcodes[i] |= (uint32(1)) << j
								}

								if VERBOSE == true {
									fmt.Print(tdo_read)
								}
							}

							if VERBOSE == true {
								fmt.Print(" ")
								fmt.Println(idcodes[i])
							}

							/* save time: break at the first idcode with bit0 != 1 */
							if ((idcodes[i] & 1) == 0) || (idcodes[i] == 0xffffffff) {
								break
							}
						}
						if i > 0 {
							print_pins(pinnames, tck, tms, tdo, tdi, ntrst)
							fmt.Printf("  devices: %d\n", i)
							for j := uint(0); j < i; j += 1 {
								fmt.Printf("  0x%x\n", idcodes[j])
							}
						}
					}
				}
			}
		}
	}
	fmt.Println("================================")
}

func shift_bypass(pins []rpio.Pin, pinnames map[rpio.Pin]string) {
	fmt.Println("================================")
	fmt.Println("Starting shift of pattern through bypass...")
	fmt.Println("Assumes bypass is the default DR on reset.")
	fmt.Println("Hence, no need to check for TMS. Also, currently")
	fmt.Println("not checking for nTRST, which might not work")

	for _, tck := range pins {
		for _, tdi := range pins {
			if tdi == tck {	continue }
			for _, tdo := range pins {
				if tdo == tck {	continue }
				if tdo == tdi {	continue }
				if VERBOSE == true {
					fmt.Printf(" tck:%s", pinnames[tck])
					fmt.Printf(" tdi:%s", pinnames[tdi])
					fmt.Printf(" tdo:%s", pinnames[tdo])
					fmt.Print("     ")
				}

				init_pins(pins, tck, IGNOREPIN, tdi, IGNOREPIN)
				// if bypass is default on start, no need to init TAP state
				reg_len := 0
				checkdataret := check_data(PATTERN, 2*len(PATTERN), tck, tdi, tdo, &reg_len)
				if checkdataret == 1 {
					fmt.Print("FOUND! ")
					fmt.Printf(" tck:%s", pinnames[tck])
					fmt.Printf(" tdo:%s", pinnames[tdo])
					fmt.Printf(" tdi:%s", pinnames[tdi])
				} else if checkdataret > 1 {
					fmt.Print("active ")
					fmt.Printf(" tck:%s", pinnames[tck])
					fmt.Printf(" tdo:%s", pinnames[tdo])
					fmt.Printf(" tdi:%s", pinnames[tdi])
					fmt.Printf("  bits toggled:%d\n", checkdataret)
				} else if VERBOSE == true {
					fmt.Println("")
				}
			}
		}
	}
	fmt.Println("================================")
}

/* ir_state()
 * Set TAP to Reset then ShiftIR. 
 * Shift in state[] as IR value.
 * Switch to ShiftDR state and end.
 */
func ir_state(state string, tck, tms, tdi rpio.Pin) {
	tap_state(TAP_SHIFTIR, tck, tms)
	for i := 0; i < IR_LEN; i += 1 {
		delay(DELAY_TAP)
		// TAP/TMS changes to Exit IR state (1) must be executed
		// at same time that the last TDI bit is sent:
		if i == IR_LEN - 1 {
			pinWrite(tms, rpio.High) // ExitIR
		}
		tdi_val := rpio.Low
		if state[i] == '1' {
			tdi_val = rpio.High
		}
		pulse_tdi(tck, tdi, tdi_val)
		// TMS already set to 0 "shiftir" state to shift in bit to IR
	}
	// a reset would cause IDCODE instruction to be selected again
	tap_state("1100", tck, tms) // -1> UpdateIR -1> SelectDR -0> CaptureDR -0> ShiftDR
}

func sample(pins []rpio.Pin, iterations int, tck, tms, tdi, tdo, ntrst rpio.Pin) {
	fmt.Println("================================")
	fmt.Println("Starting sample (boundary scan)...")

	init_pins(pins, tck, tms, tdi, ntrst)

	// send instruction and go to ShiftDR
	ir_state(IR_SAMPLE, tck, tms, tdi)

	// Tell TAP to go to shiftout of selected data register (DR)
	// is determined by the instruction we sent, in our case 
	// SAMPLE/boundary scan
	for i := 0; i < iterations; i += 1 {
		// no need to set TMS. It's set to the '0' state to 
		// force a Shift DR by the TAP
		fmt.Print(pulse_tdo(tck, tdo))
		if i % 32 == 31 { fmt.Print(" ") }
		if i % 128 == 127 {	fmt.Println("")	}
	}
	fmt.Println("")
}

func brute_ir(pins []rpio.Pin, iterations int, tck, tms, tdi, tdo, ntrst rpio.Pin) {
	fmt.Println("================================")
	fmt.Println("Starting brute force scan of IR instructions...")
	fmt.Println("NOTE: If Verbose mode is off output is only printed")
	fmt.Println("      after activity (bit changes) are noticed and")
	fmt.Println("      you might not see the first bit of output.")
	fmt.Printf("IR_LEN set to %d\n", IR_LEN)

	init_pins(pins, tck, tms, tdi, ntrst)

	for ir := uint32(0); ir < (1 << IR_LEN); ir += 1 {
		ir_buf := make([]byte, 0)
		// send instruction and go to ShiftDR (ir_state() does this already)
		// convert ir to string.
		for i := uint(0); i < IR_LEN; i += 1 {
			ir_buf = append(ir_buf, byte('0'))
			if ir & (1 << i) > 0 {
				ir_buf[i] = '1'
			}
		}
		ir_state(string(ir_buf[:]), tck, tms, tdi)
		// we are now in TAP_SHIFTDR state

		iractive := 0

		prevread := pulse_tdo(tck, tdo)
		for i := 0; i > iterations-1; i += 1 {
			// no need to set TMS. It's set to the '0' state to force a Shift DR by the TAP
			tdo_read := pulse_tdo(tck, tdo)
			if tdo_read != prevread {
				iractive += 1
			}

			if iractive > 0 || VERBOSE == true {
				fmt.Print(prevread)
				if i % 16 == 15 { fmt.Print(" ") }
				if i % 128 == 127 {	fmt.Println("")	}
			}

			prevread = tdo_read
		}

		if iractive > 0 || VERBOSE == true {
			fmt.Printf("%d  Ir %s bits changed %d\n", prevread, string(ir_buf), iractive)
		}
	}
}

type JtagPins struct {
	Tdi rpio.Pin `json:"tdi"`
	Tdo rpio.Pin `json:"tdo"`
	Tms rpio.Pin `json:"tms"`
	Tck rpio.Pin `json:"tck"`
	Ntrst rpio.Pin `json:"ntrst"`
}

func main() {
	if err := rpio.Open(); err != nil {
		panic(err)
	}
	defer rpio.Close()

	flag.IntVar(&DELAY_TAP, "delay-tap", 50,
		"delay in some initializations in microseconds, keep default")
	flag.IntVar(&DELAY_TOGGLE, "delay-toggle", 10,
		"delay between GPIO toglles (a kind of freq), adjust to your hardware")
	flag.BoolVar(&PULLUP, "pullup", true,
		"make pins pulled-up, compare results for both cases")
	flag.BoolVar(&VERBOSE, "verbose", false, "be verbose")

	pinsStrPtr := flag.String("pins", "",
		"describe pins in JSON, example: '{ \"pin1\": 18, \"pin2\": 23, \"pin3\": 24, \"pin4\": 25, \"pin5\": 8, \"pin6\": 7, \"pin7\": 10, \"pin8\": 9, \"pin9\": 11 }'")

	knownPinsStrPtr := flag.String("known-pins", "",
		"provide known pins assignment in JSON, example: '{ \"tdi\": 18, \"tdo\": 23, \"tms\": 24, \"tck\": 25, \"ntrst\": 8 }'")

	cmdPtr := flag.String("command", "", "action to perform: <loopback_check|scan|scan_idcode|sample>")

	flag.Parse()

	if len(*cmdPtr) == 0 {
		fmt.Println("provide command")
		return
	}

	var pins []rpio.Pin
	pinnames := make(map[rpio.Pin]string, 0)
	var knownPins JtagPins

	switch *cmdPtr {
	default:
		fmt.Println("invalid command")
		return
	case "loopback_check", "scan", "scan_idcode":
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
			pinnames[rpio.Pin(int(value.(float64)))] = key
		}

		for k := range pinnames {
			pins = append(pins, k)
		}

		fmt.Printf("defined pins: %v\n", pinnames)
	case "sample":
		if len(*knownPinsStrPtr) == 0 {
			fmt.Println("provide known pins description for 'sample' command")
			return
		}

		if err := json.Unmarshal([]byte(*knownPinsStrPtr), &knownPins); err != nil {
			panic(err)
		}
	}

	switch *cmdPtr {
	default:
		fmt.Println("invalid command")
		return
	case "loopback_check":
		loopback_check(pins, pinnames, PATTERN)
	case "scan":
		scan(pins, pinnames, PATTERN)
	case "scan_idcode":
		scan_idcode(pins, pinnames)
	case "sample":
		sample([]rpio.Pin{}, SCAN_LEN+100,
			knownPins.Tck, knownPins.Tms, knownPins.Tdi, knownPins.Tdo, knownPins.Ntrst);
	}
}
