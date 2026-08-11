//go:build linux

package main

import "github.com/warthog618/go-gpiocdev"

type gpioLines interface {
	SetValues(values []int) error
	Close() error
}

func requestLines(chipName string, offsets []int, initial []int) (gpioLines, error) {
	return gpiocdev.RequestLines(
		chipName,
		offsets,
		gpiocdev.AsOutput(initial...),
		gpiocdev.WithConsumer("relay-api"),
	)
}
