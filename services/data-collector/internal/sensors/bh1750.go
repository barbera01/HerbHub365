package sensors

import (
	"context"
	"fmt"
	"time"

	"herbhub365/services/data-collector/internal/config"
)

type BH1750Sensor struct {
	config config.SensorConfig
}

func NewBH1750Sensor(cfg config.SensorConfig) *BH1750Sensor {
	return &BH1750Sensor{config: cfg}
}

func (s *BH1750Sensor) ReadLux(ctx context.Context) (float64, error) {
	device, err := OpenI2CDevice(fmt.Sprintf(s.config.I2CDevicePath, s.config.I2CBus), s.config.BH1750Address)
	if err != nil {
		return 0, err
	}
	defer device.Close()

	if err := device.Write([]byte{0x10}); err != nil {
		return 0, err
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(s.config.BH1750Measurement):
	}

	raw, err := device.Read(2)
	if err != nil {
		return 0, err
	}
	if len(raw) != 2 {
		return 0, fmt.Errorf("unexpected read length %d", len(raw))
	}
	value := float64(uint16(raw[0])<<8|uint16(raw[1])) / 1.2
	return value, nil
}
