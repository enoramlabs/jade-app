package main

import (
	"context"
	"fmt"

	"github.com/enoramlabs/jade-app/core"
)

// VaultInfo is returned by OpenVault and contains basic vault metadata.
type VaultInfo struct {
	Path string `json:"path"`
}

// App is the struct bound to the Wails runtime.
// Its exported methods are available as JavaScript promises in the frontend.
type App struct {
	ctx   context.Context
	vault *core.FSVault
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// OpenVault opens a vault at the given filesystem path.
// If path is empty, callers should use a native directory dialog first.
func (a *App) OpenVault(path string) (VaultInfo, error) {
	if path == "" {
		return VaultInfo{}, fmt.Errorf("vault path must not be empty")
	}

	v := core.NewFSVault(path)
	if err := v.Open(context.Background()); err != nil {
		return VaultInfo{}, err
	}

	a.vault = v
	return VaultInfo{Path: path}, nil
}

// ListNotes returns metadata for all notes under the given sub-path.
// Pass "" for the vault root.
func (a *App) ListNotes(path string) ([]core.NoteMeta, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.List(context.Background(), path)
}

// ReadNote returns the full Note for the given vault-relative ID.
func (a *App) ReadNote(id string) (*core.Note, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.Read(context.Background(), id)
}

// CreateNote creates a new note at the given vault-relative path with the given body.
// frontmatter is serialized and prepended to body before writing.
func (a *App) CreateNote(path string, body string, frontmatter map[string]any) (*core.Note, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	fullBody, err := core.SerializeFrontmatter(frontmatter, body)
	if err != nil {
		return nil, err
	}
	return a.vault.Create(context.Background(), core.NoteMeta{ID: path}, fullBody)
}

// UpdateNote replaces the body of an existing note.
// etag is used for optimistic concurrency; pass empty string to skip the check.
func (a *App) UpdateNote(id string, body string, frontmatter map[string]any, etag string) (*core.Note, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	fullBody, err := core.SerializeFrontmatter(frontmatter, body)
	if err != nil {
		return nil, err
	}
	return a.vault.Update(context.Background(), id, fullBody, frontmatter, etag)
}

// DeleteNote removes a note permanently.
func (a *App) DeleteNote(id string) error {
	if a.vault == nil {
		return fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.Delete(context.Background(), id)
}

// MoveNote renames/moves a note from fromID to toID.
func (a *App) MoveNote(fromID, toID string) error {
	if a.vault == nil {
		return fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.Move(context.Background(), fromID, toID)
}

// ExportHTML renders a note as HTML using the goldmark GFM renderer.
func (a *App) ExportHTML(id string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault is open; call OpenVault first")
	}
	note, err := a.vault.Read(context.Background(), id)
	if err != nil {
		return "", err
	}
	return core.RenderMarkdown(note.Body), nil
}

// RenderMarkdown renders arbitrary Markdown source to HTML without persisting.
// This is used for the live preview pane. It requires no open vault.
func (a *App) RenderMarkdown(source string) string {
	return core.RenderMarkdown(source)
}
