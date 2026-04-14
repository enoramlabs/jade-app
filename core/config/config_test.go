package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/enoramlabs/jade-app/core/config"
)

// Test: Load returns empty Config when file does not exist.
func TestLoad_returns_empty_config_when_file_missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil Config")
	}
	if len(cfg.RecentVaults) != 0 {
		t.Errorf("RecentVaults should be empty for missing file, got %v", cfg.RecentVaults)
	}
	if cfg.LastOpenedVault != "" {
		t.Errorf("LastOpenedVault should be empty for missing file, got %q", cfg.LastOpenedVault)
	}
}

// Test: Save and Load round-trip preserves all fields.
func TestSave_and_Load_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := &config.Config{
		RecentVaults:    []string{"/vault/a", "/vault/b"},
		LastOpenedVault: "/vault/a",
		WindowBounds: &config.WindowBounds{
			X: 10, Y: 20, Width: 1024, Height: 768,
		},
	}

	if err := config.Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.LastOpenedVault != original.LastOpenedVault {
		t.Errorf("LastOpenedVault = %q, want %q", loaded.LastOpenedVault, original.LastOpenedVault)
	}
	if len(loaded.RecentVaults) != len(original.RecentVaults) {
		t.Fatalf("RecentVaults len = %d, want %d", len(loaded.RecentVaults), len(original.RecentVaults))
	}
	for i, v := range original.RecentVaults {
		if loaded.RecentVaults[i] != v {
			t.Errorf("RecentVaults[%d] = %q, want %q", i, loaded.RecentVaults[i], v)
		}
	}
	if loaded.WindowBounds == nil {
		t.Fatal("WindowBounds should not be nil after round-trip")
	}
	if loaded.WindowBounds.Width != 1024 {
		t.Errorf("WindowBounds.Width = %d, want 1024", loaded.WindowBounds.Width)
	}
}

// Test: Save uses atomic write (file is valid JSON if read immediately after).
func TestSave_produces_valid_json(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		RecentVaults:    []string{"/vault/x"},
		LastOpenedVault: "/vault/x",
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Errorf("saved file is not valid JSON: %v — content: %s", err, data)
	}
}

// Test: AddRecentVault prepends and deduplicates.
func TestAddRecentVault_prepends_and_deduplicates(t *testing.T) {
	cfg := &config.Config{
		RecentVaults: []string{"/b", "/c"},
	}

	config.AddRecentVault(cfg, "/a")
	if cfg.RecentVaults[0] != "/a" {
		t.Errorf("first entry should be /a, got %q", cfg.RecentVaults[0])
	}
	if len(cfg.RecentVaults) != 3 {
		t.Errorf("len = %d, want 3", len(cfg.RecentVaults))
	}

	// Adding /b again should move it to the front, not duplicate it.
	config.AddRecentVault(cfg, "/b")
	if cfg.RecentVaults[0] != "/b" {
		t.Errorf("first entry after re-add should be /b, got %q", cfg.RecentVaults[0])
	}
	for _, v := range cfg.RecentVaults[1:] {
		if v == "/b" {
			t.Error("/b appears more than once in RecentVaults")
		}
	}
}

// Test: AddRecentVault trims list to at most MaxRecentVaults entries.
func TestAddRecentVault_trims_to_max(t *testing.T) {
	cfg := &config.Config{}
	for i := 0; i < config.MaxRecentVaults+5; i++ {
		config.AddRecentVault(cfg, filepath.Join("/vault", string(rune('a'+i))))
	}
	if len(cfg.RecentVaults) > config.MaxRecentVaults {
		t.Errorf("RecentVaults len = %d, should be <= %d", len(cfg.RecentVaults), config.MaxRecentVaults)
	}
}

// Test: Load treats corrupt JSON as empty config without error.
func TestLoad_treats_corrupt_json_as_empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json {{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load should not return error for corrupt JSON, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil for corrupt JSON")
	}
}

// Test: DefaultPath returns a non-empty path under the user's home directory.
func TestDefaultPath_returns_nonempty_path(t *testing.T) {
	path, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if path == "" {
		t.Error("DefaultPath returned empty string")
	}
	if filepath.Base(path) != "config.json" {
		t.Errorf("DefaultPath base = %q, want config.json", filepath.Base(path))
	}
}
