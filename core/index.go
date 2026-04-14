package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// indexedNote is the document shape written into the Bleve index.
type indexedNote struct {
	Title  string `json:"title"`
	Tags   string `json:"tags"`   // space-joined tags, e.g. "go tdd dev"
	FM     string `json:"fm"`     // raw frontmatter YAML block for key:value queries
	Body   string `json:"body"`   // markdown body without frontmatter
	Merged string `json:"merged"` // all content merged for default free-text queries
}

// SearchResult is a single search hit with an optional snippet.
type SearchResult struct {
	ID      string
	Snippet string
}

// searchIndex wraps a Bleve index for vault note search.
type searchIndex struct {
	idx bleve.Index
}

// openSearchIndex opens (or creates) a Bleve index for the vault rooted at
// vaultRoot. If indexDir is empty it defaults to ~/.jade/indexes/<hash>/.
func openSearchIndex(vaultRoot, indexDir string) (*searchIndex, error) {
	if indexDir == "" {
		var err error
		indexDir, err = defaultIndexDir(vaultRoot)
		if err != nil {
			return nil, fmt.Errorf("resolving index directory: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(indexDir), 0o755); err != nil {
		return nil, fmt.Errorf("create index parent dirs: %w", err)
	}

	idx, err := bleve.Open(indexDir)
	if err == bleve.ErrorIndexPathDoesNotExist || err == bleve.ErrorIndexMetaMissing {
		// Index doesn't exist or directory is empty — create a fresh one.
		// Remove any partial/empty directory that might block bleve.New.
		_ = os.RemoveAll(indexDir)
		m := buildIndexMapping()
		idx, err = bleve.New(indexDir, m)
		if err != nil {
			return nil, fmt.Errorf("create search index: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("open search index: %w", err)
	}

	return &searchIndex{idx: idx}, nil
}

// defaultIndexDir returns ~/.jade/indexes/<sha256-first-16-bytes-hex-of-vault-root>.
func defaultIndexDir(vaultRoot string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(vaultRoot))
	return filepath.Join(home, ".jade", "indexes", hex.EncodeToString(h[:16])), nil
}

// buildIndexMapping constructs the Bleve index mapping for vault notes.
// All fields use the default text analyzer for language-agnostic stemming.
func buildIndexMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()

	docMapping := bleve.NewDocumentMapping()

	textField := bleve.NewTextFieldMapping()
	textField.Analyzer = "standard"
	textField.Store = true

	docMapping.AddFieldMappingsAt("title", textField)
	docMapping.AddFieldMappingsAt("tags", textField)
	docMapping.AddFieldMappingsAt("fm", textField)
	docMapping.AddFieldMappingsAt("body", textField)
	docMapping.AddFieldMappingsAt("merged", textField)

	im.DefaultMapping = docMapping
	return im
}

// indexNote adds or replaces the Bleve document for the given note.
func (s *searchIndex) indexNote(note *Note) error {
	doc := noteToIndexDoc(note)
	return s.idx.Index(note.ID, doc)
}

// removeNote deletes the document for the given note ID from the index.
func (s *searchIndex) removeNote(id string) error {
	return s.idx.Delete(id)
}

// moveNote re-indexes a note under its new ID, removing the old entry.
func (s *searchIndex) moveNote(oldID, newID string, note *Note) error {
	if err := s.idx.Delete(oldID); err != nil {
		// Non-fatal: old entry might not exist.
		_ = err
	}
	note.NoteMeta.ID = newID
	return s.indexNote(note)
}

// close flushes and closes the underlying Bleve index.
func (s *searchIndex) close() error {
	return s.idx.Close()
}

// search executes a SearchQuery against the Bleve index and returns matching IDs.
func (s *searchIndex) search(q SearchQuery) ([]SearchResult, error) {
	bq := buildBleveQuery(q)

	req := bleve.NewSearchRequest(bq)
	req.Fields = []string{"title"}
	req.Size = 200
	req.Highlight = bleve.NewHighlight()
	req.Highlight.Fields = []string{"merged", "body"}

	res, err := s.idx.Search(req)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		sr := SearchResult{ID: hit.ID}
		// Pick the first available fragment as snippet.
		for _, field := range []string{"merged", "body"} {
			if frags, ok := hit.Fragments[field]; ok && len(frags) > 0 {
				sr.Snippet = frags[0]
				break
			}
		}
		results = append(results, sr)
	}
	return results, nil
}

// noteToIndexDoc converts a Note to an indexedNote document for Bleve.
func noteToIndexDoc(note *Note) indexedNote {
	_, cleanBody, _ := ParseFrontmatter(note.Body)
	fm, _, _ := ParseFrontmatter(note.Body)
	var fmLines []string
	for k, v := range fm {
		fmLines = append(fmLines, fmt.Sprintf("%s %v", k, v))
	}
	fmText := strings.Join(fmLines, " ")
	tags := strings.Join(note.Tags, " ")
	merged := note.Title + " " + tags + " " + fmText + " " + cleanBody

	return indexedNote{
		Title:  note.Title,
		Tags:   tags,
		FM:     fmText,
		Body:   cleanBody,
		Merged: strings.TrimSpace(merged),
	}
}
