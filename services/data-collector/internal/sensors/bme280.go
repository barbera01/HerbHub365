package sensors

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"herbhub365/services/data-collector/internal/config"
)

type BME280Sensor struct {
	config config.SensorConfig
}

type Environment struct {
	Temperature float64
	Humidity    float64
	Pressure    float64
}

type bme280Calibration struct {
	digT1 uint16
	digT2 int16
	digT3 int16
	digP1 uint16
	digP2 int16
	digP3 int16
	digP4 int16
	digP5 int16
	digP6 int16
	digP7 int16
	digP8 int16
	digP9 int16
	digH1 uint8
	digH2 int16
	digH3 uint8
	digH4 int16
	digH5 int16
	digH6 int8
}

func NewBME280Sensor(cfg config.SensorConfig) *BME280Sensor {
	return &BME280Sensor{config: cfg}
}

func (s *BME280Sensor) Read(ctx context.Context) (Environment, error) {
	device, err := OpenI2CDevice(fmt.Sprintf(s.config.I2CDevicePath, s.config.I2CBus), s.config.BME280Address)
	if err != nil {
		return Environment{}, err
	}
	defer device.Close()

	calibration, err := s.readCalibration(device)
	if err != nil {
		return Environment{}, err
	}
	if err := device.WriteRegister(0xF2, []byte{0x01}); err != nil {
		return Environment{}, err
	}
	if err := device.WriteRegister(0xF4, []byte{0x27}); err != nil {
		return Environment{}, err
	}

	select {
	case <-ctx.Done():
		return Environment{}, ctx.Err()
	case <-time.After(s.config.BME280MeasureDelay):
	}

	raw, err := device.ReadRegister(0xF7, 8)
	if err != nil {
		return Environment{}, err
	}
	if len(raw) != 8 {
		return Environment{}, fmt.Errorf("unexpected read length %d", len(raw))
	}

	adcPressure := int32(raw[0])<<12 | int32(raw[1])<<4 | int32(raw[2])>>4
	adcTemperature := int32(raw[3])<<12 | int32(raw[4])<<4 | int32(raw[5])>>4
	adcHumidity := int32(raw[6])<<8 | int32(raw[7])

	temperature, tFine := compensateTemperature(adcTemperature, calibration)
	pressure := compensatePressure(adcPressure, tFine, calibration)
	humidity := compensateHumidity(adcHumidity, tFine, calibration)

	return Environment{
		Temperature: round(temperature, 2),
		Humidity:    round(humidity, 1),
		Pressure:    round(pressure, 0),
	}, nil
}

func (s *BME280Sensor) readCalibration(device *I2CDevice) (bme280Calibration, error) {
	part1, err := device.ReadRegister(0x88, 25)
	if err != nil {
		return bme280Calibration{}, err
	}
	part2, err := device.ReadRegister(0xE1, 7)
	if err != nil {
		return bme280Calibration{}, err
	}
	return bme280Calibration{
		digT1: binary.LittleEndian.Uint16(part1[0:2]),
		digT2: int16(binary.LittleEndian.Uint16(part1[2:4])),
		digT3: int16(binary.LittleEndian.Uint16(part1[4:6])),
		digP1: binary.LittleEndian.Uint16(part1[6:8]),
		digP2: int16(binary.LittleEndian.Uint16(part1[8:10])),
		digP3: int16(binary.LittleEndian.Uint16(part1[10:12])),
		digP4: int16(binary.LittleEndian.Uint16(part1[12:14])),
		digP5: int16(binary.LittleEndian.Uint16(part1[14:16])),
		digP6: int16(binary.LittleEndian.Uint16(part1[16:18])),
		digP7: int16(binary.LittleEndian.Uint16(part1[18:20])),
		digP8: int16(binary.LittleEndian.Uint16(part1[20:22])),
		digP9: int16(binary.LittleEndian.Uint16(part1[22:24])),
		digH1: part1[24],
		digH2: int16(binary.LittleEndian.Uint16(part2[0:2])),
		digH3: part2[2],
		digH4: signExtend((int(part2[3])<<4)|int(part2[4]&0x0F), 12),
		digH5: signExtend((int(part2[5])<<4)|int(part2[4]>>4), 12),
		digH6: int8(part2[6]),
	}, nil
}

func compensateTemperature(adcTemperature int32, calibration bme280Calibration) (float64, float64) {
	var1 := (float64(adcTemperature)/16384.0 - float64(calibration.digT1)/1024.0) * float64(calibration.digT2)
	var2 := (float64(adcTemperature)/131072.0 - float64(calibration.digT1)/8192.0)
	var2 = var2 * var2 * float64(calibration.digT3)
	tFine := var1 + var2
	return tFine / 5120.0, tFine
}

func compensatePressure(adcPressure int32, tFine float64, calibration bme280Calibration) float64 {
	var1 := tFine/2.0 - 64000.0
	var2 := var1 * var1 * float64(calibration.digP6) / 32768.0
	var2 += var1 * float64(calibration.digP5) * 2.0
	var2 = var2/4.0 + float64(calibration.digP4)*65536.0
	var1 = (float64(calibration.digP3)*var1*var1/524288.0 + float64(calibration.digP2)*var1) / 524288.0
	var1 = (1.0 + var1/32768.0) * float64(calibration.digP1)
	if var1 == 0 {
		return 0
	}
	pressure := 1048576.0 - float64(adcPressure)
	pressure = (pressure - var2/4096.0) * 6250.0 / var1
	var1 = float64(calibration.digP9) * pressure * pressure / 2147483648.0
	var2 = pressure * float64(calibration.digP8) / 32768.0
	pressure += (var1 + var2 + float64(calibration.digP7)) / 16.0
	return pressure / 100.0
}

func compensateHumidity(adcHumidity int32, tFine float64, calibration bme280Calibration) float64 {
	humidity := tFine - 76800.0
	humidity = (float64(adcHumidity) - (float64(calibration.digH4)*64.0 + float64(calibration.digH5)/16384.0*humidity)) *
		(float64(calibration.digH2) / 65536.0 * (1.0 + float64(calibration.digH6)/67108864.0*humidity*(1.0+float64(calibration.digH3)/67108864.0*humidity)))
	humidity *= 1.0 - float64(calibration.digH1)*humidity/524288.0
	if humidity > 100 {
		return 100
	}
	if humidity < 0 {
		return 0
	}
	return humidity
}

func signExtend(value int, bits uint) int16 {
	signBit := 1 << (bits - 1)
	mask := (1 << bits) - 1
	value &= mask
	if value&signBit != 0 {
		value -= 1 << bits
	}
	return int16(value)
}

func round(value float64, precision int) float64 {
	scale := math.Pow10(precision)
	return math.Round(value*scale) / scale
}
