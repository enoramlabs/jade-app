import { useEffect, useMemo, useState, useCallback } from 'react';
import type { FC, MouseEvent, ReactNode } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize, { defaultSchema } from 'rehype-sanitize';
import type { Schema } from 'hast-util-sanitize';

import type { MarkdownViewProps } from './types.js';
import { remarkWikilinks } from './plugins/remark-wikilinks.js';

/**
 * Extend the default sanitize schema so our custom wikilink/embed
 * elements are allowed through rehype-sanitize. Everything else about
 * the default schema (blocking raw HTML, scripts, event handlers) is
 * preserved.
 */
const sanitizeSchema: Schema = {
  ...defaultSchema,
  tagNames: [
    ...(defaultSchema.tagNames ?? []),
    'jade-wikilink',
    'jade-embed',
  ],
  attributes: {
    ...(defaultSchema.attributes ?? {}),
    'jade-wikilink': ['data-wikilink', 'data-wikilink-heading'],
    'jade-embed': ['data-embed'],
  },
};

/**
 * Strip a leading YAML frontmatter block from the source before parsing.
 *
 * Matches the behavior of Jade's Go-side RenderMarkdown — frontmatter is
 * metadata, not content, and should never appear in rendered output.
 */
function stripFrontmatter(source: string): string {
  if (!source.startsWith('---\n') && !source.startsWith('---\r\n')) {
    return source;
  }
  // Find the closing --- on its own line.
  const lines = source.split(/\r?\n/);
  if (lines[0] !== '---') return source;
  for (let i = 1; i < lines.length; i++) {
    if (lines[i] === '---') {
      return lines.slice(i + 1).join('\n');
    }
  }
  // Unclosed frontmatter — return as-is.
  return source;
}

/**
 * MarkdownView renders CommonMark + GFM + Obsidian-style wikilinks as
 * React elements. It is a pure component: consumers own all state and
 * decide what clicking a wikilink should do.
 */
export const MarkdownView: FC<MarkdownViewProps> = ({
  source,
  onWikilinkClick,
  resolveWikilink,
  codeBlockRenderer,
  embedRenderer,
  className,
  brokenLinkClassName = 'jade-wikilink-broken',
  resolvedLinkClassName = 'jade-wikilink-resolved',
}) => {
  const cleanSource = useMemo(() => stripFrontmatter(source), [source]);

  // Map of target → resolution status. Updated asynchronously as the
  // resolveWikilink callback yields results.
  const [resolutions, setResolutions] = useState<Record<string, 'resolved' | 'broken'>>({});

  // Collect the set of wikilink targets present in the current render,
  // then ask resolveWikilink about each one. Fire-and-forget — stale
  // responses are tolerated because the component keys by target.
  useEffect(() => {
    if (!resolveWikilink) {
      if (Object.keys(resolutions).length > 0) setResolutions({});
      return;
    }
    const targets = new Set<string>();
    const re = /\[\[([^\]\n|#]+?)(?:[|#][^\]\n]*)?\]\]/g;
    let match: RegExpExecArray | null;
    while ((match = re.exec(cleanSource)) !== null) {
      targets.add(match[1].trim());
    }

    let cancelled = false;
    (async () => {
      const next: Record<string, 'resolved' | 'broken'> = {};
      for (const target of targets) {
        try {
          const result = await resolveWikilink(target);
          next[target] = result ? 'resolved' : 'broken';
        } catch {
          next[target] = 'broken';
        }
      }
      if (!cancelled) setResolutions(next);
    })();
    return () => {
      cancelled = true;
    };
    // cleanSource is the primary input; resolveWikilink identity changes
    // should NOT re-trigger unless source also changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cleanSource, resolveWikilink]);

  const handleWikilinkClick = useCallback(
    (e: MouseEvent<HTMLElement>) => {
      if (!onWikilinkClick) return;
      e.preventDefault();
      const target = e.currentTarget.getAttribute('data-wikilink');
      if (target) void onWikilinkClick(target);
    },
    [onWikilinkClick],
  );

  // react-markdown lets us supply a `components` map keyed by tag name.
  // Because remark-wikilinks emits HAST nodes with hName of
  // "jade-wikilink" / "jade-embed", we can intercept them here.
  const components = useMemo(
    () => ({
      // Custom wikilink element — rendered as an anchor-like span.
      'jade-wikilink': (props: { 'data-wikilink'?: string; children?: ReactNode }) => {
        const target = props['data-wikilink'] ?? '';
        const resolution = resolutions[target];
        const classes = [
          'jade-wikilink',
          resolution === 'resolved' ? resolvedLinkClassName : '',
          resolution === 'broken' ? brokenLinkClassName : '',
        ]
          .filter(Boolean)
          .join(' ');
        return (
          <span
            role={onWikilinkClick ? 'link' : undefined}
            tabIndex={onWikilinkClick ? 0 : undefined}
            data-wikilink={target}
            className={classes}
            onClick={handleWikilinkClick}
            onKeyDown={(e) => {
              if (!onWikilinkClick) return;
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                void onWikilinkClick(target);
              }
            }}
          >
            {props.children}
          </span>
        );
      },
      // Custom embed element — consumer-supplied renderer or default placeholder.
      'jade-embed': (props: { 'data-embed'?: string }) => {
        const target = props['data-embed'] ?? '';
        if (embedRenderer) return <>{embedRenderer(target)}</>;
        return (
          <span className="jade-wikilink-embed" data-embed={target}>
            📄 {target}
          </span>
        );
      },
      // Code blocks — consumer-supplied renderer or default <pre><code>.
      code: (props: {
        inline?: boolean;
        className?: string;
        children?: ReactNode;
      }) => {
        const { inline, className: cls, children } = props;
        if (inline) return <code className={cls}>{children}</code>;
        if (codeBlockRenderer) {
          const lang = cls?.replace(/^language-/, '');
          const code = typeof children === 'string' ? children : String(children ?? '');
          return <>{codeBlockRenderer(code, lang)}</>;
        }
        return <code className={cls}>{children}</code>;
      },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [resolutions, onWikilinkClick, handleWikilinkClick, embedRenderer, codeBlockRenderer],
  );

  return (
    <div className={['jade-markdown-view', className].filter(Boolean).join(' ')}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkWikilinks]}
        rehypePlugins={[[rehypeSanitize, sanitizeSchema]]}
        // react-markdown's `components` prop is typed loosely; the cast
        // here is because we add custom tag names that the default types
        // do not know about.
        components={components as never}
      >
        {cleanSource}
      </ReactMarkdown>
    </div>
  );
};

export default MarkdownView;
