//go:build !linux

package main

import "errors"

type gpioLines interface {
	SetValues(values []int) error
	Close() error
}

type unsupportedLines struct{}

func (u *unsupportedLines) SetValues(values []int) error {
	return errors.New("gpio unsupported on this platform")
}

func (u *unsupportedLines) Close() error {
	return nil
}

func requestLines(chipName string, offsets []int, initial []int) (gpioLines, error) {
	return nil, errors.New("gpio unsupported on this platform")
}
