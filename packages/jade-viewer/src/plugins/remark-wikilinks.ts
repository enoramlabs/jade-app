/**
 * remark-wikilinks — a minimal remark plugin that recognizes Obsidian-style
 * wikilinks and embeds inside paragraph text and rewrites them as custom
 * MDAST node types that downstream React renderers can pick up.
 *
 * Recognized syntax:
 *
 *   [[target]]                  — simple link
 *   [[target|display]]          — link with alias display text
 *   [[target#heading]]          — link with heading anchor
 *   [[target#heading|display]]  — combined
 *   ![[target]]                 — embed (transclusion)
 *
 * For each match we emit one of two custom node types:
 *
 *   { type: 'jadeWikilink', target, display, heading }
 *   { type: 'jadeEmbed',    target }
 *
 * These nodes live inside phrasing contexts (paragraph, list item, etc.).
 * The React renderer in MarkdownView handles them.
 *
 * This is deliberately NOT a general CommonMark inline parser. It is a
 * regex pass over text nodes, which is sufficient for the Obsidian syntax
 * and avoids pulling in a full micromark extension.
 */

import { visit } from 'unist-util-visit';
import type { Plugin } from 'unified';
import type { Root, Text, PhrasingContent } from 'mdast';

/**
 * Custom MDAST node for a wikilink. Not part of the standard MDAST spec;
 * the React renderer handles it via react-markdown's `components` prop.
 */
export interface JadeWikilinkNode {
  type: 'jadeWikilink';
  target: string;
  display: string;
  heading?: string;
  data?: {
    hName?: string;
    hProperties?: Record<string, unknown>;
    hChildren?: Array<{ type: 'text'; value: string }>;
  };
}

/**
 * Custom MDAST node for an embed.
 */
export interface JadeEmbedNode {
  type: 'jadeEmbed';
  target: string;
  data?: {
    hName?: string;
    hProperties?: Record<string, unknown>;
  };
}

// Matches both [[...]] and ![[...]], capturing the whole inner expression.
// Greedy `]]` terminator — does not support nested `]]`.
const WIKILINK_RE = /(!?)\[\[([^\]\n]+?)\]\]/g;

/**
 * Parse the contents of a `[[...]]` expression into target, display, heading.
 * Input does NOT include the surrounding brackets.
 *
 *   "foo"                  → { target: "foo", display: "foo" }
 *   "foo|bar"              → { target: "foo", display: "bar" }
 *   "foo#sec"              → { target: "foo", display: "foo#sec", heading: "sec" }
 *   "foo#sec|bar"          → { target: "foo", display: "bar", heading: "sec" }
 */
function parseWikilinkInner(inner: string): {
  target: string;
  display: string;
  heading?: string;
} {
  // Split off the alias first (|...)
  const pipeIdx = inner.indexOf('|');
  const pre = pipeIdx === -1 ? inner : inner.slice(0, pipeIdx);
  const alias = pipeIdx === -1 ? undefined : inner.slice(pipeIdx + 1);

  // Split the pre portion into target and optional #heading
  const hashIdx = pre.indexOf('#');
  const target = hashIdx === -1 ? pre : pre.slice(0, hashIdx);
  const heading = hashIdx === -1 ? undefined : pre.slice(hashIdx + 1);

  const display = alias ?? pre;
  return {
    target: target.trim(),
    display: display.trim(),
    heading: heading?.trim(),
  };
}

/**
 * The remark plugin. Attaches no options — wikilinks are always recognized.
 */
export const remarkWikilinks: Plugin<[], Root> = () => {
  return (tree) => {
    visit(tree, 'text', (node: Text, index, parent) => {
      if (!parent || typeof index !== 'number') return;
      if (!WIKILINK_RE.test(node.value)) return;

      // Reset lastIndex after the test() above.
      WIKILINK_RE.lastIndex = 0;

      const children: PhrasingContent[] = [];
      let lastEnd = 0;
      let match: RegExpExecArray | null;

      while ((match = WIKILINK_RE.exec(node.value)) !== null) {
        const [full, bang, inner] = match;
        const start = match.index;
        const end = start + full.length;

        // Preserve any text before this match.
        if (start > lastEnd) {
          children.push({
            type: 'text',
            value: node.value.slice(lastEnd, start),
          } as Text);
        }

        const { target, display, heading } = parseWikilinkInner(inner);

        if (bang === '!') {
          // Embed node. The `data.hName` + `data.hProperties` fields are
          // picked up by mdast-util-to-hast so react-markdown sees a
          // proper HAST element we can target in the `components` map.
          const embed: JadeEmbedNode = {
            type: 'jadeEmbed',
            target,
            data: {
              hName: 'jade-embed',
              hProperties: { 'data-embed': target },
            },
          };
          children.push(embed as unknown as PhrasingContent);
        } else {
          const link: JadeWikilinkNode = {
            type: 'jadeWikilink',
            target,
            display,
            heading,
            data: {
              hName: 'jade-wikilink',
              hProperties: {
                'data-wikilink': target,
                ...(heading ? { 'data-wikilink-heading': heading } : {}),
              },
              hChildren: [{ type: 'text', value: display }],
            },
          };
          children.push(link as unknown as PhrasingContent);
        }

        lastEnd = end;
      }

      // Preserve any text after the last match.
      if (lastEnd < node.value.length) {
        children.push({
          type: 'text',
          value: node.value.slice(lastEnd),
        } as Text);
      }

      // Replace the single text node with the split children.
      parent.children.splice(index, 1, ...children);

      // Skip the nodes we just inserted so visit() does not re-enter.
      return index + children.length;
    });
  };
};
