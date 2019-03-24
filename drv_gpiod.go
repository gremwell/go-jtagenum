package main

// #cgo pkg-config: libgpiod
// #include <gpiod.h>
import "C"
import (
	"fmt"
)

type JtagPinDriverGpiod struct {
	GpioChip uint
	ctx      *C.struct_gpiod_chip
	lines    map[JtagPin]*C.struct_gpiod_line
}

func (d *JtagPinDriverGpiod) initDriver() {
	d.ctx = C.gpiod_chip_open_by_number(C.uint(d.GpioChip))
	if d.ctx == nil {
		panic(fmt.Sprintf("can't open gpio chip #%d", d.GpioChip))
	}
	d.lines = make(map[JtagPin]*C.struct_gpiod_line, 0)
}

func (d *JtagPinDriverGpiod) closeDriver() {
	for _, v := range d.lines {
		C.gpiod_line_release(v)
	}
	C.gpiod_chip_close(d.ctx)
}

func (d *JtagPinDriverGpiod) getAllocLine(pin JtagPin) *C.struct_gpiod_line {
	l, ok := d.lines[pin]
	if !ok {
		l = C.gpiod_chip_get_line(d.ctx, C.uint(pin))
		if l == nil {
			panic(fmt.Sprintf("can't reserve pin #%d", pin))
		}
		d.lines[pin] = l
	}
	return l
}

func (d *JtagPinDriverGpiod) pinWrite(pin JtagPin, state JtagPinState) {
	C.gpiod_line_set_value(d.getAllocLine(pin), C.int(state))
}

func (d *JtagPinDriverGpiod) pinRead(pin JtagPin) JtagPinState {
	v := C.gpiod_line_get_value(d.getAllocLine(pin))
	if v == -1 {
		panic(fmt.Sprintf("can't get pin #%d value", pin))
	}
	return JtagPinState(v)
}

func (d *JtagPinDriverGpiod) pinOutput(pin JtagPin) {
	l, ok := d.lines[pin]
	if ok {
		C.gpiod_line_release(l)
	}
	l = d.getAllocLine(pin)
	C.gpiod_line_request_output(l, C.CString("jtagenum"), 1)
}

func (d *JtagPinDriverGpiod) pinInput(pin JtagPin) {
	l, ok := d.lines[pin]
	if ok {
		C.gpiod_line_release(l)
	}
	l = d.getAllocLine(pin)
	C.gpiod_line_request_input(l, C.CString("jtagenum"))
}

func (d *JtagPinDriverGpiod) pinPullUp(pin JtagPin) {
}

func (d *JtagPinDriverGpiod) pinPullOff(pin JtagPin) {
}
