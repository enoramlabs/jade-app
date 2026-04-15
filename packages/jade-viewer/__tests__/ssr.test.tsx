// @vitest-environment node
//
// This file runs with vitest's Node environment (not jsdom) so that
// `document` and `window` are genuinely undefined. That mirrors what
// Next.js static generation, Nuxt SSR, SvelteKit SSR, and any other
// server-rendering toolchain does when they execute our module at
// build time.
//
// Regression coverage for the 0.1.0 bug: a transitive dep
// (decode-named-character-reference) shipped a browser-only entry
// point that evaluated `document.createElement("i")` at module init.
// Importing @enoramlabs/jade-viewer in a Node context would throw
// `ReferenceError: document is not defined` before we could even
// render anything. Fixed in 0.1.1 via a Vite plugin that forces the
// non-DOM entry point for that package.

import { describe, it, expect } from 'vitest';
import React from 'react';
import { renderToString } from 'react-dom/server';

describe('SSR safety', () => {
  it('exposes no globalThis.document or globalThis.window', () => {
    // Sanity: this test must run in a genuinely DOM-less environment.
    // If this fails, the @vitest-environment node pragma is not being
    // respected and the other assertions in this file are worthless.
    expect(typeof globalThis.document).toBe('undefined');
    expect(typeof globalThis.window).toBe('undefined');
  });

  it('can be imported in a Node context without throwing', async () => {
    // The dynamic import is what exercises module init. In the 0.1.0
    // bug this line alone would throw.
    const mod = await import('../src/index');
    expect(mod.MarkdownView).toBeDefined();
  });

  it('renders a basic markdown document via renderToString', async () => {
    const { MarkdownView } = await import('../src/index');
    const html = renderToString(
      React.createElement(MarkdownView, {
        source: '# Hello **world**\n\nA paragraph with [[wikilink]] and `code`.',
      }),
    );
    expect(html).toContain('<h1>');
    expect(html).toContain('Hello');
    expect(html).toContain('<strong>');
    expect(html).toContain('world');
    expect(html).toContain('<code>');
  });

  it('renders GFM tables server-side', async () => {
    const { MarkdownView } = await import('../src/index');
    const source = `| A | B |
| --- | --- |
| 1 | 2 |
| 3 | 4 |`;
    const html = renderToString(React.createElement(MarkdownView, { source }));
    expect(html).toContain('<table>');
    expect(html).toContain('<td>1</td>');
    expect(html).toContain('<td>4</td>');
  });

  it('decodes HTML entities server-side (the actual bug)', async () => {
    // This is the exact code path that was broken in 0.1.0. A named
    // character reference like &amp; flows through
    // decode-named-character-reference during parsing. If the wrong
    // entry point is bundled, we either throw at import or silently
    // fail to decode.
    const { MarkdownView } = await import('../src/index');
    const source = 'Tom &amp; Jerry';
    const html = renderToString(React.createElement(MarkdownView, { source }));
    // The ampersand should be preserved through to the output (via
    // react-dom's own escaping) as &amp; in the HTML string, meaning
    // the parser correctly decoded the entity to `&` in the AST.
    expect(html).toContain('Tom');
    expect(html).toContain('Jerry');
    // Negative: should NOT contain the literal raw form as text.
    expect(html).not.toContain('&amp;amp;');
  });

  it('strips frontmatter server-side', async () => {
    const { MarkdownView } = await import('../src/index');
    const source = `---
title: Hidden
---

# Visible`;
    const html = renderToString(React.createElement(MarkdownView, { source }));
    expect(html).toContain('Visible');
    expect(html).not.toContain('title:');
    expect(html).not.toContain('Hidden');
  });
});
