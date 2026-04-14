package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// FSVault is a filesystem-backed Vault implementation.
// The vault root is a directory; each .md file is a note.
type FSVault struct {
	root    string
	mu      sync.Map   // per-path *sync.Mutex for concurrent writes
	ignored *ignoreSet // paths recently written by FSVault itself
}

// NewFSVault creates an FSVault rooted at the given directory path.
func NewFSVault(root string) *FSVault {
	return &FSVault{
		root:    filepath.Clean(root),
		ignored: newIgnoreSet(),
	}
}

// Open verifies the vault root exists.
func (v *FSVault) Open(_ context.Context) error {
	info, err := os.Stat(v.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &NotFoundError{ID: v.root}
	}
	return nil
}

// Close is a no-op for FSVault in this slice.
func (v *FSVault) Close() error { return nil }

// List returns NoteMeta for every .md file under the given sub-path.
// Pass "" or "." for the vault root. Non-recursive (top-level only).
func (v *FSVault) List(_ context.Context, path string) ([]NoteMeta, error) {
	if err := v.validatePath(path); err != nil {
		return nil, err
	}

	dir := v.root
	if path != "" && path != "." {
		dir = filepath.Join(v.root, filepath.FromSlash(path))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var notes []NoteMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		id := e.Name()
		if path != "" && path != "." {
			id = path + "/" + e.Name()
		}

		notes = append(notes, NoteMeta{
			ID:      id,
			Title:   strings.TrimSuffix(e.Name(), ".md"),
			ModTime: info.ModTime().UTC().Truncate(time.Second),
		})
	}
	return notes, nil
}

// Read returns the full Note for the given vault-relative ID.
func (v *FSVault) Read(_ context.Context, id string) (*Note, error) {
	if err := v.validatePath(id); err != nil {
		return nil, err
	}
	absPath := filepath.Join(v.root, filepath.FromSlash(id))
	return v.readNote(id, absPath)
}

// Create creates a new note at note.ID with the given body.
// Returns ConflictError if a file already exists at that path.
func (v *FSVault) Create(ctx context.Context, note NoteMeta, body string) (*Note, error) {
	id := note.ID
	if err := v.validatePath(id); err != nil {
		return nil, err
	}

	mu := v.lockPath(id)
	defer mu.Unlock()

	absPath := filepath.Join(v.root, filepath.FromSlash(id))

	// Fail if file already exists.
	if _, err := os.Stat(absPath); err == nil {
		existing, readErr := v.readNote(id, absPath)
		if readErr != nil {
			return nil, &ConflictError{ID: id}
		}
		return nil, &ConflictError{ID: id, Current: existing}
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("create parent dirs: %w", err)
	}

	v.ignored.add(absPath, selfWriteTTL)
	if err := atomicWriteFile(absPath, body); err != nil {
		return nil, err
	}

	return v.readNote(id, absPath)
}

// Update replaces the body of an existing note.
// If etag is non-empty and does not match the current on-disk content hash,
// a ConflictError is returned.
func (v *FSVault) Update(ctx context.Context, id string, body string, frontmatter map[string]any, etag string) (*Note, error) {
	if err := v.validatePath(id); err != nil {
		return nil, err
	}

	mu := v.lockPath(id)
	defer mu.Unlock()

	absPath := filepath.Join(v.root, filepath.FromSlash(id))

	// Load current content for ETag check.
	existing, err := v.readNote(id, absPath)
	if err != nil {
		return nil, err
	}

	// If the caller supplied an ETag, verify it matches.
	if etag != "" && etag != existing.ETag {
		return nil, &ConflictError{ID: id, Current: existing}
	}

	v.ignored.add(absPath, selfWriteTTL)
	if err := atomicWriteFile(absPath, body); err != nil {
		return nil, err
	}

	return v.readNote(id, absPath)
}

// Delete removes a note permanently.
func (v *FSVault) Delete(ctx context.Context, id string) error {
	if err := v.validatePath(id); err != nil {
		return err
	}

	mu := v.lockPath(id)
	defer mu.Unlock()

	absPath := filepath.Join(v.root, filepath.FromSlash(id))
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return &NotFoundError{ID: id}
	}
	v.ignored.add(absPath, selfWriteTTL)
	return os.Remove(absPath)
}

// Move renames/moves a note from fromID to toID.
// Returns ConflictError if toID already exists.
func (v *FSVault) Move(ctx context.Context, fromID, toID string) error {
	if err := v.validatePath(fromID); err != nil {
		return err
	}
	if err := v.validatePath(toID); err != nil {
		return err
	}

	// Lock both paths in a consistent order to avoid deadlocks.
	first, second := fromID, toID
	if first > second {
		first, second = second, first
	}
	m1 := v.lockPath(first)
	defer m1.Unlock()
	// Acquire second only if it's a different path.
	if fromID != toID {
		m2 := v.lockPath(second)
		defer m2.Unlock()
	}

	srcAbs := filepath.Join(v.root, filepath.FromSlash(fromID))
	dstAbs := filepath.Join(v.root, filepath.FromSlash(toID))

	if _, err := os.Stat(srcAbs); os.IsNotExist(err) {
		return &NotFoundError{ID: fromID}
	}
	if _, err := os.Stat(dstAbs); err == nil {
		return &ConflictError{ID: toID}
	}

	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	v.ignored.add(srcAbs, selfWriteTTL)
	v.ignored.add(dstAbs, selfWriteTTL)
	return os.Rename(srcAbs, dstAbs)
}

func (v *FSVault) Search(_ context.Context, _ SearchQuery) ([]NoteMeta, error) {
	return nil, ErrNotImplemented
}

func (v *FSVault) Backlinks(_ context.Context, _ string) ([]NoteMeta, error) {
	return nil, ErrNotImplemented
}

// lockPath acquires (and returns) the per-path mutex for id.
// Callers must call Unlock() on the returned mutex.
func (v *FSVault) lockPath(id string) *sync.Mutex {
	actual, _ := v.mu.LoadOrStore(id, &sync.Mutex{})
	m := actual.(*sync.Mutex)
	m.Lock()
	return m
}

// readNote loads a note from absPath, populating ETag and frontmatter.
func (v *FSVault) readNote(id, absPath string) (*Note, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{ID: id}
		}
		return nil, err
	}

	body := string(data)
	if !utf8.ValidString(body) {
		return nil, &EncodingError{Path: id}
	}
	// Normalize to NFC so NFD filenames/content from macOS don't cause drift.
	body = norm.NFC.String(body)

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	fm, _, _ := ParseFrontmatter(body)

	return &Note{
		NoteMeta: NoteMeta{
			ID:          id,
			Title:       strings.TrimSuffix(filepath.Base(id), ".md"),
			Tags:        tagsFromFrontmatter(fm),
			Frontmatter: fm,
			ModTime:     info.ModTime().UTC().Truncate(time.Second),
		},
		Body: body,
		ETag: computeETag(body),
	}, nil
}

// computeETag returns the SHA-256 hex digest of body.
func computeETag(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

// atomicWriteFile writes content to path via a temp file + rename.
func atomicWriteFile(path, content string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".jade-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if writeErr != nil {
		os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// validatePath checks that path does not escape the vault root.
// It accepts "" or "." for the root, and forward-slash paths.
func (v *FSVault) validatePath(path string) error {
	if path == "" || path == "." {
		return nil
	}
	// Clean using filesystem separator then check for ".."
	cleaned := filepath.Clean(filepath.FromSlash(path))
	// After cleaning, if the path starts with ".." it escapes the vault.
	if strings.HasPrefix(cleaned, "..") {
		return &PathTraversalError{ID: path}
	}
	// Additionally verify the resolved absolute path stays within root.
	abs := filepath.Join(v.root, cleaned)
	rootClean := filepath.Clean(v.root)
	if !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) && abs != rootClean {
		return &PathTraversalError{ID: path}
	}
	return nil
}
