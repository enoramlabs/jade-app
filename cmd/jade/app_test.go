//go:build unit

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// App binding tests use the -tags unit build tag so they can run
// without the CGO/Wails runtime (no webview needed).

func TestApp_OpenVault_succeeds_on_existing_directory(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t)

	info, err := app.OpenVault(dir)
	if err != nil {
		t.Fatalf("OpenVault: %v", err)
	}
	if info.Path != dir {
		t.Errorf("VaultInfo.Path = %q, want %q", info.Path, dir)
	}
}

func TestApp_OpenVault_returns_error_for_missing_directory(t *testing.T) {
	app := newTestApp(t)
	_, err := app.OpenVault("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing vault, got nil")
	}
}

func TestApp_ListNotes_returns_notes_in_vault(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "alpha.md", "# Alpha")
	writeTestNote(t, dir, "beta.md", "# Beta")

	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	notes, err := app.ListNotes("")
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if got, want := len(notes), 2; got != want {
		t.Fatalf("ListNotes returned %d notes, want %d", got, want)
	}
}

func TestApp_ReadNote_returns_note_body(t *testing.T) {
	dir := t.TempDir()
	body := "# Hello\n\nContent."
	writeTestNote(t, dir, "hello.md", body)

	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	note, err := app.ReadNote("hello.md")
	if err != nil {
		t.Fatalf("ReadNote: %v", err)
	}
	if note.Body != body {
		t.Errorf("Body = %q, want %q", note.Body, body)
	}
}

func TestApp_ReadNote_requires_open_vault(t *testing.T) {
	app := newTestApp(t)
	_, err := app.ReadNote("any.md")
	if err == nil {
		t.Fatal("expected error when no vault is open, got nil")
	}
}

func TestApp_CreateNote_creates_note(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	note, err := app.CreateNote("new.md", "# Hello", nil)
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if note.ID != "new.md" {
		t.Errorf("ID = %q, want %q", note.ID, "new.md")
	}
}

func TestApp_UpdateNote_updates_note(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "note.md", "# Old")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	note, err := app.UpdateNote("note.md", "# New", nil, "")
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if note.Body != "# New" {
		t.Errorf("Body = %q, want %q", note.Body, "# New")
	}
}

func TestApp_DeleteNote_deletes_note(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "note.md", "# Note")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	if err := app.DeleteNote("note.md"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	notes, _ := app.ListNotes("")
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after delete, got %d", len(notes))
	}
}

func TestApp_MoveNote_moves_note(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "src.md", "# Src")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	if err := app.MoveNote("src.md", "dst.md"); err != nil {
		t.Fatalf("MoveNote: %v", err)
	}
	notes, _ := app.ListNotes("")
	if len(notes) != 1 || notes[0].ID != "dst.md" {
		t.Errorf("expected [dst.md], got %v", notes)
	}
}

func TestApp_ExportHTML_returns_rendered_html(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "note.md", "# Hello\n\nWorld.")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	html, err := app.ExportHTML("note.md")
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") {
		t.Errorf("ExportHTML: expected <h1>Hello</h1>, got: %s", html)
	}
}

func TestApp_RenderMarkdown_returns_rendered_html(t *testing.T) {
	app := newTestApp(t)
	html := app.RenderMarkdown("# Hi\n\n~~gone~~")
	if !strings.Contains(html, "<h1>Hi</h1>") {
		t.Errorf("RenderMarkdown: expected heading, got: %s", html)
	}
	if !strings.Contains(html, "<del>gone</del>") {
		t.Errorf("RenderMarkdown: expected strikethrough, got: %s", html)
	}
}

func TestApp_RenderMarkdown_requires_no_open_vault(t *testing.T) {
	// RenderMarkdown is stateless — it should work without an open vault.
	app := newTestApp(t)
	html := app.RenderMarkdown("hello")
	if html == "" {
		t.Error("RenderMarkdown should return non-empty HTML even without open vault")
	}
}

func TestApp_Backlinks_returns_linking_notes(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "A.md", "Links to [[B]].")
	writeTestNote(t, dir, "B.md", "# B")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	backlinks, err := app.Backlinks("B.md")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].ID != "A.md" {
		t.Errorf("Backlinks('B.md') = %v, want [{ID:A.md}]", backlinks)
	}
}

func TestApp_UpdateNote_returns_structured_conflict_error(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "note.md", "# Original")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	// Read to capture ETag.
	note, err := app.ReadNote("note.md")
	if err != nil {
		t.Fatalf("ReadNote: %v", err)
	}
	staleEtag := note.ETag

	// Simulate external edit.
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# External edit"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}

	// UpdateNote with stale ETag must return a structured CONFLICT error.
	_, updateErr := app.UpdateNote("note.md", "# My version", nil, staleEtag)
	if updateErr == nil {
		t.Fatal("expected error on conflict, got nil")
	}

	// The error message must be a JSON object with code == "CONFLICT".
	var appErr appError
	if err := json.Unmarshal([]byte(updateErr.Error()), &appErr); err != nil {
		t.Fatalf("error is not JSON: %v — raw: %q", err, updateErr.Error())
	}
	if appErr.Code != "CONFLICT" {
		t.Errorf("Code = %q, want %q", appErr.Code, "CONFLICT")
	}
	if appErr.CurrentContent == "" {
		t.Error("CurrentContent must be non-empty on CONFLICT")
	}
	if appErr.CurrentETag == "" {
		t.Error("CurrentETag must be non-empty on CONFLICT")
	}
}

func TestApp_ResolveWikilink_resolves_to_canonical_id(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "Alpha.md", "# Alpha")
	writeTestNote(t, dir, "beta.md", "# Beta")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	// Case-insensitive exact match: "alpha" should resolve to "Alpha.md".
	id, err := app.ResolveWikilink("alpha")
	if err != nil {
		t.Fatalf("ResolveWikilink: %v", err)
	}
	if id != "Alpha.md" {
		t.Errorf("ResolveWikilink('alpha') = %q, want %q", id, "Alpha.md")
	}
}

func TestApp_ResolveWikilink_returns_empty_for_unresolved(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "note.md", "# Note")
	app := newTestApp(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}

	id, err := app.ResolveWikilink("nonexistent")
	if err != nil {
		t.Fatalf("ResolveWikilink: %v", err)
	}
	if id != "" {
		t.Errorf("ResolveWikilink('nonexistent') = %q, want empty string", id)
	}
}

// ---- Sub-issue #9 tests: welcome screen, recent vaults, multi-window ----

func TestApp_GetStartupState_no_vault_returns_empty_path(t *testing.T) {
	app := newTestApp(t)
	state := app.GetStartupState()
	if state.VaultPath != "" {
		t.Errorf("VaultPath should be empty before any vault is opened, got %q", state.VaultPath)
	}
}

func TestApp_GetStartupState_after_open_vault_returns_path(t *testing.T) {
	dir := t.TempDir()
	app := newAppWithConfigDir(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}
	state := app.GetStartupState()
	if state.VaultPath != dir {
		t.Errorf("VaultPath = %q, want %q", state.VaultPath, dir)
	}
}

func TestApp_RecentVaults_returns_empty_before_any_vault_opened(t *testing.T) {
	app := newAppWithConfigDir(t)
	vaults, err := app.RecentVaults()
	if err != nil {
		t.Fatalf("RecentVaults: %v", err)
	}
	if len(vaults) != 0 {
		t.Errorf("RecentVaults should be empty initially, got %v", vaults)
	}
}

func TestApp_RecentVaults_records_opened_vault(t *testing.T) {
	dir := t.TempDir()
	app := newAppWithConfigDir(t)
	if _, err := app.OpenVault(dir); err != nil {
		t.Fatalf("OpenVault: %v", err)
	}
	vaults, err := app.RecentVaults()
	if err != nil {
		t.Fatalf("RecentVaults: %v", err)
	}
	if len(vaults) == 0 {
		t.Fatal("RecentVaults should contain the opened vault")
	}
	if vaults[0] != dir {
		t.Errorf("RecentVaults[0] = %q, want %q", vaults[0], dir)
	}
}

func TestApp_CreateVault_scaffolds_directory_and_welcome_note(t *testing.T) {
	parent := t.TempDir()
	vaultDir := filepath.Join(parent, "myvault")
	app := newAppWithConfigDir(t)

	info, err := app.CreateVault(vaultDir)
	if err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	if info.Path != vaultDir {
		t.Errorf("VaultInfo.Path = %q, want %q", info.Path, vaultDir)
	}

	// Directory must exist.
	if _, err := os.Stat(vaultDir); err != nil {
		t.Fatalf("vault directory not created: %v", err)
	}

	// welcome.md must exist.
	welcomePath := filepath.Join(vaultDir, "welcome.md")
	if _, err := os.Stat(welcomePath); err != nil {
		t.Fatalf("welcome.md not created: %v", err)
	}

	// Vault must be open (ListNotes should work).
	notes, err := app.ListNotes("")
	if err != nil {
		t.Fatalf("ListNotes after CreateVault: %v", err)
	}
	found := false
	for _, n := range notes {
		if n.ID == "welcome.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("welcome.md not found in notes list: %v", notes)
	}
}

func TestApp_CreateVault_returns_error_for_empty_path_without_runtime(t *testing.T) {
	app := newTestApp(t) // no ctx — simulates unit-test environment without Wails runtime
	_, err := app.CreateVault("")
	if err == nil {
		t.Fatal("expected error for empty path without runtime, got nil")
	}
}

func TestApp_InitFromConfig_opens_last_vault(t *testing.T) {
	vaultDir := t.TempDir()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")

	// Save a config pointing at vaultDir.
	cfg := &appConfig{
		LastOpenedVault: vaultDir,
	}
	if err := saveTestConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveTestConfig: %v", err)
	}

	app := newAppWithSpecificConfig(t, cfgPath)
	app.initFromConfig()

	state := app.GetStartupState()
	if state.VaultPath != vaultDir {
		t.Errorf("VaultPath = %q, want %q", state.VaultPath, vaultDir)
	}
	if state.VaultError != "" {
		t.Errorf("VaultError should be empty for reachable vault, got %q", state.VaultError)
	}
}

func TestApp_InitFromConfig_falls_back_to_welcome_when_vault_missing(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")

	cfg := &appConfig{
		LastOpenedVault: "/nonexistent/vault/that/does/not/exist",
	}
	if err := saveTestConfig(cfgPath, cfg); err != nil {
		t.Fatalf("saveTestConfig: %v", err)
	}

	app := newAppWithSpecificConfig(t, cfgPath)
	app.initFromConfig()

	state := app.GetStartupState()
	if state.VaultPath != "" {
		t.Errorf("VaultPath should be empty for missing vault, got %q", state.VaultPath)
	}
	if state.VaultError == "" {
		t.Error("VaultError should be non-empty for unreachable vault")
	}
}

func TestApp_TwoInstances_do_not_share_vault_state(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	writeTestNote(t, dir1, "note1.md", "# Note 1")
	writeTestNote(t, dir2, "note2.md", "# Note 2")

	app1 := newAppWithConfigDir(t)
	app2 := newAppWithConfigDir(t)

	if _, err := app1.OpenVault(dir1); err != nil {
		t.Fatalf("app1.OpenVault: %v", err)
	}
	if _, err := app2.OpenVault(dir2); err != nil {
		t.Fatalf("app2.OpenVault: %v", err)
	}

	notes1, err := app1.ListNotes("")
	if err != nil {
		t.Fatalf("app1.ListNotes: %v", err)
	}
	notes2, err := app2.ListNotes("")
	if err != nil {
		t.Fatalf("app2.ListNotes: %v", err)
	}

	// The two vaults must have different notes.
	if len(notes1) != 1 || notes1[0].ID != "note1.md" {
		t.Errorf("app1 notes = %v, want [note1.md]", notes1)
	}
	if len(notes2) != 1 || notes2[0].ID != "note2.md" {
		t.Errorf("app2 notes = %v, want [note2.md]", notes2)
	}
}

// ---- helpers ----

// appConfig mirrors the config.Config struct so tests can construct configs
// without importing the config package directly.
type appConfig struct {
	LastOpenedVault string `json:"lastOpenedVault"`
}

func saveTestConfig(path string, cfg *appConfig) error {
	data, _ := json.Marshal(cfg)
	return os.WriteFile(path, data, 0o644)
}

// newAppWithConfigDir creates an App that uses a temp dir for its config file.
func newAppWithConfigDir(t *testing.T) *App {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	return newAppWithSpecificConfig(t, cfgPath)
}

// newTestApp creates an App whose cfgPath is pinned to a per-test temp
// directory. This isolates every test run from the real user's
// ~/.jade/config.json — otherwise every test that calls OpenVault with a
// t.TempDir() path would persist that tempdir into recentVaults on disk,
// polluting the real config file.
func newTestApp(t *testing.T) *App {
	t.Helper()
	cfgDir := t.TempDir()
	app := NewApp()
	app.cfgPath = filepath.Join(cfgDir, "config.json")
	return app
}

// newAppWithSpecificConfig creates an App pinned to the given config file path.
// Used by tests that need to seed a specific config on disk and verify
// how the app reads it.
func newAppWithSpecificConfig(t *testing.T, cfgPath string) *App {
	t.Helper()
	app := newTestApp(t)
	app.cfgPath = cfgPath
	return app
}

func writeTestNote(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestNote: %v", err)
	}
}
