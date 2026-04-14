package core

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// md is a shared goldmark instance with GFM extensions enabled.
// Raw HTML is not rendered (unsafe=false), so untrusted input is safe.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM, // tables, strikethrough, task lists, autolinks
	),
)

// RenderMarkdown converts Markdown source to an HTML string.
// It strips YAML frontmatter before rendering and does not allow raw HTML
// injection (unsafe option is off).
//
// It is a pure function: no I/O, no global state mutations, safe for
// concurrent use.
func RenderMarkdown(source string) string {
	_, cleanBody, _ := ParseFrontmatter(source)

	var buf bytes.Buffer
	if err := md.Convert([]byte(cleanBody), &buf); err != nil {
		// goldmark errors are extremely rare for valid UTF-8; return empty string.
		return ""
	}
	return buf.String()
}
