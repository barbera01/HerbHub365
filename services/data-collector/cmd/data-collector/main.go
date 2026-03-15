package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"herbhub365/services/data-collector/internal/collector"
	"herbhub365/services/data-collector/internal/config"
	"herbhub365/services/data-collector/internal/model"
	"herbhub365/services/data-collector/internal/publisher"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.RunTimeout)
	defer cancel()

	app := collector.New(cfg)
	snapshot, warnings := app.Collect(ctx)

	renderSnapshot(snapshot)
	for _, warning := range warnings {
		log.Printf("warning: %v", warning)
	}

	pub, err := publisher.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		log.Fatalf("connect rabbitmq: %v", err)
	}
	defer pub.Close()

	if err := pub.Publish(ctx, snapshot); err != nil {
		log.Fatalf("publish snapshot: %v", err)
	}

	fmt.Printf("\nPublished snapshot to queue %q\n", cfg.RabbitMQ.QueueName)
	if cfg.EmitJSONToStdout {
		payload, err := model.Marshal(snapshot)
		if err != nil {
			log.Fatalf("marshal snapshot: %v", err)
		}
		fmt.Printf("\n%s\n", payload)
	}

	os.Exit(0)
}

func renderSnapshot(snapshot model.Snapshot) {
	fmt.Println()
	fmt.Println("===== HerbHub Sensor Snapshot =====")
	fmt.Println()
	fmt.Println("Environment")
	printValue("  Temperature", snapshot.Environment.Temperature, "C", 2)
	printValue("  Humidity", snapshot.Environment.Humidity, "%", 1)
	printValue("  Pressure", snapshot.Environment.Pressure, "hPa", 0)
	printValue("  Light", snapshot.Environment.LightLux, "Lux", 1)
	printValue("  Water Level", snapshot.WaterReservoir.DistanceCM, "cm", 2)
	if snapshot.WaterReservoir.PercentFull != nil || snapshot.WaterReservoir.VolumeML != nil {
		fmt.Printf("  Reservoir  : %s full, ~%s ml\n", formatFloat(snapshot.WaterReservoir.PercentFull, 1), formatInt(snapshot.WaterReservoir.VolumeML))
	}

	fmt.Println()
	fmt.Println("Temperatures (C)")
	tempKeys := sortedKeys(snapshot.Temperatures)
	for _, key := range tempKeys {
		fmt.Printf("%-10s : %s C\n", key, formatFloat(snapshot.Temperatures[key], 3))
	}

	fmt.Println()
	fmt.Println("Soil Moisture")
	soilKeys := sortedSoilKeys(snapshot.SoilMoisture)
	for _, key := range soilKeys {
		reading := snapshot.SoilMoisture[key]
		fmt.Printf("%-10s : %s V  (%s%%)\n", key, formatFloat(reading.Voltage, 3), formatFloat(reading.Percent, 1))
	}

	fmt.Printf("\nTimestamp   : %s\n", snapshot.Timestamp.Format(time.RFC3339))
	if snapshot.Device != "" {
		fmt.Printf("Device      : %s\n", snapshot.Device)
	}
	if snapshot.Source != "" {
		fmt.Printf("Source      : %s\n", snapshot.Source)
	}
	if snapshot.CollectedAt != nil {
		fmt.Printf("Collected   : %s\n", snapshot.CollectedAt.Format(time.RFC3339))
	}
	if snapshot.MessageID != "" {
		fmt.Printf("Message ID  : %s\n", snapshot.MessageID)
	}
	if snapshot.Version != "" {
		fmt.Printf("Schema      : %s\n", snapshot.Version)
	}
	if len(snapshot.Warnings) > 0 {
		fmt.Printf("Warnings    : %d\n", len(snapshot.Warnings))
	}
}

func printValue(label string, value *float64, unit string, precision int) {
	if value == nil {
		fmt.Printf("%s : n/a\n", label)
		return
	}
	if unit == "" {
		fmt.Printf("%s : %.*f\n", label, precision, *value)
		return
	}
	fmt.Printf("%s : %.*f %s\n", label, precision, *value, unit)
}

func formatFloat(value *float64, precision int) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.*f", precision, *value)
}

func formatInt(value *int) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%d", *value)
}

func sortedKeys(values map[string]*float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSoilKeys(values map[string]model.SoilMoistureReading) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
