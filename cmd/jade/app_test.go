//go:build unit

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// App binding tests use the -tags unit build tag so they can run
// without the CGO/Wails runtime (no webview needed).

func TestApp_OpenVault_succeeds_on_existing_directory(t *testing.T) {
	dir := t.TempDir()
	app := NewApp()

	info, err := app.OpenVault(dir)
	if err != nil {
		t.Fatalf("OpenVault: %v", err)
	}
	if info.Path != dir {
		t.Errorf("VaultInfo.Path = %q, want %q", info.Path, dir)
	}
}

func TestApp_OpenVault_returns_error_for_missing_directory(t *testing.T) {
	app := NewApp()
	_, err := app.OpenVault("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for missing vault, got nil")
	}
}

func TestApp_ListNotes_returns_notes_in_vault(t *testing.T) {
	dir := t.TempDir()
	writeTestNote(t, dir, "alpha.md", "# Alpha")
	writeTestNote(t, dir, "beta.md", "# Beta")

	app := NewApp()
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

	app := NewApp()
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
	app := NewApp()
	_, err := app.ReadNote("any.md")
	if err == nil {
		t.Fatal("expected error when no vault is open, got nil")
	}
}

func TestApp_CreateNote_creates_note(t *testing.T) {
	dir := t.TempDir()
	app := NewApp()
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
	app := NewApp()
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
	app := NewApp()
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
	app := NewApp()
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

func writeTestNote(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestNote: %v", err)
	}
}
