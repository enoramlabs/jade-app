package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/enoramlabs/jade-app/core"
)

// --- ExtractWikilinks ---

// --- RenderMarkdownWithWikilinks ---

func TestRenderMarkdownWithWikilinks_renders_wikilink_as_anchor(t *testing.T) {
	html := core.RenderMarkdownWithWikilinks("See [[notes/foo]].")
	if !strings.Contains(html, `data-wikilink="notes/foo"`) {
		t.Errorf("expected data-wikilink attribute, got: %s", html)
	}
}

func TestRenderMarkdownWithWikilinks_uses_display_text(t *testing.T) {
	html := core.RenderMarkdownWithWikilinks("[[target|My Display]]")
	if !strings.Contains(html, "My Display") {
		t.Errorf("expected display text 'My Display', got: %s", html)
	}
}

func TestRenderMarkdownWithWikilinks_renders_embed_as_div(t *testing.T) {
	html := core.RenderMarkdownWithWikilinks("![[image.png]]")
	if !strings.Contains(html, `data-embed="image.png"`) {
		t.Errorf("expected data-embed attribute, got: %s", html)
	}
}

// --- FSVault backlinks integration ---

func TestFSVault_Backlinks_returns_notes_linking_to_id(t *testing.T) {
	dir := t.TempDir()
	// A.md links to B; B.md has no links.
	writeNote(t, dir, "A.md", "See [[B]].")
	writeNote(t, dir, "B.md", "# B\n\nNo outgoing links.")
	v := openVault(t, dir)
	defer v.Close()

	backlinks, err := v.Backlinks(context.Background(), "B.md")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0].ID != "A.md" {
		t.Errorf("Backlinks('B.md') = %v, want [{ID:'A.md'}]", backlinks)
	}
}

func TestFSVault_Backlinks_updates_after_update_adds_link(t *testing.T) {
	dir := t.TempDir()
	// Initially A has no links.
	writeNote(t, dir, "A.md", "# A — no links yet.")
	writeNote(t, dir, "B.md", "# B")
	v := openVault(t, dir)
	defer v.Close()

	bl, _ := v.Backlinks(context.Background(), "B.md")
	if len(bl) != 0 {
		t.Fatalf("before update: expected 0 backlinks, got %d", len(bl))
	}

	// Update A to add a link to B.
	if _, err := v.Update(context.Background(), "A.md", "Now links [[B]].", nil, ""); err != nil {
		t.Fatalf("Update: %v", err)
	}

	bl, err := v.Backlinks(context.Background(), "B.md")
	if err != nil {
		t.Fatalf("Backlinks after update: %v", err)
	}
	if len(bl) != 1 || bl[0].ID != "A.md" {
		t.Errorf("after update: expected [{A.md}], got %v", bl)
	}
}

func TestFSVault_Backlinks_updates_after_delete(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "A.md", "See [[B]].")
	writeNote(t, dir, "B.md", "# B")
	v := openVault(t, dir)
	defer v.Close()

	// Confirm A appears in B's backlinks.
	bl, _ := v.Backlinks(context.Background(), "B.md")
	if len(bl) != 1 {
		t.Fatalf("before delete: expected 1 backlink, got %d", len(bl))
	}

	// Delete A.
	if err := v.Delete(context.Background(), "A.md"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// B should now have no backlinks.
	bl, err := v.Backlinks(context.Background(), "B.md")
	if err != nil {
		t.Fatalf("Backlinks after delete: %v", err)
	}
	if len(bl) != 0 {
		t.Errorf("after delete: expected 0 backlinks, got %d: %v", len(bl), bl)
	}
}

func TestFSVault_Backlinks_returns_empty_for_note_with_no_inbound_links(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "orphan.md", "# Orphan\n\nNo one links here.")
	v := openVault(t, dir)
	defer v.Close()

	backlinks, err := v.Backlinks(context.Background(), "orphan.md")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(backlinks) != 0 {
		t.Errorf("expected 0 backlinks, got %d: %v", len(backlinks), backlinks)
	}
}

// --- ResolveWikilink ---

func TestResolveWikilink_exact_case_insensitive(t *testing.T) {
	ids := []string{"Notes.md", "folder/bar.md"}
	got := core.ResolveWikilink("notes", ids)
	if got != "Notes.md" {
		t.Errorf("resolved to %q, want %q", got, "Notes.md")
	}
}

func TestResolveWikilink_filename_only_in_nested_path(t *testing.T) {
	ids := []string{"folder/deep/MyNote.md", "other.md"}
	got := core.ResolveWikilink("mynote", ids)
	if got != "folder/deep/MyNote.md" {
		t.Errorf("resolved to %q, want %q", got, "folder/deep/MyNote.md")
	}
}

func TestResolveWikilink_returns_empty_when_no_match(t *testing.T) {
	ids := []string{"alpha.md", "beta.md"}
	got := core.ResolveWikilink("gamma", ids)
	if got != "" {
		t.Errorf("expected empty string for unresolvable target, got %q", got)
	}
}

func TestExtractWikilinks_heading_anchor(t *testing.T) {
	links := core.ExtractWikilinks("[[notes/foo#section]]")
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Target != "notes/foo" {
		t.Errorf("Target = %q, want %q", links[0].Target, "notes/foo")
	}
	if links[0].Heading != "section" {
		t.Errorf("Heading = %q, want %q", links[0].Heading, "section")
	}
}

func TestExtractWikilinks_embed(t *testing.T) {
	links := core.ExtractWikilinks("![[image.png]]")
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Target != "image.png" {
		t.Errorf("Target = %q, want %q", links[0].Target, "image.png")
	}
	if !links[0].IsEmbed {
		t.Error("IsEmbed should be true for ![[...]]")
	}
}

func TestExtractWikilinks_pipe_alias(t *testing.T) {
	links := core.ExtractWikilinks("See [[notes/foo|My Note]].")
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Target != "notes/foo" {
		t.Errorf("Target = %q, want %q", links[0].Target, "notes/foo")
	}
	if links[0].Display != "My Note" {
		t.Errorf("Display = %q, want %q", links[0].Display, "My Note")
	}
}

func TestExtractWikilinks_simple_link(t *testing.T) {
	links := core.ExtractWikilinks("See [[notes/foo]] and more.")
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d: %v", len(links), links)
	}
	if links[0].Target != "notes/foo" {
		t.Errorf("Target = %q, want %q", links[0].Target, "notes/foo")
	}
	if links[0].Display != "" {
		t.Errorf("Display = %q, want empty", links[0].Display)
	}
	if links[0].IsEmbed {
		t.Error("IsEmbed should be false for [[...]]")
	}
}
