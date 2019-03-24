package main

import (
	"github.com/stianeikeland/go-rpio"
)

type JtagPinDriverRpio struct {
}

func (d *JtagPinDriverRpio) initDriver() {
	if err := rpio.Open(); err != nil {
		panic(err)
	}
}

func (d *JtagPinDriverRpio) closeDriver() {
	rpio.Close()
}

func (d *JtagPinDriverRpio) pinWrite(pin JtagPin, state JtagPinState) {
	if state == StateHigh {
		rpio.WritePin(rpio.Pin(pin), rpio.High)
	} else {
		rpio.WritePin(rpio.Pin(pin), rpio.Low)
	}
}

func (d *JtagPinDriverRpio) pinRead(pin JtagPin) JtagPinState {
	if rpio.ReadPin(rpio.Pin(pin)) == rpio.High {
		return StateHigh
	} else {
		return StateLow
	}
}

func (d *JtagPinDriverRpio) pinOutput(pin JtagPin) {
	rpio.PinMode(rpio.Pin(pin), rpio.Output)
}

func (d *JtagPinDriverRpio) pinInput(pin JtagPin) {
	rpio.PinMode(rpio.Pin(pin), rpio.Input)
}

func (d *JtagPinDriverRpio) pinPullUp(pin JtagPin) {
	rpio.PullMode(rpio.Pin(pin), rpio.PullUp)
}

func (d *JtagPinDriverRpio) pinPullOff(pin JtagPin) {
	rpio.PullMode(rpio.Pin(pin), rpio.PullOff)
}
