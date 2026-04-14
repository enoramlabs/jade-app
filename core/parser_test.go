package core_test

import (
	"strings"
	"testing"

	"github.com/enoramlabs/jade-app/core"
)

func TestRenderMarkdown_renders_heading(t *testing.T) {
	html := core.RenderMarkdown("# Hello World")
	if !strings.Contains(html, "<h1>Hello World</h1>") {
		t.Errorf("expected <h1>Hello World</h1> in output, got: %s", html)
	}
}

func TestRenderMarkdown_renders_gfm_table(t *testing.T) {
	src := "| A | B |\n|---|---|\n| 1 | 2 |\n"
	html := core.RenderMarkdown(src)
	if !strings.Contains(html, "<table>") {
		t.Errorf("expected <table> in output, got: %s", html)
	}
}

func TestRenderMarkdown_renders_task_list(t *testing.T) {
	src := "- [x] done\n- [ ] todo\n"
	html := core.RenderMarkdown(src)
	if !strings.Contains(html, `type="checkbox"`) {
		t.Errorf("expected checkbox input in task list output, got: %s", html)
	}
}

func TestRenderMarkdown_renders_strikethrough(t *testing.T) {
	html := core.RenderMarkdown("~~deleted~~")
	if !strings.Contains(html, "<del>deleted</del>") {
		t.Errorf("expected <del>deleted</del> in output, got: %s", html)
	}
}

func TestRenderMarkdown_strips_frontmatter(t *testing.T) {
	src := "---\ntitle: My Note\ntags:\n  - go\n---\n# Hello\n\nBody text."
	html := core.RenderMarkdown(src)
	if strings.Contains(html, "title: My Note") {
		t.Errorf("frontmatter should be stripped from rendered HTML, got: %s", html)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") {
		t.Errorf("expected heading rendered after stripping frontmatter, got: %s", html)
	}
}

func TestRenderMarkdown_escapes_raw_html(t *testing.T) {
	src := "<script>alert('xss')</script>\n\nHello."
	html := core.RenderMarkdown(src)
	if strings.Contains(html, "<script>") {
		t.Errorf("raw <script> tag should be escaped, got: %s", html)
	}
}
