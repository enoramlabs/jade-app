package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
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
	root     string
	indexDir string     // if empty, defaults to ~/.jade/indexes/<hash>/
	mu       sync.Map   // per-path *sync.Mutex for concurrent writes
	ignored  *ignoreSet // paths recently written by FSVault itself

	// Backlink index — protected by blMu.
	// blOutgoing: sourceID → set of resolved target IDs
	// blIncoming: targetID → set of source IDs that link to it
	// blAllIDs:   current flat list of all note IDs (for resolver)
	blMu       sync.RWMutex
	blOutgoing map[string]map[string]struct{}
	blIncoming map[string]map[string]struct{}
	blAllIDs   []string

	// Full-text search index.
	search *searchIndex
}

// FSVaultOption is a functional option for NewFSVault.
type FSVaultOption func(*FSVault)

// WithIndexDir overrides the directory used for the Bleve search index.
// Useful in tests to avoid polluting ~/.jade/indexes/.
func WithIndexDir(dir string) FSVaultOption {
	return func(v *FSVault) { v.indexDir = dir }
}

// NewFSVault creates an FSVault rooted at the given directory path.
func NewFSVault(root string, opts ...FSVaultOption) *FSVault {
	v := &FSVault{
		root:       filepath.Clean(root),
		ignored:    newIgnoreSet(),
		blOutgoing: make(map[string]map[string]struct{}),
		blIncoming: make(map[string]map[string]struct{}),
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// Open verifies the vault root exists, builds the backlink index, and opens the
// full-text search index.
func (v *FSVault) Open(ctx context.Context) error {
	info, err := os.Stat(v.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &NotFoundError{ID: v.root}
	}

	// Open the Bleve search index.
	si, err := openSearchIndex(v.root, v.indexDir)
	if err != nil {
		return fmt.Errorf("opening search index: %w", err)
	}
	v.search = si

	if err := v.rebuildBacklinkIndex(ctx); err != nil {
		return err
	}

	// Seed the search index with all existing notes.
	return v.rebuildSearchIndex(ctx)
}

// rebuildSearchIndex indexes every note currently in the vault.
func (v *FSVault) rebuildSearchIndex(ctx context.Context) error {
	if v.search == nil {
		return nil
	}
	var ids []string
	err := filepath.WalkDir(v.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip well-known noise directories (node_modules, .git, build
			// output, venvs, etc.). Never skip the vault root itself.
			if path != v.root && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(v.root, path)
		if relErr != nil {
			return nil
		}
		ids = append(ids, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		note, readErr := v.Read(ctx, id)
		if readErr != nil {
			continue // skip unreadable notes
		}
		_ = v.search.indexNote(note) // best-effort; don't abort on index failure
	}
	return nil
}

// rebuildBacklinkIndex scans the entire vault and reconstructs the backlink index.
func (v *FSVault) rebuildBacklinkIndex(_ context.Context) error {
	var ids []string
	err := filepath.WalkDir(v.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip well-known noise directories (node_modules, .git, build
			// output, venvs, etc.). Never skip the vault root itself.
			if path != v.root && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(v.root, path)
		if relErr != nil {
			return nil
		}
		ids = append(ids, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}

	v.blMu.Lock()
	defer v.blMu.Unlock()
	v.blAllIDs = ids
	v.blOutgoing = make(map[string]map[string]struct{})
	v.blIncoming = make(map[string]map[string]struct{})

	for _, id := range ids {
		absPath := filepath.Join(v.root, filepath.FromSlash(id))
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			continue
		}
		v.indexNoteLocked(id, string(data))
	}
	return nil
}

// indexNoteLocked adds a note's outgoing wikilinks to the index.
// Must be called with blMu write-locked.
func (v *FSVault) indexNoteLocked(sourceID, body string) {
	links := ExtractWikilinks(body)
	targets := make(map[string]struct{})
	for _, wl := range links {
		if wl.IsEmbed {
			continue
		}
		resolved := ResolveWikilink(wl.Target, v.blAllIDs)
		if resolved != "" {
			targets[resolved] = struct{}{}
		}
	}
	v.blOutgoing[sourceID] = targets
	for target := range targets {
		if v.blIncoming[target] == nil {
			v.blIncoming[target] = make(map[string]struct{})
		}
		v.blIncoming[target][sourceID] = struct{}{}
	}
}

// unindexNoteLocked removes all outgoing links from sourceID from the index.
// Must be called with blMu write-locked.
func (v *FSVault) unindexNoteLocked(sourceID string) {
	for target := range v.blOutgoing[sourceID] {
		delete(v.blIncoming[target], sourceID)
		if len(v.blIncoming[target]) == 0 {
			delete(v.blIncoming, target)
		}
	}
	delete(v.blOutgoing, sourceID)
}

// Close flushes the search index and releases resources.
func (v *FSVault) Close() error {
	if v.search != nil {
		return v.search.close()
	}
	return nil
}

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

	n, err := v.readNote(id, absPath)
	if err != nil {
		return nil, err
	}

	// Add to backlink index.
	v.blMu.Lock()
	v.blAllIDs = append(v.blAllIDs, id)
	v.indexNoteLocked(id, body)
	v.blMu.Unlock()

	// Add to search index.
	if v.search != nil {
		_ = v.search.indexNote(n)
	}

	return n, nil
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

	n, err := v.readNote(id, absPath)
	if err != nil {
		return nil, err
	}

	// Update backlink index: remove old links, add new ones.
	v.blMu.Lock()
	v.unindexNoteLocked(id)
	v.indexNoteLocked(id, body)
	v.blMu.Unlock()

	// Update search index.
	if v.search != nil {
		_ = v.search.indexNote(n)
	}

	return n, nil
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
	if err := os.Remove(absPath); err != nil {
		return err
	}

	// Remove from backlink index.
	v.blMu.Lock()
	v.unindexNoteLocked(id)
	// Remove id from allIDs.
	newIDs := v.blAllIDs[:0]
	for _, existing := range v.blAllIDs {
		if existing != id {
			newIDs = append(newIDs, existing)
		}
	}
	v.blAllIDs = newIDs
	v.blMu.Unlock()

	// Remove from search index.
	if v.search != nil {
		_ = v.search.removeNote(id)
	}

	return nil
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
	if err := os.Rename(srcAbs, dstAbs); err != nil {
		return err
	}

	// Update backlink index: re-index moved note under new ID.
	data, _ := os.ReadFile(dstAbs)
	v.blMu.Lock()
	v.unindexNoteLocked(fromID)
	// Replace fromID with toID in allIDs.
	found := false
	for i, id := range v.blAllIDs {
		if id == fromID {
			v.blAllIDs[i] = toID
			found = true
			break
		}
	}
	if !found {
		v.blAllIDs = append(v.blAllIDs, toID)
	}
	if data != nil {
		v.indexNoteLocked(toID, string(data))
	}
	v.blMu.Unlock()

	// Update search index: delete old entry, index under new ID.
	if v.search != nil {
		_ = v.search.removeNote(fromID)
		if data != nil {
			movedNote, readErr := v.readNote(toID, dstAbs)
			if readErr == nil {
				_ = v.search.indexNote(movedNote)
			}
		}
	}

	return nil
}

// Search queries the Bleve full-text index and returns matching notes.
// The SearchQuery.Text field is parsed as a query expression (see search.go).
func (v *FSVault) Search(ctx context.Context, q SearchQuery) ([]NoteMeta, error) {
	if v.search == nil {
		return nil, fmt.Errorf("search index not available; call Open first")
	}

	hits, err := v.search.search(q)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	results := make([]NoteMeta, 0, len(hits))
	for _, hit := range hits {
		note, readErr := v.Read(ctx, hit.ID)
		if readErr != nil {
			continue // note may have been deleted since last index update
		}
		meta := note.NoteMeta
		meta.Snippet = hit.Snippet
		results = append(results, meta)
	}
	return results, nil
}

// Backlinks returns all notes that contain a wikilink resolving to id.
func (v *FSVault) Backlinks(ctx context.Context, id string) ([]NoteMeta, error) {
	if err := v.validatePath(id); err != nil {
		return nil, err
	}

	v.blMu.RLock()
	sources := v.blIncoming[id]
	ids := make([]string, 0, len(sources))
	for src := range sources {
		ids = append(ids, src)
	}
	v.blMu.RUnlock()

	result := make([]NoteMeta, 0, len(ids))
	for _, src := range ids {
		note, err := v.Read(ctx, src)
		if err != nil {
			continue // note may have been deleted since indexing
		}
		result = append(result, note.NoteMeta)
	}
	return result, nil
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

// skipDirNames is the set of non-hidden directory names that the vault
// walker will never descend into. Hidden directories (names starting with
// ".") are skipped unconditionally — this covers .git, .svn, .hg, .next,
// .svelte-kit, .turbo, .venv, .gradle, .idea, .vscode, .cache, etc.
//
// The explicit list covers well-known noise directories that don't start
// with a dot: package managers, build output, language toolchains.
// Without this filter, opening a repository-as-vault would attempt to
// index every README.md inside node_modules — often thousands of files.
var skipDirNames = map[string]struct{}{
	"node_modules": {},
	"dist":         {},
	"build":        {},
	"out":          {},
	"target":       {},
	"vendor":       {},
	"venv":         {},
	"env":          {},
	"__pycache__":  {},
}

// shouldSkipDir reports whether a directory with the given basename should
// be skipped during a vault walk. Called with the directory's base name
// only (not a full path).
func shouldSkipDir(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	_, ok := skipDirNames[name]
	return ok
}
