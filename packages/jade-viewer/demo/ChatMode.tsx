import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { MarkdownView } from '../src/index';

/**
 * ChatMode — simulates an AI chat surface like the one in advanceai or
 * rc-web. The point of this demo is NOT the chat UX itself; it is to
 * prove that MarkdownView handles a realistic streaming scenario where
 * the source grows character-by-character and the rendered output must
 * stay correct at every intermediate state.
 *
 * In production, the body of each assistant message would come from an
 * SSE stream off an LLM. Here we fake it by picking a canned response
 * and chunking it into small pieces with a jittered delay, so you can
 * see the markdown rendering grow live: headings open mid-stream, code
 * fences close on the last chunk, wikilinks resolve as they arrive,
 * tables render row-by-row, etc.
 */

type Role = 'user' | 'assistant';

interface Message {
  id: string;
  role: Role;
  body: string;
  streaming?: boolean;
}

// A small pool of canned assistant responses that collectively exercise
// every markdown feature the viewer supports. When the user hits Send,
// one of these is streamed back as the assistant reply.
const CANNED_RESPONSES: string[] = [
  // 1 — table + wikilink + inline emphasis
  `Sure! Here's how the main desktop-wrapper frameworks compare:

| Framework  | Language | Webview    | Binary size |
| ---------- | -------- | ---------- | ----------- |
| **Wails**  | Go       | System     | ~15 MB      |
| **Tauri**  | Rust     | System     | ~10 MB      |
| **Electron** | Node   | Chromium   | ~200 MB     |

The tl;dr: use system webviews unless you *need* Chromium-identical
rendering everywhere. More context at [[frameworks/desktop-wrappers]]
and [[architecture/runtime-choice]].`,

  // 2 — code block + nested list
  `Good question. The canonical pattern is:

1. **Check for existing data** in local state first
2. **Return early** if you have it
3. **Fetch and cache** otherwise

\`\`\`typescript
function useNote(id: string) {
  const cached = useNoteCache(id);
  if (cached) return cached;
  return useQuery(['note', id], () => fetchNote(id));
}
\`\`\`

Avoid putting the fetch inside a plain \`useEffect\` without a stable
dependency — you'll get request storms. See [[react/effect-gotchas]].`,

  // 3 — headings + blockquote + ordered + task list
  `# Architecture recap

Jade has three layers:

1. **\`core/\`** — pure Go engine, no CGO, no HTTP. This is the product.
2. **\`cmd/jade/\`** — Wails desktop app that wraps \`core\`
3. **\`packages/jade-viewer\`** — this React component

## What's done

- [x] Engine with CRUD + search + backlinks + watch
- [x] Desktop binary that actually runs
- [x] Shareable React viewer (you are looking at it!)
- [ ] Advanceai integration
- [ ] Release pipeline with signed Windows MSI

> The hardest part wasn't the code — it was making sure the Go side
> and the JS side render markdown identically. See [[compat/fixtures]]
> for the test plan.

Related: [[jade/roadmap]], [[jade/release-notes]]`,

  // 4 — streaming-hostile content: code that builds up, then a closing fence
  `Here's a minimal example that streams correctly:

\`\`\`go
func Stream(ctx context.Context, w io.Writer) error {
    for chunk := range source {
        if _, err := fmt.Fprint(w, chunk); err != nil {
            return err
        }
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    }
    return nil
}
\`\`\`

Key points:
- Wrap the \`Flush()\` in a type assertion so tests can pass a plain
  \`bytes.Buffer\`
- Return errors early — never swallow
- Never log the chunks in a hot path

See also [[go/streaming-patterns]].`,

  // 5 — shorter, mostly prose with inline wikilinks
  `Short version: yes, **it works offline**. The component runs
entirely in the browser with no server round-trip, which is why it is
safe to drop into rc-web's PWA mode. When the network comes back you
can sync via [[sync/protocol]] — the viewer doesn't care, it just
re-renders whatever source you pass it.`,
];

// Pick responses round-robin so the same user sees variety.
let responseCursor = 0;
function nextResponse(): string {
  const r = CANNED_RESPONSES[responseCursor % CANNED_RESPONSES.length];
  responseCursor += 1;
  return r;
}

/**
 * Split a long string into small chunks that simulate LLM token arrival.
 *
 * We deliberately pick chunks that are usually 1-6 characters so the
 * component has to handle many intermediate states — including states
 * where a markdown token is half-formed (e.g. one "*" before its pair).
 */
function streamChunks(text: string): string[] {
  const chunks: string[] = [];
  let i = 0;
  while (i < text.length) {
    // Jitter chunk size between 1 and 6 characters.
    const size = Math.max(1, Math.floor(Math.random() * 6));
    chunks.push(text.slice(i, i + size));
    i += size;
  }
  return chunks;
}

// A fake "resolver" — marks a handful of wikilinks as resolved, the
// rest as broken. Same shape as what advanceai would wire up.
const RESOLVED = new Set([
  'frameworks/desktop-wrappers',
  'architecture/runtime-choice',
  'react/effect-gotchas',
  'jade/roadmap',
  'go/streaming-patterns',
]);

async function fakeResolve(target: string): Promise<string | null> {
  await new Promise((r) => setTimeout(r, 80));
  return RESOLVED.has(target) ? `notes/${target}.md` : null;
}

function makeId(): string {
  return `msg_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

export function ChatMode() {
  const [messages, setMessages] = useState<Message[]>([
    {
      id: makeId(),
      role: 'assistant',
      body: `Hi! Type a question and I'll stream back a canned markdown
response so you can see how \`MarkdownView\` handles partial markdown
mid-stream. Try asking about **Wails**, **hooks**, or anything really —
all responses come from a fixed pool.`,
    },
  ]);
  const [input, setInput] = useState('');
  const [isStreaming, setIsStreaming] = useState(false);

  const scrollRef = useRef<HTMLDivElement>(null);
  const streamCancelRef = useRef<{ cancelled: boolean } | null>(null);

  // Auto-scroll to bottom whenever messages change.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  const handleSend = useCallback(async () => {
    const text = input.trim();
    if (!text || isStreaming) return;

    // 1. Append the user message.
    const userMsg: Message = { id: makeId(), role: 'user', body: text };
    setMessages((prev) => [...prev, userMsg]);
    setInput('');

    // 2. Append an empty assistant message and start streaming into it.
    const assistantId = makeId();
    setMessages((prev) => [
      ...prev,
      { id: assistantId, role: 'assistant', body: '', streaming: true },
    ]);
    setIsStreaming(true);

    const cancelToken = { cancelled: false };
    streamCancelRef.current = cancelToken;

    const full = nextResponse();
    const chunks = streamChunks(full);

    for (const chunk of chunks) {
      if (cancelToken.cancelled) break;
      await new Promise((r) => setTimeout(r, 15 + Math.random() * 25));
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantId ? { ...m, body: m.body + chunk } : m,
        ),
      );
    }

    // Mark the assistant message as done streaming.
    setMessages((prev) =>
      prev.map((m) => (m.id === assistantId ? { ...m, streaming: false } : m)),
    );
    setIsStreaming(false);
    streamCancelRef.current = null;
  }, [input, isStreaming]);

  const handleStop = useCallback(() => {
    const token = streamCancelRef.current;
    if (token) token.cancelled = true;
  }, []);

  const handleClear = useCallback(() => {
    if (isStreaming) handleStop();
    setMessages([]);
  }, [isStreaming, handleStop]);

  const handleWikilinkClick = useCallback((target: string) => {
    // In production this would navigate. Here we just alert.
    alert(`Wikilink clicked: ${target}`);
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        void handleSend();
      }
    },
    [handleSend],
  );

  const suggestions = useMemo(
    () => [
      'Compare Wails, Tauri, and Electron',
      'How should I handle data fetching in React?',
      'Recap the Jade architecture',
      'Show me a streaming Go example',
      'Does this work offline?',
    ],
    [],
  );

  return (
    <div className="chat-root">
      <div className="chat-scroll" ref={scrollRef}>
        <div className="chat-messages">
          {messages.map((msg) => (
            <div key={msg.id} className={`chat-row chat-row-${msg.role}`}>
              <div className="chat-avatar">
                {msg.role === 'user' ? 'YOU' : 'AI'}
              </div>
              <div className={`chat-bubble chat-bubble-${msg.role}`}>
                {msg.role === 'user' ? (
                  <div className="chat-user-body">{msg.body}</div>
                ) : (
                  <>
                    <MarkdownView
                      source={msg.body || '_thinking…_'}
                      onWikilinkClick={handleWikilinkClick}
                      resolveWikilink={fakeResolve}
                      className="chat-md"
                    />
                    {msg.streaming && <span className="chat-cursor" aria-hidden="true" />}
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {messages.length <= 1 && (
        <div className="chat-suggestions">
          {suggestions.map((s) => (
            <button
              key={s}
              className="chat-suggestion"
              onClick={() => setInput(s)}
              disabled={isStreaming}
            >
              {s}
            </button>
          ))}
        </div>
      )}

      <div className="chat-composer">
        <textarea
          className="chat-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
          rows={2}
          disabled={isStreaming}
        />
        <div className="chat-composer-actions">
          {isStreaming ? (
            <button className="chat-btn chat-btn-stop" onClick={handleStop}>
              Stop
            </button>
          ) : (
            <button
              className="chat-btn chat-btn-send"
              onClick={handleSend}
              disabled={!input.trim()}
            >
              Send
            </button>
          )}
          <button className="chat-btn chat-btn-clear" onClick={handleClear}>
            Clear
          </button>
        </div>
      </div>
    </div>
  );
}
