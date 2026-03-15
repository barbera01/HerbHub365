package sensors

import (
	"context"
	"fmt"
	"sort"
	"time"

	"herbhub365/services/data-collector/internal/config"
)

const adsConfigLow = 0x83

type ADS1115Sensor struct {
	config config.SensorConfig
}

type SoilReading struct {
	Voltage float64
	Percent float64
}

func NewADS1115Sensor(cfg config.SensorConfig) *ADS1115Sensor {
	return &ADS1115Sensor{config: cfg}
}

func (s *ADS1115Sensor) ReadAll(ctx context.Context) (map[string]SoilReading, error) {
	device, err := OpenI2CDevice(fmt.Sprintf(s.config.I2CDevicePath, s.config.I2CBus), s.config.ADSAddress)
	if err != nil {
		return nil, err
	}
	defer device.Close()

	readings := make(map[string]SoilReading, len(s.config.MoistureMap))
	for _, channel := range sortedChannels(s.config.MoistureMap) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		plant := s.config.MoistureMap[channel]
		voltage, err := s.readVoltage(device, channel)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", channel, err)
		}
		dry := s.config.DryCalibration[plant]
		wet := s.config.WetCalibration[plant]
		readings[plant] = SoilReading{
			Voltage: voltage,
			Percent: MoisturePercent(voltage, dry, wet),
		}
	}

	return readings, nil
}

func (s *ADS1115Sensor) readVoltage(device *I2CDevice, channel string) (float64, error) {
	configHigh, ok := s.config.ADSChannelConfig[channel]
	if !ok {
		return 0, fmt.Errorf("unknown channel %s", channel)
	}
	if err := device.WriteRegister(0x01, []byte{configHigh, adsConfigLow}); err != nil {
		return 0, err
	}
	time.Sleep(s.config.ADSSettleDelay)
	raw, err := device.ReadRegister(0x00, 2)
	if err != nil {
		return 0, err
	}
	if len(raw) != 2 {
		return 0, fmt.Errorf("unexpected conversion length %d", len(raw))
	}
	value := int16(raw[0])<<8 | int16(raw[1])
	return float64(value) * 4.096 / 32768.0, nil
}

func sortedChannels(values map[string]string) []string {
	channels := make([]string, 0, len(values))
	for channel := range values {
		channels = append(channels, channel)
	}
	sort.Strings(channels)
	return channels
}
