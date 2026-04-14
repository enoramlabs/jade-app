package core_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/enoramlabs/jade-app/core"
)

// --- Open ---

func TestFSVault_Open_succeeds_on_existing_directory(t *testing.T) {
	dir := t.TempDir()
	v := core.NewFSVault(dir)
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("expected Open to succeed, got: %v", err)
	}
	_ = v.Close()
}

func TestFSVault_Open_errors_on_missing_directory(t *testing.T) {
	v := core.NewFSVault("/nonexistent/path/that/does/not/exist")
	err := v.Open(context.Background())
	if err == nil {
		t.Fatal("expected error opening non-existent directory, got nil")
	}
}

// --- List ---

func TestFSVault_List_returns_md_files_in_vault_root(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "alpha.md", "# Alpha\nHello.")
	writeNote(t, dir, "beta.md", "# Beta\nWorld.")
	writeNote(t, dir, "not-a-note.txt", "ignore me")

	v := openVault(t, dir)
	defer v.Close()

	notes, err := v.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := len(notes), 2; got != want {
		t.Fatalf("List returned %d notes, want %d; notes: %v", got, want, notes)
	}
}

func TestFSVault_List_returns_md_files_in_subdirectory(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	writeNote(t, dir, "root.md", "# Root")
	writeNote(t, filepath.Join(dir, "sub"), "child.md", "# Child")

	v := openVault(t, dir)
	defer v.Close()

	notes, err := v.List(context.Background(), "sub")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := len(notes), 1; got != want {
		t.Fatalf("List('sub') returned %d notes, want %d", got, want)
	}
	if notes[0].ID != "sub/child.md" {
		t.Errorf("ID = %q, want %q", notes[0].ID, "sub/child.md")
	}
}

// --- Read ---

func TestFSVault_Read_returns_note_body(t *testing.T) {
	dir := t.TempDir()
	body := "# My Note\n\nSome content here."
	writeNote(t, dir, "note.md", body)

	v := openVault(t, dir)
	defer v.Close()

	note, err := v.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if note.Body != body {
		t.Errorf("Body = %q, want %q", note.Body, body)
	}
	if note.ID != "note.md" {
		t.Errorf("ID = %q, want %q", note.ID, "note.md")
	}
}

func TestFSVault_Read_returns_not_found_for_missing_note(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Read(context.Background(), "missing.md")
	if err == nil {
		t.Fatal("expected error reading missing note, got nil")
	}
	var nfe *core.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *core.NotFoundError, got %T: %v", err, err)
	}
}

// --- Path traversal ---

func TestFSVault_Read_rejects_path_traversal(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Read(context.Background(), "../secret.md")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

func TestFSVault_List_rejects_path_traversal(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.List(context.Background(), "../other")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

// --- helpers ---

func writeNote(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeNote: %v", err)
	}
}

func openVault(t *testing.T, dir string) *core.FSVault {
	t.Helper()
	v := core.NewFSVault(dir)
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return v
}
