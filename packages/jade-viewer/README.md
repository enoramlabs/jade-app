# @enoramlabs/jade-viewer

Jade's markdown viewer as a React component. GFM + Obsidian-style wikilinks + safe rendering, client-side.

This package exists so **advanceai**, **rc-web**, and the **Jade desktop app** can all share exactly one markdown renderer. Write notes once in Jade, render them identically in your chat UI, your scheduler view, your mailbox preview.

## Features

- CommonMark + **GitHub Flavored Markdown** (tables, task lists, strikethrough, autolinks) via `remark-gfm`
- **Obsidian-style wikilinks**: `[[target]]`, `[[target|display]]`, `[[target#heading]]`, `![[embed]]`
- **Frontmatter stripping** — leading YAML blocks are hidden from rendered output, matching Jade desktop
- **Safe by default** — raw HTML, scripts, and event handlers are stripped via `rehype-sanitize`
- **Pure component** — no internal state beyond async wikilink resolution. Consumers own all navigation and note lookup.
- **Zero parser bundled on your behalf** — you can plug in your own code syntax highlighter via `codeBlockRenderer`
- **Small**: ~50 KB gzipped (react + react-dom are peer dependencies)

## Install

```sh
npm install @enoramlabs/jade-viewer
# or
pnpm add @enoramlabs/jade-viewer
# or
yarn add @enoramlabs/jade-viewer
```

React 18 or 19 is a peer dependency.

## Quick start

```tsx
import { MarkdownView } from '@enoramlabs/jade-viewer';
import '@enoramlabs/jade-viewer/styles.css'; // optional default theme

function ChatMessage({ body }: { body: string }) {
  return (
    <MarkdownView
      source={body}
      onWikilinkClick={(target) => {
        // Navigate, open a modal, log an event — your call.
        router.push(`/notes/${target}`);
      }}
    />
  );
}
```

## Wikilink resolution

Pass `resolveWikilink` to mark links as resolved or broken. The component calls it once per unique wikilink target, asynchronously, and applies a CSS class based on the result:

```tsx
<MarkdownView
  source={note.body}
  resolveWikilink={async (target) => {
    const note = await noteStore.findByTarget(target);
    return note?.id ?? null; // null = broken (red dashed underline)
  }}
  onWikilinkClick={(target) => openNote(target)}
/>
```

`.jade-wikilink-broken` and `.jade-wikilink-resolved` are the default class names; override via `brokenLinkClassName` and `resolvedLinkClassName` props.

## Code syntax highlighting (opt-in)

Jade-viewer does NOT bundle a syntax highlighter — it would force every consumer to ship shiki or highlight.js whether they want it or not. Instead, pass a `codeBlockRenderer`:

```tsx
import { MarkdownView } from '@enoramlabs/jade-viewer';
import { Highlight } from 'prism-react-renderer';

<MarkdownView
  source={note.body}
  codeBlockRenderer={(code, lang) => (
    <Highlight code={code} language={lang ?? 'text'}>
      {/* ... your highlight renderer ... */}
    </Highlight>
  )}
/>;
```

## Custom embed rendering

By default, `![[target]]` renders as a minimal placeholder showing the target name. For true transclusion (rendering the embedded note's body inline), supply `embedRenderer`:

```tsx
<MarkdownView
  source={note.body}
  embedRenderer={(target) => {
    const embedded = noteStore.get(target);
    if (!embedded) return <span>📄 {target} (not found)</span>;
    return <MarkdownView source={embedded.body} />; // recursive
  }}
/>
```

## Theming

Import `@enoramlabs/jade-viewer/styles.css` for a default dark Catppuccin-inspired theme, or write your own CSS against `.jade-markdown-view` and its descendants. The default theme uses CSS custom properties that you can override:

```css
.my-chat-container {
  --jade-fg-override: #222;
  --jade-link-override: #0066cc;
  --jade-bg-code-override: #f5f5f5;
}
```

## API reference

See `src/types.ts` for the full prop interface.

## Consistency with Jade desktop

Jade desktop renders markdown via goldmark on the Go side. This package renders via `react-markdown` + `remark-gfm` + a custom wikilink plugin on the JS side. The two parsers are tested against shared fixtures in `fixtures/markdown-compat.json` to guarantee identical output for every supported syntax.

If you find a case where Jade desktop and jade-viewer render the same markdown differently, it's a bug — file an issue.

## License

MIT
