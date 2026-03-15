package model

import (
	"encoding/json"
	"time"
)

type Snapshot struct {
	Timestamp      time.Time                      `json:"timestamp"`
	Source         string                         `json:"source,omitempty"`
	Device         string                         `json:"device,omitempty"`
	Version        string                         `json:"schema_version,omitempty"`
	MessageID      string                         `json:"message_id,omitempty"`
	CollectedAt    *time.Time                     `json:"collected_at,omitempty"`
	Environment    EnvironmentReading             `json:"environment"`
	WaterReservoir WaterReservoirReading          `json:"water_reservoir"`
	Temperatures   map[string]*float64            `json:"temperatures"`
	SoilMoisture   map[string]SoilMoistureReading `json:"soil_moisture"`
	Warnings       []string                       `json:"warnings,omitempty"`
}

type EnvironmentReading struct {
	Temperature *float64 `json:"temperature"`
	Humidity    *float64 `json:"humidity"`
	Pressure    *float64 `json:"pressure"`
	LightLux    *float64 `json:"light_lux"`
}

type WaterReservoirReading struct {
	DistanceCM  *float64 `json:"distance_cm"`
	PercentFull *float64 `json:"percent_full"`
	VolumeML    *int     `json:"volume_ml"`
}

type SoilMoistureReading struct {
	Voltage *float64 `json:"voltage"`
	Percent *float64 `json:"percent"`
}

func Marshal(snapshot Snapshot) ([]byte, error) {
	return json.MarshalIndent(snapshot, "", "  ")
}
