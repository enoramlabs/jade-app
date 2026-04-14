package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FSVault is a filesystem-backed Vault implementation.
// The vault root is a directory; each .md file is a note.
type FSVault struct {
	root string
}

// NewFSVault creates an FSVault rooted at the given directory path.
func NewFSVault(root string) *FSVault {
	return &FSVault{root: filepath.Clean(root)}
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

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{ID: id}
		}
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	body := string(data)
	return &Note{
		NoteMeta: NoteMeta{
			ID:      id,
			Title:   strings.TrimSuffix(filepath.Base(id), ".md"),
			ModTime: info.ModTime().UTC().Truncate(time.Second),
		},
		Body: body,
	}, nil
}

// Create, Update, Delete, Move, Search, Backlinks, Watch are not implemented in this slice.

func (v *FSVault) Create(_ context.Context, _ NoteMeta, _ string) (*Note, error) {
	return nil, ErrNotImplemented
}

func (v *FSVault) Update(_ context.Context, _ string, _ string, _ map[string]any, _ string) (*Note, error) {
	return nil, ErrNotImplemented
}

func (v *FSVault) Delete(_ context.Context, _ string) error {
	return ErrNotImplemented
}

func (v *FSVault) Move(_ context.Context, _, _ string) error {
	return ErrNotImplemented
}

func (v *FSVault) Search(_ context.Context, _ SearchQuery) ([]NoteMeta, error) {
	return nil, ErrNotImplemented
}

func (v *FSVault) Backlinks(_ context.Context, _ string) ([]NoteMeta, error) {
	return nil, ErrNotImplemented
}

func (v *FSVault) Watch(_ context.Context) (<-chan VaultEvent, error) {
	return nil, ErrNotImplemented
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
