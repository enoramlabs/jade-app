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
