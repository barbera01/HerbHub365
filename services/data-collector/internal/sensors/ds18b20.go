package sensors

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"herbhub365/services/data-collector/internal/config"
)

type DS18B20Reader struct {
	config config.SensorConfig
}

func NewDS18B20Reader(cfg config.SensorConfig) *DS18B20Reader {
	return &DS18B20Reader{config: cfg}
}

func (r *DS18B20Reader) ReadAll(ctx context.Context) (map[string]float64, error) {
	pattern := filepath.Join(r.config.OneWireBasePath, "28-*", "w1_slave")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no one-wire sensors found at %s", pattern)
	}

	sort.Strings(files)
	readings := make(map[string]float64, len(files))
	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		contents, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(contents, []byte("YES")) {
			return nil, fmt.Errorf("crc check failed for %s", file)
		}
		index := bytes.Index(contents, []byte("t="))
		if index == -1 {
			return nil, fmt.Errorf("temperature marker missing in %s", file)
		}
		rawValue := string(bytes.TrimSpace(contents[index+2:]))
		milliCelsius, err := strconv.Atoi(rawValue)
		if err != nil {
			return nil, fmt.Errorf("parse temperature from %s: %w", file, err)
		}
		deviceID := filepath.Base(filepath.Dir(file))
		name := r.config.TempMap[deviceID]
		if name == "" {
			name = deviceID
		}
		readings[name] = float64(milliCelsius) / 1000.0
	}

	return readings, nil
}
