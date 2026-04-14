package core

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	// selfWriteTTL is how long a path is marked as ignored after FSVault writes it.
	selfWriteTTL = 500 * time.Millisecond
	// debounceDuration coalesces rapid fsnotify bursts into a single event.
	debounceDuration = 100 * time.Millisecond
)

// ignoreSet tracks absolute paths that should be suppressed from Watch events
// because FSVault itself caused the change.
type ignoreSet struct {
	mu      sync.Mutex
	entries map[string]time.Time // absPath → expiry
}

func newIgnoreSet() *ignoreSet {
	return &ignoreSet{entries: make(map[string]time.Time)}
}

// add marks absPath as ignored until TTL expires.
func (s *ignoreSet) add(absPath string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[absPath] = time.Now().Add(ttl)
}

// contains reports whether absPath is currently in the ignore set.
// Expired entries are removed lazily.
func (s *ignoreSet) contains(absPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.entries[absPath]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.entries, absPath)
		return false
	}
	return true
}

// pendingEntry tracks a debounce timer for a path.
type pendingEntry struct {
	evType VaultEventType
	timer  *time.Timer
}

// Watch starts an fsnotify watcher on the vault root and returns a channel of
// VaultEvents. Only .md file changes are forwarded. Events caused by FSVault's
// own writes are suppressed. Rapid bursts are coalesced by a 100ms debounce.
// The returned channel is closed when ctx is cancelled.
func (v *FSVault) Watch(ctx context.Context) (<-chan VaultEvent, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(v.root); err != nil {
		watcher.Close()
		return nil, err
	}

	out := make(chan VaultEvent, 16)

	go func() {
		defer close(out)
		defer watcher.Close()

		var debMu sync.Mutex
		pending := make(map[string]*pendingEntry)

		// emit sends a VaultEvent for the given vault-relative id.
		emit := func(id string, evType VaultEventType) {
			select {
			case out <- VaultEvent{Type: evType, ID: id}:
			case <-ctx.Done():
			}
		}

		// schedule debounces an event for absPath.
		schedule := func(absPath string, evType VaultEventType) {
			// Filter: only .md files matter.
			rel, err := filepath.Rel(v.root, absPath)
			if err != nil || strings.HasPrefix(rel, "..") {
				return
			}
			id := filepath.ToSlash(rel)
			if !strings.HasSuffix(id, ".md") {
				return
			}

			debMu.Lock()
			defer debMu.Unlock()

			if e, ok := pending[absPath]; ok {
				// Update the event type and reset the timer.
				e.evType = evType
				e.timer.Reset(debounceDuration)
				return
			}

			e := &pendingEntry{evType: evType}
			e.timer = time.AfterFunc(debounceDuration, func() {
				debMu.Lock()
				cur, ok := pending[absPath]
				if ok {
					delete(pending, absPath)
				}
				debMu.Unlock()
				if ok && !v.ignored.contains(absPath) {
					emit(id, cur.evType)
				}
			})
			pending[absPath] = e
		}

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				absPath := filepath.Clean(event.Name)
				switch {
				case event.Has(fsnotify.Create):
					schedule(absPath, EventCreate)
				case event.Has(fsnotify.Write):
					schedule(absPath, EventUpdate)
				case event.Has(fsnotify.Remove):
					schedule(absPath, EventDelete)
				case event.Has(fsnotify.Rename):
					schedule(absPath, EventRename)
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
				// Non-fatal: log would go here; watcher continues.
			}
		}
	}()

	return out, nil
}
