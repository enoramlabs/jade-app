import { useState, useCallback } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { oneDark } from '@codemirror/theme-one-dark';
import { OpenVault, ListNotes, ReadNote } from '../wailsjs/go/main/App';
import type { NoteMeta, Note } from '../wailsjs/go/main/App';
import './App.css';

function App() {
    const [vaultPath, setVaultPath] = useState('');
    const [notes, setNotes] = useState<NoteMeta[]>([]);
    const [selectedNote, setSelectedNote] = useState<Note | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);

    const openVault = useCallback(async () => {
        const path = vaultPath.trim();
        if (!path) {
            setError('Enter a vault path to open.');
            return;
        }
        setLoading(true);
        setError(null);
        try {
            await OpenVault(path);
            const list = await ListNotes('');
            setNotes(list ?? []);
            setSelectedNote(null);
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [vaultPath]);

    const selectNote = useCallback(async (meta: NoteMeta) => {
        setLoading(true);
        setError(null);
        try {
            const note = await ReadNote(meta.ID);
            setSelectedNote(note);
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, []);

    return (
        <div id="app-shell">
            <div id="toolbar">
                <input
                    id="vault-path-input"
                    type="text"
                    placeholder="Vault directory path…"
                    value={vaultPath}
                    onChange={e => setVaultPath(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && openVault()}
                />
                <button id="open-vault-btn" onClick={openVault} disabled={loading}>
                    {loading ? 'Opening…' : 'Open Vault'}
                </button>
                {error && <span id="error-msg">{error}</span>}
            </div>

            <div id="main-content">
                <aside id="note-tree">
                    {notes.length === 0 ? (
                        <p className="tree-empty">No notes found.</p>
                    ) : (
                        <ul>
                            {notes.map(n => (
                                <li
                                    key={n.ID}
                                    className={selectedNote?.ID === n.ID ? 'active' : ''}
                                    onClick={() => selectNote(n)}
                                    title={n.ID}
                                >
                                    {n.Title || n.ID}
                                </li>
                            ))}
                        </ul>
                    )}
                </aside>

                <main id="editor-pane">
                    {selectedNote ? (
                        <CodeMirror
                            value={selectedNote.Body}
                            extensions={[markdown()]}
                            theme={oneDark}
                            readOnly
                            height="100%"
                            style={{ height: '100%', fontSize: '14px' }}
                        />
                    ) : (
                        <div id="editor-placeholder">
                            <p>Select a note from the tree to view its source.</p>
                        </div>
                    )}
                </main>
            </div>
        </div>
    );
}

export default App;
