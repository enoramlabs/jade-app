import { useCallback, useState } from 'react';
import { MarkdownView } from '../src/index';

const SAMPLE = `---
title: Jade Viewer Demo
tags: [demo, markdown, wikilinks]
status: draft
---

# Jade Viewer — Sandbox Mode

This page renders whatever you type on the left, live, using the
\`@enoramlabs/jade-viewer\` React component.

Edit anything — try adding wikilinks, tables, task lists, or even
\`<script>\` tags (they'll be stripped).

## Paragraphs & inline

A paragraph with **bold**, *italic*, ~~strikethrough~~, and a bit of
\`inline code\`. Autolinked: https://github.com/enoramlabs/jade-app

## GFM tables

| Feature         | Status  | Notes                        |
| --------------- | ------- | ---------------------------- |
| Headings        | ✅      | All levels                   |
| GFM tables      | ✅      | Via remark-gfm               |
| Task lists      | ✅      | Read-only checkboxes         |
| Wikilinks       | ✅      | Obsidian-style syntax        |
| Frontmatter     | ✅      | Stripped from output         |
| Raw HTML        | ❌      | Blocked by rehype-sanitize   |

## Task lists

- [x] Scaffold the package
- [x] Implement MarkdownView
- [x] Write 15 unit tests
- [ ] Ship a demo page
- [ ] Integrate into advanceai

## Wikilinks

### Resolved links (blue, dotted underline)

- [[resolved-note]] — a link that exists
- [[notes/deep/page|custom alias]] — aliased resolved link

### Broken links (red, dashed underline)

- [[ghost]] — a link that does not exist
- [[never-written]] — another broken one

### Other variants

- [[target#section]] — link with heading anchor
- ![[embed]] — transclusion placeholder

## Code blocks

\`\`\`typescript
import { MarkdownView } from '@enoramlabs/jade-viewer';

function ChatMessage({ body }: { body: string }) {
  return (
    <MarkdownView
      source={body}
      onWikilinkClick={(target) => router.push(\`/notes/\${target}\`)}
    />
  );
}
\`\`\`

Inline code: \`const x = 42;\`

## Lists

Nested bullet list:

- Top level
  - Second level
    - Third level
  - Back to second
- Top level again

Numbered:

1. First
2. Second
3. Third

## Blockquote

> This is a blockquote.
> It can span multiple lines.
>
> And have multiple paragraphs.

## Raw HTML is blocked

The following is a live XSS attempt that gets stripped by rehype-sanitize:

<script>alert('pwned')</script>

<iframe src="https://evil.example.com"></iframe>

## Horizontal rule

---

And we're done. Try editing anything on the left!
`;

const RESOLVED = new Set(['resolved-note', 'notes/deep/page', 'target']);

async function fakeResolve(target: string): Promise<string | null> {
  await new Promise((r) => setTimeout(r, 120));
  return RESOLVED.has(target) ? `notes/${target}.md` : null;
}

export function SandboxMode() {
  const [source, setSource] = useState(SAMPLE);
  const [clickLog, setClickLog] = useState<string[]>([]);

  const handleClick = useCallback((target: string) => {
    const entry = `${new Date().toLocaleTimeString()} → clicked [[${target}]]`;
    setClickLog((prev) => [entry, ...prev].slice(0, 5));
  }, []);

  const handleReset = useCallback(() => setSource(SAMPLE), []);
  const handleClear = useCallback(() => setSource(''), []);

  return (
    <div className="sandbox-root">
      <div className="sandbox-actions-bar">
        <button className="demo-btn" onClick={handleReset}>
          Reset sample
        </button>
        <button className="demo-btn" onClick={handleClear}>
          Clear
        </button>
      </div>

      <main className="demo-split">
        <section className="demo-pane demo-editor-pane">
          <div className="demo-pane-label">Source (editable)</div>
          <textarea
            className="demo-editor"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            spellCheck={false}
          />
        </section>

        <section className="demo-pane demo-preview-pane">
          <div className="demo-pane-label">Rendered (MarkdownView)</div>
          <div className="demo-preview-scroll">
            <MarkdownView
              source={source}
              onWikilinkClick={handleClick}
              resolveWikilink={fakeResolve}
              className="demo-md"
            />
          </div>
        </section>
      </main>

      <footer className="demo-footer">
        <div className="demo-footer-section">
          <div className="demo-footer-label">Wikilink click log</div>
          {clickLog.length === 0 ? (
            <div className="demo-footer-empty">Click a wikilink in the preview above…</div>
          ) : (
            <ul className="demo-footer-list">
              {clickLog.map((entry, i) => (
                <li key={i}>{entry}</li>
              ))}
            </ul>
          )}
        </div>
        <div className="demo-footer-section demo-footer-legend">
          <div className="demo-footer-label">Legend</div>
          <div className="demo-legend-row">
            <span className="demo-legend-swatch demo-legend-resolved" /> resolved
          </div>
          <div className="demo-legend-row">
            <span className="demo-legend-swatch demo-legend-broken" /> broken (unresolved)
          </div>
          <div className="demo-legend-row">
            <span className="demo-legend-swatch demo-legend-embed" /> ![[embed]] placeholder
          </div>
        </div>
      </footer>
    </div>
  );
}
