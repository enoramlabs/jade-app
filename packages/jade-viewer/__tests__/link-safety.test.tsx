import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { MarkdownView } from '../src/index.js';
import { isUnsafeHref } from '../src/MarkdownView.js';

describe('isUnsafeHref', () => {
  it('flags protocol-relative + backslash-normalized + percent-encoded variants', () => {
    for (const h of [
      '//evil.com',
      '/\\evil.com',
      '\\\\evil.com',
      '\\/evil.com',
      '/%5Cevil.com', // encoded "/\evil.com"
      '  //evil.com', // leading whitespace
    ]) {
      expect(isUnsafeHref(h), h).toBe(true);
    }
  });

  it('allows http(s) / mailto / same-origin relative / anchor', () => {
    for (const h of [
      'https://example.com',
      'http://example.com',
      'mailto:a@b.com',
      '/notes/foo',
      'foo.md',
      '#section',
    ]) {
      expect(isUnsafeHref(h), h).toBe(false);
    }
  });
});

describe('MarkdownView — link safety', () => {
  it('renders an http(s) markdown link as an external anchor (noopener)', () => {
    const { container } = render(<MarkdownView source="[site](https://example.com)" />);
    const a = container.querySelector('a');
    expect(a?.getAttribute('href')).toBe('https://example.com');
    expect(a?.getAttribute('target')).toBe('_blank');
    expect(a?.getAttribute('rel')).toContain('noopener');
  });

  it('autolinks a bare https URL (GFM)', () => {
    const { container } = render(
      <MarkdownView source="see https://example.com here" />,
    );
    expect(container.querySelector('a[href="https://example.com"]')).not.toBeNull();
  });

  it('does NOT render a protocol-relative markdown link as an anchor', () => {
    const { container } = render(<MarkdownView source="[x](//evil.com)" />);
    expect(container.querySelector('a')).toBeNull();
    expect(container.textContent).toContain('x');
  });

  it('does NOT render a javascript: href as an anchor (defense in depth)', () => {
    const { container } = render(<MarkdownView source="[x](javascript:alert(1))" />);
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
  });
});
