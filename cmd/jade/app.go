package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/enoramlabs/jade-app/core"
	"github.com/enoramlabs/jade-app/core/config"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// appError is a structured error that crosses the Wails bridge as JSON.
// The frontend parses the error string as JSON to detect structured errors.
type appError struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	CurrentContent string `json:"currentContent,omitempty"`
	CurrentETag    string `json:"currentEtag,omitempty"`
}

func (e *appError) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// toAppError converts a core error to an appError for the Wails bridge.
// Returns nil if err is nil.
func toAppError(err error) error {
	if err == nil {
		return nil
	}
	var ce *core.ConflictError
	if errors.As(err, &ce) {
		ae := &appError{Code: "CONFLICT", Message: err.Error()}
		if ce.Current != nil {
			ae.CurrentContent = ce.Current.Body
			ae.CurrentETag = ce.Current.ETag
		}
		return ae
	}
	var nfe *core.NotFoundError
	if errors.As(err, &nfe) {
		return &appError{Code: "NOT_FOUND", Message: err.Error()}
	}
	var encErr *core.EncodingError
	if errors.As(err, &encErr) {
		return &appError{Code: "ENCODING_ERROR", Message: err.Error()}
	}
	var pte *core.PathTraversalError
	if errors.As(err, &pte) {
		return &appError{Code: "PATH_TRAVERSAL", Message: err.Error()}
	}
	return &appError{Code: "UNKNOWN", Message: err.Error()}
}

// VaultInfo is returned by OpenVault and contains basic vault metadata.
type VaultInfo struct {
	Path string `json:"path"`
}

// StartupState is returned by GetStartupState and tells the frontend what to show.
type StartupState struct {
	VaultPath  string   `json:"vaultPath"`  // empty when no vault is open
	VaultError string   `json:"vaultError"` // non-empty when the last-opened vault was unreachable
	Recent     []string `json:"recent"`     // recent vault paths for the welcome screen
}

// App is the struct bound to the Wails runtime.
// Its exported methods are available as JavaScript promises in the frontend.
type App struct {
	ctx         context.Context
	vault       *core.FSVault
	vaultPath   string             // path of the currently-open vault
	watchCancel context.CancelFunc // cancels the active Watch goroutine

	// Config state.
	cfgPath              string         // explicit config file path (empty → use default)
	cfg                  *config.Config // loaded at startup or on first access
	startupErr           string         // error opening the last vault on startup
	startupVaultOverride string         // --vault flag value from the command line
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.initFromConfig()

	// Restore window bounds from config if available.
	if a.cfg != nil && a.cfg.WindowBounds != nil {
		wb := a.cfg.WindowBounds
		if wb.Width > 0 && wb.Height > 0 {
			runtime.WindowSetSize(ctx, wb.Width, wb.Height)
		}
		if wb.X != 0 || wb.Y != 0 {
			runtime.WindowSetPosition(ctx, wb.X, wb.Y)
		}
	}
}

// shutdown is called by Wails when the app is about to exit.
func (a *App) shutdown(ctx context.Context) {
	if a.cfg != nil {
		// Persist window bounds.
		w, h := runtime.WindowGetSize(ctx)
		x, y := runtime.WindowGetPosition(ctx)
		a.cfg.WindowBounds = &config.WindowBounds{X: x, Y: y, Width: w, Height: h}
		path := a.resolvedCfgPath()
		if path != "" {
			_ = config.Save(path, a.cfg)
		}
	}
	if a.vault != nil {
		_ = a.vault.Close()
	}
}

// initFromConfig loads config and tries to open the last-opened vault.
// Called from startup. Safe to call from tests (ctx may be nil).
func (a *App) initFromConfig() {
	path := a.resolvedCfgPath()
	if path == "" {
		a.cfg = &config.Config{}
		return
	}
	cfg, err := config.Load(path)
	if err != nil {
		cfg = &config.Config{}
	}
	a.cfg = cfg

	// Prefer the --vault flag over the persisted last-opened vault.
	target := a.startupVaultOverride
	if target == "" {
		target = cfg.LastOpenedVault
	}

	if target != "" {
		if _, err := a.openVaultInternal(target); err != nil {
			a.startupErr = fmt.Sprintf("could not reopen %q: %v", target, err)
			// Clear so the welcome screen is shown.
			a.cfg.LastOpenedVault = ""
		}
	}
}

// ensureConfig loads the config lazily if not already loaded.
func (a *App) ensureConfig() {
	if a.cfg != nil {
		return
	}
	path := a.resolvedCfgPath()
	if path == "" {
		return
	}
	cfg, err := config.Load(path)
	if err != nil {
		cfg = &config.Config{}
	}
	a.cfg = cfg
}

// resolvedCfgPath returns the effective config file path.
// Uses a.cfgPath if set, otherwise ~/.jade/config.json.
func (a *App) resolvedCfgPath() string {
	if a.cfgPath != "" {
		return a.cfgPath
	}
	p, err := config.DefaultPath()
	if err != nil {
		return ""
	}
	return p
}

// GetStartupState returns the current app state so the frontend can decide
// whether to show the welcome screen or the main editor view.
func (a *App) GetStartupState() StartupState {
	recent := []string{}
	if a.cfg != nil && len(a.cfg.RecentVaults) > 0 {
		recent = a.cfg.RecentVaults
	}
	return StartupState{
		VaultPath:  a.vaultPath,
		VaultError: a.startupErr,
		Recent:     recent,
	}
}

// RecentVaults returns the list of recently-opened vault paths.
func (a *App) RecentVaults() ([]string, error) {
	a.ensureConfig()
	if a.cfg == nil || a.cfg.RecentVaults == nil {
		return []string{}, nil
	}
	return a.cfg.RecentVaults, nil
}

// CreateVault creates a new vault at path (creates the directory if needed),
// scaffolds a welcome.md, and opens the vault.
// If path is empty and the Wails runtime is available, a native dir-picker is shown.
func (a *App) CreateVault(path string) (VaultInfo, error) {
	if path == "" && a.ctx != nil {
		selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
			Title: "Choose a directory for your new vault",
		})
		if err != nil {
			return VaultInfo{}, err
		}
		if selected == "" {
			return VaultInfo{}, fmt.Errorf("no directory selected")
		}
		path = selected
	} else if path == "" {
		return VaultInfo{}, fmt.Errorf("vault path must not be empty")
	}

	// Create the vault directory.
	if err := os.MkdirAll(path, 0o755); err != nil {
		return VaultInfo{}, fmt.Errorf("create vault directory: %w", err)
	}

	// Scaffold welcome.md (only if it doesn't already exist).
	welcomePath := filepath.Join(path, "welcome.md")
	if _, err := os.Stat(welcomePath); os.IsNotExist(err) {
		welcome := "# Welcome to Jade\n\nThis is your new vault. Create a note to get started.\n"
		if err := os.WriteFile(welcomePath, []byte(welcome), 0o644); err != nil {
			return VaultInfo{}, fmt.Errorf("create welcome.md: %w", err)
		}
	}

	return a.OpenVault(path)
}

// OpenInNewWindow spawns a second Jade process pointing at the given vault path.
// Each process is independent — no cross-talk between vaults.
func (a *App) OpenInNewWindow(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	args := []string{}
	if path != "" {
		args = append(args, "--vault", path)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = os.Environ()
	return cmd.Start()
}

// OpenVault opens a vault at the given filesystem path.
// If path is empty and the Wails runtime is available, a native directory
// dialog is presented. If path is empty and no runtime is available (unit
// tests), an error is returned.
func (a *App) OpenVault(path string) (VaultInfo, error) {
	if path == "" && a.ctx != nil {
		selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
			Title: "Open Vault",
		})
		if err != nil {
			return VaultInfo{}, err
		}
		if selected == "" {
			return VaultInfo{}, fmt.Errorf("no directory selected")
		}
		path = selected
	} else if path == "" {
		return VaultInfo{}, fmt.Errorf("vault path must not be empty")
	}

	info, err := a.openVaultInternal(path)
	if err != nil {
		return info, err
	}

	// Persist to config.
	a.ensureConfig()
	if a.cfg != nil {
		a.cfg.LastOpenedVault = path
		config.AddRecentVault(a.cfg, path)
		if cfgPath := a.resolvedCfgPath(); cfgPath != "" {
			_ = config.Save(cfgPath, a.cfg)
		}
	}

	return info, nil
}

// openVaultInternal opens the vault without touching the config file.
// Used by initFromConfig to avoid recording the auto-open as a user action.
func (a *App) openVaultInternal(path string) (VaultInfo, error) {
	v := core.NewFSVault(path)
	if err := v.Open(context.Background()); err != nil {
		return VaultInfo{}, err
	}

	// Cancel any previously running watcher.
	if a.watchCancel != nil {
		a.watchCancel()
	}

	a.vault = v
	a.vaultPath = path
	a.startWatch()
	return VaultInfo{Path: path}, nil
}

// startWatch launches a goroutine that bridges vault watch events to the Wails
// event bus as "vault.changed" events. It is a no-op when called outside the
// Wails runtime (e.g. in unit tests where a.ctx is nil).
func (a *App) startWatch() {
	if a.ctx == nil {
		return
	}

	watchCtx, cancel := context.WithCancel(a.ctx)
	a.watchCancel = cancel

	ch, err := a.vault.Watch(watchCtx)
	if err != nil {
		cancel()
		return
	}

	go func() {
		for evt := range ch {
			runtime.EventsEmit(a.ctx, "vault.changed", map[string]string{
				"type": string(evt.Type),
				"id":   evt.ID,
			})
		}
	}()
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
// On ETag mismatch the returned error marshals to JSON with code "CONFLICT".
func (a *App) UpdateNote(id string, body string, frontmatter map[string]any, etag string) (*core.Note, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	fullBody, err := core.SerializeFrontmatter(frontmatter, body)
	if err != nil {
		return nil, err
	}
	note, err := a.vault.Update(context.Background(), id, fullBody, frontmatter, etag)
	return note, toAppError(err)
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

// Backlinks returns all notes that contain a wikilink resolving to id.
func (a *App) Backlinks(id string) ([]core.NoteMeta, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.Backlinks(context.Background(), id)
}

// ExportHTML renders a note as HTML using the goldmark GFM + wikilink renderer.
func (a *App) ExportHTML(id string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault is open; call OpenVault first")
	}
	note, err := a.vault.Read(context.Background(), id)
	if err != nil {
		return "", err
	}
	return core.RenderMarkdownWithWikilinks(note.Body), nil
}

// RenderMarkdown renders arbitrary Markdown source to HTML without persisting.
// This is used for the live preview pane. It requires no open vault.
// Wikilinks are rendered as <a data-wikilink="..."> anchors.
func (a *App) RenderMarkdown(source string) string {
	return core.RenderMarkdownWithWikilinks(source)
}

// Search queries the vault's full-text index with an expression string.
// The expression supports: free text, tag:x, key:value, AND, OR, parens.
// Returns ranked NoteMeta results with optional Snippet fields populated.
func (a *App) Search(query string) ([]core.NoteMeta, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault is open; call OpenVault first")
	}
	return a.vault.Search(context.Background(), core.SearchQuery{Text: query})
}

// ResolveWikilink maps a wikilink target string to a canonical vault-relative note ID.
// Resolution is case-insensitive. Returns empty string if no note matches.
func (a *App) ResolveWikilink(target string) (string, error) {
	if a.vault == nil {
		return "", fmt.Errorf("no vault is open; call OpenVault first")
	}
	notes, err := a.vault.List(context.Background(), "")
	if err != nil {
		return "", err
	}
	allIDs := make([]string, len(notes))
	for i, n := range notes {
		allIDs[i] = n.ID
	}
	return core.ResolveWikilink(target, allIDs), nil
}
