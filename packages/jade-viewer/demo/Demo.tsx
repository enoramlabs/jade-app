import { useState } from 'react';
import { SandboxMode } from './SandboxMode';
import { ChatMode } from './ChatMode';
import '../src/styles.css';
import './demo.css';

type Mode = 'sandbox' | 'chat';

/**
 * The top-level demo shell. Hosts a mode toggle in the header and
 * swaps between the interactive sandbox (editable markdown +
 * split-pane preview) and the streaming chat simulation (fake AI
 * conversation with chunk-by-chunk markdown arrival).
 */
export function Demo() {
  const [mode, setMode] = useState<Mode>('chat');

  return (
    <div className="demo-shell">
      <header className="demo-header">
        <div className="demo-title">
          <strong>@enoramlabs/jade-viewer</strong>
          <span className="demo-tag">interactive demo</span>
        </div>
        <nav className="demo-modes">
          <button
            className={`demo-mode-btn${mode === 'chat' ? ' active' : ''}`}
            onClick={() => setMode('chat')}
          >
            Chat
          </button>
          <button
            className={`demo-mode-btn${mode === 'sandbox' ? ' active' : ''}`}
            onClick={() => setMode('sandbox')}
          >
            Sandbox
          </button>
        </nav>
      </header>

      <div className="demo-body">
        {mode === 'sandbox' ? <SandboxMode /> : <ChatMode />}
      </div>
    </div>
  );
}
