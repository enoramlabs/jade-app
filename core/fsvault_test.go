package core_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// --- Create ---

func TestFSVault_Create_creates_new_note(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	body := "# New Note\n\nHello world."
	note, err := v.Create(context.Background(), core.NoteMeta{ID: "new.md", Title: "new"}, body)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if note.ID != "new.md" {
		t.Errorf("ID = %q, want %q", note.ID, "new.md")
	}
	if note.Body != body {
		t.Errorf("Body = %q, want %q", note.Body, body)
	}
	data, err := os.ReadFile(filepath.Join(dir, "new.md"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != body {
		t.Errorf("file content = %q, want %q", string(data), body)
	}
}

func TestFSVault_Create_rejects_path_traversal(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Create(context.Background(), core.NoteMeta{ID: "../escape.md"}, "body")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

func TestFSVault_Create_returns_conflict_for_existing_note(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "existing.md", "# Existing")
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Create(context.Background(), core.NoteMeta{ID: "existing.md"}, "new body")
	if err == nil {
		t.Fatal("expected ConflictError creating existing note, got nil")
	}
	var ce *core.ConflictError
	if !errors.As(err, &ce) {
		t.Errorf("expected *core.ConflictError, got %T: %v", err, err)
	}
}

// --- Update ---

func TestFSVault_Update_updates_existing_note(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", "# Old body")
	v := openVault(t, dir)
	defer v.Close()

	newBody := "# Updated body"
	note, err := v.Update(context.Background(), "note.md", newBody, nil, "")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if note.Body != newBody {
		t.Errorf("Body = %q, want %q", note.Body, newBody)
	}
	// verify on disk
	data, _ := os.ReadFile(filepath.Join(dir, "note.md"))
	if string(data) != newBody {
		t.Errorf("file content = %q, want %q", string(data), newBody)
	}
}

func TestFSVault_Update_returns_not_found_for_missing_note(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Update(context.Background(), "missing.md", "body", nil, "")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfe *core.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *core.NotFoundError, got %T: %v", err, err)
	}
}

func TestFSVault_Update_returns_conflict_on_etag_mismatch(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", "# Body")
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Update(context.Background(), "note.md", "new body", nil, "wrong-etag")
	if err == nil {
		t.Fatal("expected ConflictError on ETag mismatch, got nil")
	}
	var ce *core.ConflictError
	if !errors.As(err, &ce) {
		t.Errorf("expected *core.ConflictError, got %T: %v", err, err)
	}
}

func TestFSVault_Update_rejects_path_traversal(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	_, err := v.Update(context.Background(), "../escape.md", "body", nil, "")
	if err == nil {
		t.Fatal("expected PathTraversalError, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

// --- Delete ---

func TestFSVault_Delete_removes_note(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", "# Note")
	v := openVault(t, dir)
	defer v.Close()

	if err := v.Delete(context.Background(), "note.md"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "note.md")); !os.IsNotExist(err) {
		t.Error("expected file to be deleted, but it still exists")
	}
}

func TestFSVault_Delete_returns_not_found_for_missing_note(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	err := v.Delete(context.Background(), "missing.md")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfe *core.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *core.NotFoundError, got %T: %v", err, err)
	}
}

func TestFSVault_Delete_rejects_path_traversal(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	err := v.Delete(context.Background(), "../escape.md")
	if err == nil {
		t.Fatal("expected PathTraversalError, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

// --- Move ---

func TestFSVault_Move_renames_note(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "old.md", "# Old")
	v := openVault(t, dir)
	defer v.Close()

	if err := v.Move(context.Background(), "old.md", "new.md"); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.md")); !os.IsNotExist(err) {
		t.Error("source file should not exist after move")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.md")); err != nil {
		t.Errorf("destination file should exist: %v", err)
	}
}

func TestFSVault_Move_returns_not_found_for_missing_source(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	err := v.Move(context.Background(), "missing.md", "dest.md")
	if err == nil {
		t.Fatal("expected NotFoundError, got nil")
	}
	var nfe *core.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected *core.NotFoundError, got %T: %v", err, err)
	}
}

func TestFSVault_Move_returns_conflict_on_target_exists(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "src.md", "# Src")
	writeNote(t, dir, "dst.md", "# Dst")
	v := openVault(t, dir)
	defer v.Close()

	err := v.Move(context.Background(), "src.md", "dst.md")
	if err == nil {
		t.Fatal("expected ConflictError when destination exists, got nil")
	}
	var ce *core.ConflictError
	if !errors.As(err, &ce) {
		t.Errorf("expected *core.ConflictError, got %T: %v", err, err)
	}
}

func TestFSVault_Move_rejects_path_traversal_in_source(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	err := v.Move(context.Background(), "../escape.md", "dest.md")
	if err == nil {
		t.Fatal("expected PathTraversalError, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

func TestFSVault_Move_rejects_path_traversal_in_dest(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", "# Note")
	v := openVault(t, dir)
	defer v.Close()

	err := v.Move(context.Background(), "note.md", "../escape.md")
	if err == nil {
		t.Fatal("expected PathTraversalError for dest, got nil")
	}
	var pte *core.PathTraversalError
	if !errors.As(err, &pte) {
		t.Errorf("expected *core.PathTraversalError, got %T: %v", err, err)
	}
}

// --- Frontmatter + ETag ---

func TestFSVault_Read_populates_etag(t *testing.T) {
	dir := t.TempDir()
	body := "# Note\n\nContent."
	writeNote(t, dir, "note.md", body)
	v := openVault(t, dir)
	defer v.Close()

	note, err := v.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if note.ETag == "" {
		t.Error("ETag should be non-empty after Read")
	}
	// Reading twice should produce the same ETag.
	note2, _ := v.Read(context.Background(), "note.md")
	if note.ETag != note2.ETag {
		t.Errorf("ETag not stable: %q vs %q", note.ETag, note2.ETag)
	}
}

func TestFSVault_Read_populates_frontmatter(t *testing.T) {
	dir := t.TempDir()
	body := "---\ntitle: My Note\ntags:\n  - go\n  - tdd\n---\n# Body"
	writeNote(t, dir, "note.md", body)
	v := openVault(t, dir)
	defer v.Close()

	note, err := v.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if note.Frontmatter == nil {
		t.Fatal("Frontmatter should be non-nil for note with YAML block")
	}
	if note.Frontmatter["title"] != "My Note" {
		t.Errorf("title = %v, want %q", note.Frontmatter["title"], "My Note")
	}
	if len(note.Tags) != 2 {
		t.Errorf("Tags = %v, want [go tdd]", note.Tags)
	}
}

func TestParseFrontmatter_roundtrip(t *testing.T) {
	body := "---\ntitle: Test\nstatus: done\n---\n# Hello\n\nBody text."
	fm, clean, err := core.ParseFrontmatter(body)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm["title"] != "Test" {
		t.Errorf("title = %v, want %q", fm["title"], "Test")
	}
	if fm["status"] != "done" {
		t.Errorf("status = %v, want %q", fm["status"], "done")
	}
	if clean != "# Hello\n\nBody text." {
		t.Errorf("cleanBody = %q, want %q", clean, "# Hello\n\nBody text.")
	}
}

func TestParseFrontmatter_no_frontmatter(t *testing.T) {
	body := "# Just a note\n\nNo frontmatter here."
	fm, clean, err := core.ParseFrontmatter(body)
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil fm for note without frontmatter, got %v", fm)
	}
	if clean != body {
		t.Errorf("cleanBody should equal body when no frontmatter")
	}
}

func TestSerializeFrontmatter_prepends_yaml_block(t *testing.T) {
	fm := map[string]any{"title": "Test", "status": "done"}
	body := "# Content"
	out, err := core.SerializeFrontmatter(fm, body)
	if err != nil {
		t.Fatalf("SerializeFrontmatter: %v", err)
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("output should start with ---\\n, got: %q", out[:min(20, len(out))])
	}
	// Roundtrip: parse it back.
	fm2, clean, err := core.ParseFrontmatter(out)
	if err != nil {
		t.Fatalf("parse after serialize: %v", err)
	}
	if clean != body {
		t.Errorf("clean body after roundtrip = %q, want %q", clean, body)
	}
	_ = fm2
}

func TestFSVault_concurrent_writes_to_same_note_serialize(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", "initial")
	v := openVault(t, dir)
	defer v.Close()

	const goroutines = 20
	done := make(chan struct{}, goroutines)
	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			_, _ = v.Update(context.Background(), "note.md", fmt.Sprintf("writer %d", n), nil, "")
		}(i)
	}
	for range goroutines {
		<-done
	}
	// After all writes, the file must still be valid (non-empty, readable).
	note, err := v.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatalf("Read after concurrent writes: %v", err)
	}
	if note.Body == "" {
		t.Error("body should not be empty after concurrent writes")
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
