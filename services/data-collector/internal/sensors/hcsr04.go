package sensors

import (
	"context"
	"fmt"
	"sort"
	"time"

	"herbhub365/services/data-collector/internal/config"
)

type HCSR04Sensor struct {
	config config.SensorConfig
}

func NewHCSR04Sensor(cfg config.SensorConfig) *HCSR04Sensor {
	return &HCSR04Sensor{config: cfg}
}

func (s *HCSR04Sensor) ReadDistanceCM(ctx context.Context) (float64, error) {
	trigger, err := OpenGPIOPin(s.config.GPIOBasePath, s.config.TriggerPin, "out")
	if err != nil {
		return 0, err
	}
	defer trigger.Close()

	echo, err := OpenGPIOPin(s.config.GPIOBasePath, s.config.EchoPin, "in")
	if err != nil {
		return 0, err
	}
	defer echo.Close()

	readings := make([]float64, 0, s.config.HCSR04Samples)
	for i := 0; i < s.config.HCSR04Samples; i++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		distance, err := s.readOnce(trigger, echo)
		if err == nil && distance > 2 && distance < 400 {
			readings = append(readings, distance)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if len(readings) < 3 {
		return 0, fmt.Errorf("need at least 3 valid readings, got %d", len(readings))
	}
	sort.Float64s(readings)
	return readings[len(readings)/2], nil
}

func (s *HCSR04Sensor) readOnce(trigger, echo *GPIOPin) (float64, error) {
	if err := trigger.Write(0); err != nil {
		return 0, err
	}
	time.Sleep(50 * time.Millisecond)
	if err := trigger.Write(1); err != nil {
		return 0, err
	}
	time.Sleep(10 * time.Microsecond)
	if err := trigger.Write(0); err != nil {
		return 0, err
	}

	deadline := time.Now().Add(s.config.HCSR04Timeout)
	for {
		state, err := echo.Read()
		if err != nil {
			return 0, err
		}
		if state == 1 {
			break
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("waiting for echo high timed out")
		}
	}
	start := time.Now()

	for {
		state, err := echo.Read()
		if err != nil {
			return 0, err
		}
		if state == 0 {
			break
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("waiting for echo low timed out")
		}
	}
	duration := time.Since(start)
	return duration.Seconds() * 17150, nil
}
