// Package config manages the Jade application configuration file at
// ~/.jade/config.json. It exposes a simple Load/Save API with atomic writes.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// MaxRecentVaults is the maximum number of recent vault paths kept in config.
const MaxRecentVaults = 10

// WindowBounds stores the last-known window position and size.
type WindowBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Config is the application configuration persisted to disk.
type Config struct {
	RecentVaults    []string      `json:"recentVaults"`
	LastOpenedVault string        `json:"lastOpenedVault"`
	WindowBounds    *WindowBounds `json:"windowBounds,omitempty"`
}

// DefaultPath returns the path to ~/.jade/config.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".jade", "config.json"), nil
}

// Load reads config from path. If the file does not exist, an empty Config is
// returned with no error. Corrupt JSON is treated as empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Treat corrupt config as empty rather than hard-failing.
		return &Config{}, nil
	}
	return &cfg, nil
}

// Save writes cfg to path using a temp-file + atomic rename.
// The parent directory is created if it does not exist.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// AddRecentVault prepends path to cfg.RecentVaults, deduplicates, and trims
// the list to at most MaxRecentVaults entries.
func AddRecentVault(cfg *Config, path string) {
	// Build deduplicated list without path.
	filtered := cfg.RecentVaults[:0]
	for _, v := range cfg.RecentVaults {
		if v != path {
			filtered = append(filtered, v)
		}
	}
	// Prepend.
	cfg.RecentVaults = append([]string{path}, filtered...)
	// Trim.
	if len(cfg.RecentVaults) > MaxRecentVaults {
		cfg.RecentVaults = cfg.RecentVaults[:MaxRecentVaults]
	}
}

// atomicWrite writes data to path via a temp file + rename in the same directory.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".jade-cfg-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
