package core

import "fmt"

// ErrNotImplemented is returned by Vault methods not yet implemented.
var ErrNotImplemented = fmt.Errorf("not implemented")

// NotFoundError is returned when a note does not exist.
type NotFoundError struct {
	ID string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("note not found: %q", e.ID)
}

// ConflictError is returned when an ETag mismatch is detected on write.
type ConflictError struct {
	ID      string
	Current *Note
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict on note %q: disk content has changed since last read", e.ID)
}

// EncodingError is returned when a file cannot be decoded as UTF-8.
type EncodingError struct {
	Path string
}

func (e *EncodingError) Error() string {
	return fmt.Sprintf("encoding error: file %q is not valid UTF-8", e.Path)
}

// PathTraversalError is returned when a path escapes the vault root.
type PathTraversalError struct {
	ID string
}

func (e *PathTraversalError) Error() string {
	return fmt.Sprintf("path traversal rejected: %q", e.ID)
}
