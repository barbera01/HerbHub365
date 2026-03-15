package collector

import (
	"context"
	"fmt"
	"sort"
	"time"

	"herbhub365/services/data-collector/internal/config"
	"herbhub365/services/data-collector/internal/model"
	"herbhub365/services/data-collector/internal/sensors"
)

type Collector struct {
	cfg          config.Config
	bme280       *sensors.BME280Sensor
	bh1750       *sensors.BH1750Sensor
	hcsr04       *sensors.HCSR04Sensor
	temperatures *sensors.DS18B20Reader
	ads1115      *sensors.ADS1115Sensor
}

func New(cfg config.Config) *Collector {
	return &Collector{
		cfg:          cfg,
		bme280:       sensors.NewBME280Sensor(cfg.Sensor),
		bh1750:       sensors.NewBH1750Sensor(cfg.Sensor),
		hcsr04:       sensors.NewHCSR04Sensor(cfg.Sensor),
		temperatures: sensors.NewDS18B20Reader(cfg.Sensor),
		ads1115:      sensors.NewADS1115Sensor(cfg.Sensor),
	}
}

func (c *Collector) Collect(ctx context.Context) (model.Snapshot, []error) {
	collectedAt := time.Now().UTC()
	snapshot := model.Snapshot{
		Timestamp:      collectedAt,
		Source:         c.cfg.Source,
		Device:         c.cfg.DeviceName,
		Version:        "1.0",
		MessageID:      fmt.Sprintf("%s-%d", c.cfg.DeviceName, collectedAt.UnixNano()),
		CollectedAt:    &collectedAt,
		Environment:    model.EnvironmentReading{},
		WaterReservoir: model.WaterReservoirReading{},
		Temperatures:   make(map[string]*float64),
		SoilMoisture:   make(map[string]model.SoilMoistureReading),
	}

	var warnings []error
	for _, name := range sortedTempNames(c.cfg.Sensor.TempMap) {
		snapshot.Temperatures[name] = nil
	}
	for _, plant := range sortedPlants(c.cfg.Sensor.MoistureMap) {
		snapshot.SoilMoisture[plant] = model.SoilMoistureReading{}
	}

	if reading, err := c.bme280.Read(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("bme280: %w", err))
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("bme280: %v", err))
	} else {
		snapshot.Environment.Temperature = floatPtr(reading.Temperature)
		snapshot.Environment.Humidity = floatPtr(reading.Humidity)
		snapshot.Environment.Pressure = floatPtr(reading.Pressure)
	}

	if lux, err := c.bh1750.ReadLux(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("bh1750: %w", err))
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("bh1750: %v", err))
	} else {
		snapshot.Environment.LightLux = floatPtr(lux)
	}

	if distance, err := c.hcsr04.ReadDistanceCM(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("hcsr04: %w", err))
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("hcsr04: %v", err))
	} else {
		snapshot.WaterReservoir.DistanceCM = floatPtr(distance)
		percent := sensors.WaterPercent(distance, c.cfg.Sensor.EmptyDistanceCM, c.cfg.Sensor.FullDistanceCM)
		volume := sensors.WaterVolumeML(percent)
		snapshot.WaterReservoir.PercentFull = floatPtr(percent)
		snapshot.WaterReservoir.VolumeML = intPtr(volume)
	}

	if readings, err := c.temperatures.ReadAll(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("ds18b20: %w", err))
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("ds18b20: %v", err))
	} else {
		for name, value := range readings {
			valueCopy := value
			snapshot.Temperatures[name] = &valueCopy
		}
	}

	if readings, err := c.ads1115.ReadAll(ctx); err != nil {
		warnings = append(warnings, fmt.Errorf("ads1115: %w", err))
		snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("ads1115: %v", err))
	} else {
		for plant, reading := range readings {
			voltage := reading.Voltage
			percent := reading.Percent
			snapshot.SoilMoisture[plant] = model.SoilMoistureReading{
				Voltage: &voltage,
				Percent: &percent,
			}
		}
	}

	return snapshot, warnings
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func sortedTempNames(tempMap map[string]string) []string {
	names := make([]string, 0, len(tempMap))
	seen := make(map[string]struct{}, len(tempMap))
	for _, name := range tempMap {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedPlants(moistureMap map[string]string) []string {
	plants := make([]string, 0, len(moistureMap))
	for _, plant := range moistureMap {
		plants = append(plants, plant)
	}
	sort.Strings(plants)
	return plants
}
