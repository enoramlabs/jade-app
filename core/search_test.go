package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/enoramlabs/jade-app/core"
)

// TestFSVault_Search_returns_empty_on_empty_vault verifies that searching an empty vault
// returns an empty slice without error.
func TestFSVault_Search_returns_empty_on_empty_vault(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "anything"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestFSVault_Search_finds_note_by_body_text verifies free-text search across note body.
func TestFSVault_Search_finds_note_by_body_text(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "alpha.md", "# Alpha\nThis note talks about golang programming.")
	writeNote(t, dir, "beta.md", "# Beta\nThis note is about Python and data science.")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "golang"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}
	if results[0].ID != "alpha.md" {
		t.Errorf("expected alpha.md, got %s", results[0].ID)
	}
}

// TestFSVault_Search_tag_filter finds notes with a specific tag.
func TestFSVault_Search_tag_filter(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "tagged.md", "---\ntags:\n  - go\n  - tdd\n---\n# Tagged note")
	writeNote(t, dir, "untagged.md", "# No tags here")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "tag:go"})
	if err != nil {
		t.Fatalf("Search tag:go: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for tag:go, got %d: %v", len(results), results)
	}
	if results[0].ID != "tagged.md" {
		t.Errorf("expected tagged.md, got %s", results[0].ID)
	}
}

// TestFSVault_Search_frontmatter_filter finds notes by frontmatter key:value.
func TestFSVault_Search_frontmatter_filter(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "done.md", "---\nstatus: done\n---\n# Finished task")
	writeNote(t, dir, "wip.md", "---\nstatus: wip\n---\n# Work in progress")
	writeNote(t, dir, "bare.md", "# No frontmatter")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "status:done"})
	if err != nil {
		t.Fatalf("Search status:done: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for status:done, got %d: %v", len(results), results)
	}
	if results[0].ID != "done.md" {
		t.Errorf("expected done.md, got %s", results[0].ID)
	}
}

// TestFSVault_Search_AND_expression finds notes matching both terms.
func TestFSVault_Search_AND_expression(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "both.md", "---\ntags:\n  - go\nstatus: done\n---\n# Both tags and status")
	writeNote(t, dir, "tagonly.md", "---\ntags:\n  - go\n---\n# Only go tag")
	writeNote(t, dir, "statusonly.md", "---\nstatus: done\n---\n# Only done status")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "tag:go AND status:done"})
	if err != nil {
		t.Fatalf("Search AND: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for AND query, got %d: %v", len(results), results)
	}
	if results[0].ID != "both.md" {
		t.Errorf("expected both.md, got %s", results[0].ID)
	}
}

// TestFSVault_Search_OR_expression finds notes matching either term.
func TestFSVault_Search_OR_expression(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "go.md", "---\ntags:\n  - go\n---\n# Go note")
	writeNote(t, dir, "rust.md", "---\ntags:\n  - rust\n---\n# Rust note")
	writeNote(t, dir, "python.md", "---\ntags:\n  - python\n---\n# Python note")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "tag:go OR tag:rust"})
	if err != nil {
		t.Fatalf("Search OR: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for OR query, got %d: %v", len(results), results)
	}
}

// TestFSVault_Search_incremental_create verifies a newly created note is immediately searchable.
func TestFSVault_Search_incremental_create(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	// Nothing found before creation.
	results, err := v.Search(context.Background(), core.SearchQuery{Text: "elephant"})
	if err != nil {
		t.Fatalf("Search before create: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results before create, got %d", len(results))
	}

	// Create a note containing "elephant".
	_, err = v.Create(context.Background(), core.NoteMeta{ID: "new.md"}, "# Elephant\nI love elephants.")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Now it should be found.
	results, err = v.Search(context.Background(), core.SearchQuery{Text: "elephant"})
	if err != nil {
		t.Fatalf("Search after create: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after create, got %d", len(results))
	}
	if results[0].ID != "new.md" {
		t.Errorf("expected new.md, got %s", results[0].ID)
	}
}

// TestFSVault_Search_incremental_update verifies that the index reflects updated content.
func TestFSVault_Search_incremental_update(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "note.md", "# Note about cats")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	// Original content has "cats".
	results, err := v.Search(context.Background(), core.SearchQuery{Text: "cats"})
	if err != nil {
		t.Fatalf("Search cats: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for cats, got %d", len(results))
	}

	// Update the note to talk about dogs instead.
	_, err = v.Update(context.Background(), "note.md", "# Note about dogs", nil, "")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// "cats" should no longer be found.
	results, err = v.Search(context.Background(), core.SearchQuery{Text: "cats"})
	if err != nil {
		t.Fatalf("Search cats after update: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for cats after update, got %d", len(results))
	}

	// "dogs" should now be found.
	results, err = v.Search(context.Background(), core.SearchQuery{Text: "dogs"})
	if err != nil {
		t.Fatalf("Search dogs after update: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for dogs, got %d", len(results))
	}
}

// TestFSVault_Search_incremental_delete verifies that deleted notes are removed from search.
func TestFSVault_Search_incremental_delete(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "note.md", "# Note about unicorns")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	// Found before deletion.
	results, err := v.Search(context.Background(), core.SearchQuery{Text: "unicorns"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result before delete, got %d", len(results))
	}

	// Delete the note.
	if err := v.Delete(context.Background(), "note.md"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// No longer found.
	results, err = v.Search(context.Background(), core.SearchQuery{Text: "unicorns"})
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

// TestFSVault_Search_incremental_move verifies note is found under new ID after move.
func TestFSVault_Search_incremental_move(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()
	writeNote(t, dir, "old.md", "# Note about dragons")

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	if err := v.Move(context.Background(), "old.md", "new.md"); err != nil {
		t.Fatalf("Move: %v", err)
	}

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "dragons"})
	if err != nil {
		t.Fatalf("Search after move: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after move, got %d", len(results))
	}
	if results[0].ID != "new.md" {
		t.Errorf("expected new.md after move, got %s", results[0].ID)
	}
}

// TestFSVault_Search_seeded_vault verifies ranked results across a populated vault.
func TestFSVault_Search_seeded_vault(t *testing.T) {
	dir := t.TempDir()
	idxDir := t.TempDir()

	// Seed 20 notes; some mention "telescope", others don't.
	for i := 0; i < 10; i++ {
		name := filepath.Join(dir, "telescope-"+string(rune('a'+i))+".md")
		content := "# Telescope note\nThis note discusses telescope observations."
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatalf("writeNote: %v", err)
		}
	}
	for i := 0; i < 10; i++ {
		name := filepath.Join(dir, "other-"+string(rune('a'+i))+".md")
		content := "# Other note\nThis note is about cooking."
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatalf("writeNote: %v", err)
		}
	}

	v := core.NewFSVault(dir, core.WithIndexDir(idxDir))
	if err := v.Open(context.Background()); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer v.Close()

	results, err := v.Search(context.Background(), core.SearchQuery{Text: "telescope"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 telescope results, got %d", len(results))
	}
	// All results should have IDs starting with "telescope-"
	for _, r := range results {
		if len(r.ID) < 10 || r.ID[:10] != "telescope-" {
			t.Errorf("unexpected result ID: %s", r.ID)
		}
	}
}
