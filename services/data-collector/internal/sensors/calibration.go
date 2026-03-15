package sensors

import "math"

func MoisturePercent(voltage, dry, wet float64) float64 {
	if dry == wet {
		return 0
	}
	moisture := (dry - voltage) / (dry - wet)
	if moisture < 0 {
		moisture = 0
	}
	if moisture > 1 {
		moisture = 1
	}
	return math.Round(moisture*1000) / 10
}

func WaterPercent(distance, emptyDistance, fullDistance float64) float64 {
	if emptyDistance == fullDistance {
		return 0
	}
	percent := ((emptyDistance - distance) / (emptyDistance - fullDistance)) * 100
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return math.Round(percent*10) / 10
}

func WaterVolumeML(percent float64) int {
	return int(math.Round(percent * 10))
}
