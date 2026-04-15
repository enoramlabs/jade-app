import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MarkdownView } from '../src/index.js';

describe('MarkdownView — basic rendering', () => {
  it('renders a simple heading', () => {
    render(<MarkdownView source="# Hello" />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Hello');
  });

  it('renders a paragraph with inline emphasis', () => {
    render(<MarkdownView source="This is **bold** and *italic*." />);
    expect(screen.getByText('bold').tagName).toBe('STRONG');
    expect(screen.getByText('italic').tagName).toBe('EM');
  });

  it('strips leading YAML frontmatter from the rendered output', () => {
    const source = `---
title: My note
tags: [foo, bar]
---

# Body Heading

Body paragraph.`;
    const { container } = render(<MarkdownView source={source} />);
    expect(container.textContent).not.toContain('title:');
    expect(container.textContent).not.toContain('tags:');
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Body Heading');
  });

  it('renders GFM tables', () => {
    const source = `| A | B |
| --- | --- |
| 1 | 2 |
| 3 | 4 |`;
    render(<MarkdownView source={source} />);
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
  });

  it('renders GFM task list checkboxes', () => {
    const source = `- [x] done\n- [ ] todo`;
    const { container } = render(<MarkdownView source={source} />);
    const checkboxes = container.querySelectorAll('input[type="checkbox"]');
    expect(checkboxes.length).toBe(2);
    expect((checkboxes[0] as HTMLInputElement).checked).toBe(true);
    expect((checkboxes[1] as HTMLInputElement).checked).toBe(false);
  });

  it('escapes raw HTML via the sanitize plugin', () => {
    const source = `Hello <script>alert('xss')</script> world.`;
    const { container } = render(<MarkdownView source={source} />);
    expect(container.querySelector('script')).toBeNull();
  });
});

describe('MarkdownView — wikilinks', () => {
  it('renders a simple [[wikilink]] as a clickable span', () => {
    const { container } = render(<MarkdownView source="See [[foo]]." onWikilinkClick={() => {}} />);
    const link = container.querySelector('[data-wikilink="foo"]');
    expect(link).not.toBeNull();
    expect(link?.textContent).toBe('foo');
    expect(link?.getAttribute('role')).toBe('link');
  });

  it('renders [[target|display]] with the alias text', () => {
    const { container } = render(<MarkdownView source="See [[notes/foo|the foo note]]." />);
    const link = container.querySelector('[data-wikilink="notes/foo"]');
    expect(link).not.toBeNull();
    expect(link?.textContent).toBe('the foo note');
  });

  it('calls onWikilinkClick with the target when clicked', () => {
    const onClick = vi.fn();
    const { container } = render(
      <MarkdownView source="See [[foo]] and [[bar|baz]]." onWikilinkClick={onClick} />,
    );
    const foo = container.querySelector('[data-wikilink="foo"]') as HTMLElement;
    const bar = container.querySelector('[data-wikilink="bar"]') as HTMLElement;
    fireEvent.click(foo);
    fireEvent.click(bar);
    expect(onClick).toHaveBeenCalledTimes(2);
    expect(onClick).toHaveBeenNthCalledWith(1, 'foo');
    expect(onClick).toHaveBeenNthCalledWith(2, 'bar');
  });

  it('marks unresolved wikilinks with the broken class after resolveWikilink returns null', async () => {
    const resolve = vi.fn().mockResolvedValue(null);
    const { container } = render(
      <MarkdownView source="See [[ghost]]." resolveWikilink={resolve} />,
    );
    await waitFor(() => {
      const link = container.querySelector('[data-wikilink="ghost"]');
      expect(link?.className).toContain('jade-wikilink-broken');
    });
    expect(resolve).toHaveBeenCalledWith('ghost');
  });

  it('marks resolved wikilinks with the resolved class', async () => {
    const resolve = vi.fn().mockResolvedValue('notes/real.md');
    const { container } = render(
      <MarkdownView source="See [[real]]." resolveWikilink={resolve} />,
    );
    await waitFor(() => {
      const link = container.querySelector('[data-wikilink="real"]');
      expect(link?.className).toContain('jade-wikilink-resolved');
    });
  });

  it('renders ![[embed]] with the default placeholder', () => {
    const { container } = render(<MarkdownView source="![[other]]" />);
    const embed = container.querySelector('[data-embed="other"]');
    expect(embed).not.toBeNull();
    expect(embed?.textContent).toContain('other');
  });

  it('uses custom embedRenderer when provided', () => {
    const { container } = render(
      <MarkdownView
        source="![[chart]]"
        embedRenderer={(target) => <div className="custom-embed">EMBED:{target}</div>}
      />,
    );
    const custom = container.querySelector('.custom-embed');
    expect(custom).not.toBeNull();
    expect(custom?.textContent).toBe('EMBED:chart');
  });
});

describe('MarkdownView — code blocks', () => {
  it('renders fenced code blocks as <pre><code> by default', () => {
    const source = '```js\nconst x = 1;\n```';
    const { container } = render(<MarkdownView source={source} />);
    const code = container.querySelector('pre code');
    expect(code).not.toBeNull();
    expect(code?.textContent).toContain('const x = 1;');
  });

  it('uses custom codeBlockRenderer when provided', () => {
    const renderer = vi.fn((code: string, lang?: string) => (
      <pre data-lang={lang}>{`CUSTOM:${code}`}</pre>
    ));
    const source = '```ts\nlet y = 2;\n```';
    const { container } = render(<MarkdownView source={source} codeBlockRenderer={renderer} />);
    const custom = container.querySelector('pre[data-lang="ts"]');
    expect(custom).not.toBeNull();
    expect(custom?.textContent).toContain('CUSTOM:');
    expect(renderer).toHaveBeenCalled();
  });
});
