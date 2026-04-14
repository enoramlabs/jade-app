package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/enoramlabs/jade-app/core"
	"golang.org/x/text/unicode/norm"
)

// --- NFC normalization ---

func TestFSVault_Read_normalizes_body_to_NFC(t *testing.T) {
	dir := t.TempDir()
	// Write content that contains an NFD sequence: 'e' + combining acute accent
	// This is a valid UTF-8 NFD representation of 'é'.
	nfdCafe := "caf\u0065\u0301" // NFD: 'e' + U+0301 combining accent
	body := "# Note\n\n" + nfdCafe + " is great."
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	v := openVault(t, dir)
	defer v.Close()

	note, err := v.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !norm.NFC.IsNormalString(note.Body) {
		t.Error("Body should be NFC normalized after Read")
	}
}

// --- Watch ---

func TestFSVault_Watch_returns_non_nil_channel(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if ch == nil {
		t.Error("Watch should return a non-nil channel")
	}
}

func TestFSVault_Watch_external_create_emits_event(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	// Use a generous timeout so CI machines with slow fsnotify aren't flaky.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Give watcher time to initialise before writing.
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "external.md"), []byte("# External"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-ch:
		if evt.ID != "external.md" {
			t.Errorf("event ID = %q, want %q", evt.ID, "external.md")
		}
		if evt.Type != core.EventCreate && evt.Type != core.EventUpdate {
			t.Errorf("event type = %q, want Create or Update", evt.Type)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for external create event")
	}
}

func TestFSVault_Watch_external_delete_emits_event(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "todelete.md", "# Delete me")
	v := openVault(t, dir)
	defer v.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := os.Remove(filepath.Join(dir, "todelete.md")); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-ch:
		if evt.ID != "todelete.md" {
			t.Errorf("event ID = %q, want %q", evt.ID, "todelete.md")
		}
		if evt.Type != core.EventDelete && evt.Type != core.EventRename {
			t.Errorf("event type = %q, want Delete or Rename", evt.Type)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for external delete event")
	}
}

func TestFSVault_Watch_self_writes_are_suppressed(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// Write via FSVault itself — should NOT appear on the watch channel.
	if _, err := v.Create(context.Background(), core.NoteMeta{ID: "self.md"}, "# Self"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait long enough that any event would have arrived.
	select {
	case evt := <-ch:
		if evt.ID == "self.md" {
			t.Errorf("self-write event should be suppressed, got %+v", evt)
		}
		// An event for another path is unexpected but not a test failure here.
	case <-time.After(400 * time.Millisecond):
		// No event — correct.
	}
}

func TestFSVault_Watch_channel_closed_on_ctx_cancel(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// A stray event is acceptable; the channel should eventually close.
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Error("channel should be closed after ctx cancel")
				}
			case <-time.After(2 * time.Second):
				t.Error("channel not closed within 2s of ctx cancel")
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("channel not closed within 2s of ctx cancel")
	}
}

func TestFSVault_Watch_non_md_files_ignored(t *testing.T) {
	dir := t.TempDir()
	v := openVault(t, dir)
	defer v.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := v.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// Write a non-.md file — should not appear on channel.
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case evt := <-ch:
		t.Errorf("non-.md file change should be ignored, got %+v", evt)
	case <-time.After(500 * time.Millisecond):
		// Good — no event.
	}
}
