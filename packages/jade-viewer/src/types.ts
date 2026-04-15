import type { ReactNode } from 'react';

/**
 * Result of resolving a wikilink target.
 *
 * - `string` — the canonical ID (e.g. "notes/foo.md") of the resolved note
 * - `null` or `undefined` — the target could not be resolved (broken link)
 */
export type WikilinkResolution = string | null | undefined;

/**
 * Props for the {@link MarkdownView} component.
 *
 * The component is a pure renderer — it has no internal state beyond what
 * React hooks it uses for async resolution. Consumers own all state and
 * decide what clicking a wikilink should do.
 */
export interface MarkdownViewProps {
  /**
   * The raw Markdown source to render.
   *
   * Leading YAML frontmatter (`---` fenced) is automatically stripped from
   * the rendered output, matching Obsidian and Jade's desktop behavior.
   */
  source: string;

  /**
   * Called when the user clicks a `[[wikilink]]`.
   *
   * The component handles the click event itself — the consumer just gets
   * the raw target string. It is the consumer's job to resolve the target
   * and navigate, open a modal, etc.
   *
   * If omitted, wikilinks are rendered but non-interactive.
   */
  onWikilinkClick?: (target: string) => void | Promise<void>;

  /**
   * Optional async resolver used to decorate wikilinks with a
   * resolved/broken CSS class.
   *
   * If provided, each wikilink in the rendered output calls this function
   * once to determine whether the target exists. Returning a non-empty
   * string marks the link as resolved; returning `null` or `undefined`
   * marks it as broken.
   *
   * If omitted, all wikilinks are rendered as "unknown" (no class).
   */
  resolveWikilink?: (target: string) => WikilinkResolution | Promise<WikilinkResolution>;

  /**
   * Optional custom renderer for fenced code blocks.
   *
   * If provided, replaces the default `<pre><code>` rendering. Useful for
   * plugging in syntax highlighting (shiki, highlight.js, etc.) without
   * forcing that dependency into the viewer's bundle.
   */
  codeBlockRenderer?: (code: string, language?: string) => ReactNode;

  /**
   * Optional custom renderer for `![[embed]]` transclusions.
   *
   * If provided, replaces the default placeholder that just shows the
   * target name. Useful for recursively rendering the embedded note's
   * body inline.
   */
  embedRenderer?: (target: string) => ReactNode;

  /** Optional `className` to apply to the root element. */
  className?: string;

  /** Optional override for the CSS class applied to unresolved wikilinks. */
  brokenLinkClassName?: string;

  /** Optional override for the CSS class applied to resolved wikilinks. */
  resolvedLinkClassName?: string;
}
