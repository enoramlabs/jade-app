package core

import (
	"context"
	"time"
)

// NoteMeta holds metadata about a note without loading the full body.
type NoteMeta struct {
	ID          string         // vault-relative path, forward-slash, e.g. "folder/note.md"
	Title       string         // derived from filename or frontmatter
	Tags        []string       // frontmatter tags
	Frontmatter map[string]any // parsed frontmatter fields
	ModTime     time.Time      // last modification time
}

// Note is a full note including body content.
type Note struct {
	NoteMeta
	Body string // raw Markdown source (including frontmatter)
	ETag string // SHA-256 hash of Body, for optimistic concurrency
}

// SearchQuery describes a search request.
type SearchQuery struct {
	Text string   // free-text query
	Tags []string // must match all tags
	// Frontmatter key:value filters
	Filters map[string]string
}

// VaultEventType classifies a filesystem event.
type VaultEventType string

const (
	EventCreate VaultEventType = "create"
	EventUpdate VaultEventType = "update"
	EventDelete VaultEventType = "delete"
	EventRename VaultEventType = "rename"
)

// VaultEvent is emitted by Watch when the vault changes.
type VaultEvent struct {
	Type VaultEventType
	ID   string // affected note path
}

// Vault is the primary interface for all vault operations.
// Implementations must be safe for concurrent use.
type Vault interface {
	// Open initialises the vault (scans the directory, builds indexes).
	// Returns an error if the root path does not exist.
	Open(ctx context.Context) error

	// Close flushes indexes and releases resources.
	Close() error

	// List returns metadata for all .md files under the given sub-path.
	// Pass "" or "." for the vault root.
	List(ctx context.Context, path string) ([]NoteMeta, error)

	// Read returns the full Note for the given ID.
	// ID is a vault-relative path (forward-slash, e.g. "folder/note.md").
	Read(ctx context.Context, id string) (*Note, error)

	// Create creates a new note at the given path with the provided body.
	Create(ctx context.Context, note NoteMeta, body string) (*Note, error)

	// Update replaces the body and frontmatter of an existing note.
	// etag must match the current on-disk ETag or ConflictError is returned.
	Update(ctx context.Context, id string, body string, frontmatter map[string]any, etag string) (*Note, error)

	// Delete removes a note permanently.
	Delete(ctx context.Context, id string) error

	// Move renames/moves a note from fromID to toID.
	Move(ctx context.Context, fromID, toID string) error

	// Search returns notes matching the query.
	Search(ctx context.Context, query SearchQuery) ([]NoteMeta, error)

	// Backlinks returns all notes that contain a wikilink resolving to id.
	Backlinks(ctx context.Context, id string) ([]NoteMeta, error)

	// Watch returns a channel of vault events. The channel is closed when ctx
	// is cancelled.
	Watch(ctx context.Context) (<-chan VaultEvent, error)
}
