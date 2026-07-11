package config

import (
	"encoding/json"
	"os"
)

// ChannelEntry describes a single relay channel.
type ChannelEntry struct {
	Name string `json:"name"`
	Pin  int    `json:"pin"`
}

// AddConfig holds settings for the "Add" channel screen.
type AddConfig struct {
	Enabled bool `json:"enabled"`
}

// AdminConfig holds settings for the Admin screen.
type AdminConfig struct {
	Enabled        bool     `json:"enabled"`
	Actions        []string `json:"actions"`
	PromptUsername string   `json:"promptUsername"`
	PromptPassword string   `json:"promptPassword"`
}

// RootConfig is the top-level config structure.
type RootConfig struct {
	Default  DefaultConfig `json:"default"`
	Channels ChannelConfig `json:"channels"`
	Add      AddConfig     `json:"add"`
	Admin    AdminConfig   `json:"admin"`
}

// DefaultConfig holds default runtime settings.
type DefaultConfig struct {
	Host         string `json:"host"`
	PollInterval int    `json:"pollInterval"` // seconds, default 2
}

// ChannelConfig holds the initial channel definitions.
type ChannelConfig struct {
	Enabled bool           `json:"enabled"`
	Entries []ChannelEntry `json:"entries"`
}

// LoadConfig reads and parses config.json from disk.
// Returns a default config if the file is missing or malformed.
func LoadConfig(path string) (RootConfig, error) {
	cfg := RootConfig{
		Default:  DefaultConfig{Host: "http://hh-02:8181", PollInterval: 2},
		Channels: ChannelConfig{Enabled: true, Entries: []ChannelEntry{{Name: "BASIL", Pin: 1}, {Name: "CHILLI", Pin: 2}, {Name: "OREGANO", Pin: 3}}},
		Add:      AddConfig{Enabled: true},
		Admin:    AdminConfig{Enabled: true, Actions: []string{"reset_all", "ping", "version"}},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil // silently fall back to defaults
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, nil // silently fall back to defaults
	}

	return cfg, nil
}
