# Changelog

All notable changes to `@enoramlabs/jade-viewer` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Since the package is pre-1.0, minor version bumps MAY include breaking changes
as noted in each release.

## [0.1.0] — 2026-04-14

First public release. Shipping the core `MarkdownView` React component so
advanceai, rc-web, and the Jade desktop app can all share one markdown
renderer.

### Added

- `MarkdownView` React component that renders CommonMark + GitHub Flavored
  Markdown + Obsidian-style wikilinks + embeds, safe-by-default.
- `remark-wikilinks` plugin recognizing `[[target]]`, `[[target|display]]`,
  `[[target#heading]]`, and `![[embed]]` syntax.
- `onWikilinkClick` prop — pure callback, consumer decides navigation.
- `resolveWikilink` prop — async resolver that marks wikilinks as
  resolved/broken via CSS classes.
- `codeBlockRenderer` prop — slot for consumer-supplied syntax highlighting
  (e.g. shiki, highlight.js). No highlighter bundled.
- `embedRenderer` prop — slot for custom `![[embed]]` rendering (e.g.
  recursive transclusion).
- Automatic YAML frontmatter stripping on the rendered output, matching
  Jade desktop's behavior.
- XSS-safe by default via `rehype-sanitize` — raw HTML, scripts, iframes,
  and event handlers are stripped. Custom `jade-wikilink` / `jade-embed`
  elements are whitelisted in the schema.
- Optional default dark theme (`@enoramlabs/jade-viewer/styles.css`) with
  Catppuccin-inspired colors and CSS custom properties for overriding.
- Interactive demo page (`npm run demo`) with two modes:
  - **Chat mode**: simulated AI streaming chat that tests partial markdown
    rendering mid-stream.
  - **Sandbox mode**: editable split-pane with a rich sample document.
- 15 unit tests covering basic rendering, frontmatter stripping, GFM tables,
  task lists, XSS escaping, wikilinks (simple, aliased, click, resolved,
  broken), embeds, and custom renderers.

### Technical notes

- **Peer dependencies**: `react` ^18 or ^19, `react-dom` ^18 or ^19. Never
  bundled.
- **Bundle size**: ~67 kB gzipped ESM (`dist/index.js`), ~51 kB gzipped CJS
  (`dist/index.cjs`), plus 3.7 kB of optional CSS.
- **Runtime dependencies**: `react-markdown`, `remark-gfm`, `rehype-sanitize`,
  `unist-util-visit`. All ESM-first.
- **Module format**: dual ESM + CJS via Vite library build, types via `tsc`.
- **No CGO / native dependencies** — runs in any browser or JS runtime that
  can render React.

### Known limitations

- Jade desktop currently renders markdown via goldmark (Go side), while this
  package uses `react-markdown` (JS side). A shared Go↔JS compatibility test
  suite is planned for 0.2.0 so both parsers guarantee identical output.
- No syntax highlighting for code blocks out of the box — pass a
  `codeBlockRenderer` prop if you want one.
- No math rendering (KaTeX) or diagram rendering (Mermaid). These are
  intentionally deferred to keep the bundle small; they may ship as
  separate companion packages later.
- Wikilink resolution happens per-unique-target on every `source` change.
  For very long documents with many wikilinks this could be slow — consider
  memoizing your `resolveWikilink` callback.
