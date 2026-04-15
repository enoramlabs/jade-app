# Changelog

All notable changes to `@enoramlabs/jade-viewer` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Since the package is pre-1.0, minor version bumps MAY include breaking changes
as noted in each release.

## [0.1.1] — 2026-04-15

Fixes a server-side rendering crash that blocked 0.1.0 from being
consumable in Next.js App Router, Nuxt SSR, SvelteKit, Astro, or any
other toolchain that evaluates the module graph in a Node environment
at build time.

### Fixed

- **SSR safety**: importing `@enoramlabs/jade-viewer` in a DOM-less
  JS runtime (Next.js static generation, `renderToString`, etc.)
  threw `ReferenceError: document is not defined` during module init.
  Root cause: a transitive dependency (`decode-named-character-reference`)
  ships two implementations via conditional exports:
  - `./index.dom.js` — uses `document.createElement("i")` at module
    init to decode HTML entities via the browser's parser (resolved
    by the `browser` export condition)
  - `./index.js` — a pure-JS character-entities lookup table (the
    default export condition)

  Vite's library-build resolver prefers the `browser` condition, so
  0.1.0's bundle embedded the DOM-dependent version. Fixed by adding
  a top-level Vite plugin that intercepts the import and points it
  to the absolute path of `index.js`, bypassing the exports map.

### Added

- **SSR regression test suite** (`__tests__/ssr.test.tsx`) running
  under vitest's Node environment. Verifies the package can be
  imported in Node without throwing and that `MarkdownView` renders
  via `react-dom/server.renderToString` for basic markdown, GFM
  tables, HTML entities, and frontmatter stripping. The test uses
  `// @vitest-environment node` to genuinely disable jsdom so DOM
  globals are actually undefined (not polyfilled).

### Changed

- Bundle size increased slightly: ~67 kB → ~80 kB gzipped (ESM),
  ~51 kB → ~63 kB gzipped (CJS). The extra ~12 kB is the static
  character-entities lookup table from the pure-JS decoder. No
  runtime performance difference in practice.

### Migration

No API changes. Consumers of 0.1.0 can upgrade with a plain version
bump — no code changes required.

### Consumers to update

- **rc-web**: currently works around 0.1.0 with a `next/dynamic`
  wrapper at `src/components/MarkdownView.tsx`. After upgrading to
  0.1.1, that wrapper can be deleted and pages can import directly
  from `@enoramlabs/jade-viewer` again (which preserves SSR/static
  generation for pages that use markdown).
- **Jade desktop app**: consumed 0.1.0 successfully because it only
  runs in a Wails webview (always has a `document`). Upgrade is
  still worthwhile for the larger character-entities coverage of
  the pure-JS decoder.

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
