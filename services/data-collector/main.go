package main

import (
	"HerbHub365/services/data-collector/sensors/envtemp"
	"HerbHub365/services/data-collector/sensors/moisture"
	"HerbHub365/services/data-collector/sensors/soiltemp"
)

func main() {
	envtemp.PrintName()
	soiltemp.ReadTemp()
	moisture.PrintName()
}
