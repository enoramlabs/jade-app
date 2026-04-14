package core

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Wikilink represents a parsed [[wikilink]] or ![[embed]].
type Wikilink struct {
	Target  string // note title or path (before # and |)
	Display string // optional display text after |
	Heading string // optional heading anchor after #
	IsEmbed bool   // true if prefixed with !
}

// ExtractWikilinks returns all wikilinks found in the Markdown body.
// Frontmatter is stripped before scanning.
func ExtractWikilinks(body string) []Wikilink {
	_, cleanBody, _ := ParseFrontmatter(body)
	return extractFromString(cleanBody)
}

// extractFromString scans raw text for [[...]] and ![[...]] patterns.
func extractFromString(s string) []Wikilink {
	var result []Wikilink
	for {
		// Find the opening of a wikilink or embed.
		idx := strings.Index(s, "[[")
		if idx < 0 {
			break
		}
		isEmbed := idx > 0 && s[idx-1] == '!'

		// Find the closing ]].
		inner := s[idx+2:]
		end := strings.Index(inner, "]]")
		if end < 0 {
			break
		}

		content := inner[:end]
		wl := Wikilink{IsEmbed: isEmbed}

		// Split off display text (|...).
		if pipe := strings.IndexByte(content, '|'); pipe >= 0 {
			wl.Display = strings.TrimSpace(content[pipe+1:])
			content = content[:pipe]
		}

		// Split off heading anchor (#...).
		if hash := strings.IndexByte(content, '#'); hash >= 0 {
			wl.Heading = strings.TrimSpace(content[hash+1:])
			content = content[:hash]
		}

		wl.Target = strings.TrimSpace(content)
		if wl.Target != "" {
			result = append(result, wl)
		}

		// Advance past this wikilink.
		s = inner[end+2:]
	}
	return result
}

// ResolveWikilink maps a wikilink target string to a vault-relative note ID.
// Resolution is case-insensitive. It first tries an exact path match
// (target + ".md"), then falls back to filename-only matching (useful
// for nested vaults where [[Note]] should find "folder/Note.md").
// Returns "" if no note matches.
func ResolveWikilink(target string, allIDs []string) string {
	targetLower := strings.ToLower(target)

	// First pass: exact vault-relative path match (case-insensitive).
	// Try both "target" and "target.md".
	for _, id := range allIDs {
		idLower := strings.ToLower(id)
		if idLower == targetLower || idLower == targetLower+".md" {
			return id
		}
	}

	// Second pass: filename-only match (case-insensitive).
	targetBase := targetLower
	if !strings.HasSuffix(targetBase, ".md") {
		targetBase += ".md"
	}
	for _, id := range allIDs {
		base := strings.ToLower(filepath.Base(filepath.FromSlash(id)))
		if base == targetBase {
			return id
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// Goldmark wikilink extension
// ---------------------------------------------------------------------------

// WikilinkNode is a goldmark AST node representing a [[wikilink]] or ![[embed]].
type WikilinkNode struct {
	ast.BaseInline
	Target  string
	Display string
	IsEmbed bool
}

func (n *WikilinkNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{
		"Target":  n.Target,
		"Display": n.Display,
		"IsEmbed": fmt.Sprintf("%v", n.IsEmbed),
	}, nil)
}

// KindWikilink is the NodeKind for WikilinkNode.
var KindWikilink = ast.NewNodeKind("Wikilink")

func (n *WikilinkNode) Kind() ast.NodeKind { return KindWikilink }

// wikilinkParser is a goldmark inline parser for [[...]] and ![[...]].
type wikilinkParser struct{}

var defaultWikilinkParser = &wikilinkParser{}

func (p *wikilinkParser) Trigger() []byte { return []byte{'['} }

func (p *wikilinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	bLine, bSeg := block.Position()
	line, _ := block.PeekLine()

	// We need at least "[[x]]" (5 bytes).
	if len(line) < 5 || line[0] != '[' || line[1] != '[' {
		return nil
	}

	// Find closing ]].
	rest := line[2:]
	end := strings.Index(string(rest), "]]")
	if end < 0 {
		return nil
	}

	content := string(rest[:end])
	node := &WikilinkNode{}

	// Split off display text (|...).
	if pipe := strings.IndexByte(content, '|'); pipe >= 0 {
		node.Display = strings.TrimSpace(content[pipe+1:])
		content = content[:pipe]
	}

	// Split off heading anchor (#...).
	if hash := strings.IndexByte(content, '#'); hash >= 0 {
		content = content[:hash]
	}

	node.Target = strings.TrimSpace(content)
	if node.Target == "" {
		block.SetPosition(bLine, bSeg)
		return nil
	}

	// Advance the reader past "[[...]]".
	block.Advance(2 + end + 2)
	return node
}

// embedParser is a goldmark inline parser for ![[...]].
type embedParser struct{}

var defaultEmbedParser = &embedParser{}

func (p *embedParser) Trigger() []byte { return []byte{'!'} }

func (p *embedParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	bLine, bSeg := block.Position()
	line, _ := block.PeekLine()

	// We need at least "![[x]]" (6 bytes).
	if len(line) < 6 || line[0] != '!' || line[1] != '[' || line[2] != '[' {
		return nil
	}

	rest := line[3:]
	end := strings.Index(string(rest), "]]")
	if end < 0 {
		return nil
	}

	target := strings.TrimSpace(string(rest[:end]))
	if target == "" {
		block.SetPosition(bLine, bSeg)
		return nil
	}

	node := &WikilinkNode{Target: target, IsEmbed: true}
	block.Advance(3 + end + 2)
	return node
}

// wikilinkRenderer renders WikilinkNode to HTML.
type wikilinkRenderer struct{}

func (r *wikilinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindWikilink, r.renderWikilink)
}

func (r *wikilinkRenderer) renderWikilink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	node := n.(*WikilinkNode)
	display := node.Display
	if display == "" {
		display = node.Target
	}
	if node.IsEmbed {
		fmt.Fprintf(w, `<div class="wikilink-embed" data-embed="%s"></div>`,
			escapeAttr(node.Target))
	} else {
		fmt.Fprintf(w, `<a class="wikilink" data-wikilink="%s">%s</a>`,
			escapeAttr(node.Target),
			escapeText(display))
	}
	return ast.WalkContinue, nil
}

// escapeAttr escapes a string for use in an HTML attribute value.
func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeText escapes a string for use in HTML text content.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// WikilinkExtension is a goldmark.Extender that adds wikilink/embed parsing.
type WikilinkExtension struct{}

func (e WikilinkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			// Priority < 100 so we run before goldmark's standard link parser,
			// which also triggers on '[' at priority 100 and would consume [[...]].
			util.Prioritized(defaultEmbedParser, 50),
			util.Prioritized(defaultWikilinkParser, 55),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&wikilinkRenderer{}, 500),
		),
	)
}

// mdWithWikilinks is a goldmark instance that includes the wikilink extension.
var mdWithWikilinks = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		WikilinkExtension{},
	),
)

// RenderMarkdownWithWikilinks converts Markdown source to HTML including
// wikilink/embed rendering. Wikilinks appear as <a data-wikilink="...">.
// Frontmatter is stripped before rendering.
func RenderMarkdownWithWikilinks(source string) string {
	_, cleanBody, _ := ParseFrontmatter(source)
	var buf bytes.Buffer
	if err := mdWithWikilinks.Convert([]byte(cleanBody), &buf); err != nil {
		return ""
	}
	return buf.String()
}
