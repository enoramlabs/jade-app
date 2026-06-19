# Jade

**Local-first Markdown knowledge base** — a fast, offline desktop app for your
notes, plus a shareable Markdown renderer your other apps can reuse.

Jade is an Obsidian-style notes app: point it at a folder of `.md` files (a
"vault") and get a live editor, instant full-text search, `[[wikilinks]]`,
backlinks, and a real-time preview — all running locally against plain files
on disk. No account, no cloud, no lock-in.

This repo is a small monorepo with two deliverables:

| Path | What it is |
|---|---|
| [`cmd/jade`](cmd/jade) + [`core`](core) | **The Jade desktop app** — a [Wails v2](https://wails.io) app (Go backend + React/TypeScript frontend). |
| [`packages/jade-viewer`](packages/jade-viewer) | **`@enoramlabs/jade-viewer`** — Jade's Markdown renderer as a standalone React component, published to npm so other apps render Markdown identically. |

---

## The Jade desktop app

A two-pane editor over a local vault of Markdown notes.

**Features**
- **Vaults** — open or create any folder of `.md` files; recent vaults are remembered.
- **Editor + live preview** — a CodeMirror Markdown editor beside a live preview powered by `@enoramlabs/jade-viewer`.
- **Wikilinks & backlinks** — `[[Note]]`, `[[Note|alias]]`, `[[Note#heading]]`, and `![[embed]]`; a backlinks panel shows what links to the current note.
- **Full-text search** — fast, debounced search with highlighted snippets.
- **Frontmatter** — leading YAML (`--- … ---`) is parsed for metadata and hidden from the preview.
- **Safe by design** — files are watched on disk (`vault.changed` events), and concurrent external edits are caught with an ETag conflict dialog (keep mine / keep theirs).

**Architecture**
- [`core/`](core) — the pure-Go vault engine: `fsvault` (filesystem store), `parser` ([goldmark](https://github.com/yuin/goldmark)), `index`, `wikilink`, `search`, `frontmatter`, `watch`. No UI, fully unit-tested (`go test ./core/...`).
- [`cmd/jade/`](cmd/jade) — the Wails shell: `App` binds the core engine to the frontend (`OpenVault`, `ListNotes`, `ReadNote`, `CreateNote`, `UpdateNote`, `DeleteNote`, `MoveNote`, `Backlinks`, `ResolveWikilink`, `Search`, …).
- [`cmd/jade/frontend/`](cmd/jade/frontend) — the React/Vite UI (note tree, editor, preview, dialogs).

### Run it

**Prerequisites**
- [Go](https://go.dev/dl/) **1.23+**
- [Node.js](https://nodejs.org/) **20+**
- The [Wails CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
  (and its [platform dependencies](https://wails.io/docs/gettingstarted/installation#platform-specific-dependencies) — e.g. WebKit on Linux, WebView2 on Windows).

**Develop** (hot-reloads Go + frontend):
```sh
cd cmd/jade
wails dev
```

**Build a native binary** (output in `cmd/jade/build/bin`):
```sh
cd cmd/jade
wails build
```

**Open a specific vault at launch:**
```sh
./build/bin/jade --vault /path/to/your/vault
```

### Test
```sh
go test ./core/... -v               # the vault engine (no CGO/Wails required)
go test -tags unit ./cmd/jade/...   # app unit tests (main.go is build-tagged `!unit`)
```

---

## `@enoramlabs/jade-viewer` — the shared Markdown renderer

A pure React component that renders the same Markdown Jade writes — **GitHub
Flavored Markdown + Obsidian-style wikilinks + safe-by-default sanitization** —
so **advanceai**, **rc-web**, the Jade desktop app, and any chat/preview UI all
render notes identically. Write a note once; show it the same everywhere.

```sh
npm install @enoramlabs/jade-viewer
```

```tsx
import { MarkdownView } from "@enoramlabs/jade-viewer";
import "@enoramlabs/jade-viewer/styles.css"; // optional default theme

// e.g. render an agent/chat message body as rich Markdown instead of raw text:
function ChatMessage({ body }: { body: string }) {
  return <MarkdownView source={body} />;
}
```

It's a pure renderer — you own navigation and note lookup via `onWikilinkClick`
/ `resolveWikilink`, and can plug in your own syntax highlighter via
`codeBlockRenderer`. Full props, examples, and the wikilink contract are in the
package README: **[packages/jade-viewer/README.md](packages/jade-viewer/README.md)**.

### Develop the package
```sh
cd packages/jade-viewer
npm install
npm run dev        # live playground (Vite)
npm run demo       # standalone demo page
npm test           # vitest
npm run build      # emit dist/ (esm + cjs + types + styles.css)
```

It publishes to npm via [`.github/workflows/publish-jade-viewer.yml`](.github/workflows/publish-jade-viewer.yml).

---

## Repository layout

```
.
├── cmd/jade/            # Wails desktop app (Go + React frontend)
│   ├── app.go           #   the App: binds core → frontend
│   ├── main.go          #   Wails bootstrap
│   └── frontend/        #   React/Vite UI
├── core/                # pure-Go vault engine (no UI)
├── packages/
│   └── jade-viewer/     # @enoramlabs/jade-viewer (published React renderer)
└── .github/workflows/   # ci.yml + publish-jade-viewer.yml
```

## Contributing

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs `gofmt`,
`go vet`, the `core` tests, and a frontend build on every push — keep them
green. The `core` engine is the source of truth for vault behavior; prefer
adding logic there (with table tests) over the Wails/React layer.

## License

The `@enoramlabs/jade-viewer` package is **MIT** (see
[`packages/jade-viewer/package.json`](packages/jade-viewer/package.json)). The
desktop app has no top-level `LICENSE` file yet — add one to make the repo's
terms explicit.
