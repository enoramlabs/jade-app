import { useState, useCallback, useRef, useEffect } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { oneDark } from '@codemirror/theme-one-dark';
import {
    OpenVault, ListNotes, ReadNote,
    CreateNote, UpdateNote, DeleteNote, MoveNote,
    RenderMarkdown,
} from '../wailsjs/go/main/App';
import type { NoteMeta, Note } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import './App.css';

function App() {
    const [vaultPath, setVaultPath] = useState('');
    const [notes, setNotes] = useState<NoteMeta[]>([]);
    const [selectedNote, setSelectedNote] = useState<Note | null>(null);
    const [editBody, setEditBody] = useState('');
    const [previewHtml, setPreviewHtml] = useState('');
    const [dirty, setDirty] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [newNoteName, setNewNoteName] = useState('');
    const [renameTarget, setRenameTarget] = useState<string | null>(null);
    const [renameTo, setRenameTo] = useState('');
    const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
    const currentEtag = useRef('');
    const previewDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);

    // Debounced preview update: re-renders the preview 100ms after source changes.
    const updatePreview = useCallback((source: string) => {
        if (previewDebounce.current) clearTimeout(previewDebounce.current);
        previewDebounce.current = setTimeout(async () => {
            const html = await RenderMarkdown(source);
            setPreviewHtml(html);
        }, 100);
    }, []);

    // Flush preview debounce on unmount.
    useEffect(() => {
        return () => {
            if (previewDebounce.current) clearTimeout(previewDebounce.current);
        };
    }, []);

    const refreshTree = useCallback(async () => {
        const list = await ListNotes('');
        setNotes(list ?? []);
    }, []);

    // Subscribe to vault.changed events emitted by the Go Watch bridge.
    // When a note that is currently open changes externally, reload it.
    // Always refresh the tree so new/deleted notes appear.
    useEffect(() => {
        const off = EventsOn('vault.changed', (evt: { type: string; id: string }) => {
            // Refresh tree on any structural change.
            refreshTree().catch(() => {});

            // If the currently open note changed externally, reload its content.
            setSelectedNote(prev => {
                if (prev && prev.ID === evt.id && (evt.type === 'update' || evt.type === 'create')) {
                    ReadNote(evt.id)
                        .then(note => {
                            setSelectedNote(note);
                            setEditBody(note.Body);
                            setDirty(false);
                            currentEtag.current = note.ETag;
                            return RenderMarkdown(note.Body);
                        })
                        .then(html => setPreviewHtml(html))
                        .catch(() => {});
                }
                return prev;
            });
        });
        return off;
    }, [refreshTree]);

    const openVault = useCallback(async () => {
        const path = vaultPath.trim();
        if (!path) { setError('Enter a vault path to open.'); return; }
        setLoading(true); setError(null);
        try {
            await OpenVault(path);
            await refreshTree();
            setSelectedNote(null);
            setEditBody('');
            setPreviewHtml('');
            setDirty(false);
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [vaultPath, refreshTree]);

    const selectNote = useCallback(async (meta: NoteMeta) => {
        if (dirty && selectedNote && !confirm('Unsaved changes — discard?')) return;
        setLoading(true); setError(null);
        try {
            const note = await ReadNote(meta.ID);
            setSelectedNote(note);
            setEditBody(note.Body);
            setDirty(false);
            currentEtag.current = note.ETag;
            const html = await RenderMarkdown(note.Body);
            setPreviewHtml(html);
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [dirty, selectedNote]);

    const saveNote = useCallback(async () => {
        if (!selectedNote) return;
        setLoading(true); setError(null);
        try {
            const updated = await UpdateNote(selectedNote.ID, editBody, null, currentEtag.current);
            setSelectedNote(updated);
            setDirty(false);
            currentEtag.current = updated.ETag;
            await refreshTree();
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [selectedNote, editBody, refreshTree]);

    const createNote = useCallback(async () => {
        const name = newNoteName.trim();
        if (!name) { setError('Note name cannot be empty.'); return; }
        const id = name.endsWith('.md') ? name : name + '.md';
        setLoading(true); setError(null);
        try {
            const note = await CreateNote(id, '', null);
            await refreshTree();
            setSelectedNote(note);
            setEditBody(note.Body);
            setPreviewHtml('');
            setDirty(false);
            currentEtag.current = note.ETag;
            setNewNoteName('');
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [newNoteName, refreshTree]);

    const confirmDelete = useCallback(async () => {
        if (!deleteTarget) return;
        setLoading(true); setError(null);
        try {
            await DeleteNote(deleteTarget);
            if (selectedNote?.ID === deleteTarget) {
                setSelectedNote(null);
                setEditBody('');
                setPreviewHtml('');
                setDirty(false);
            }
            await refreshTree();
            setDeleteTarget(null);
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [deleteTarget, selectedNote, refreshTree]);

    const confirmRename = useCallback(async () => {
        if (!renameTarget) return;
        const toName = renameTo.trim();
        if (!toName) { setError('New name cannot be empty.'); return; }
        const toID = toName.endsWith('.md') ? toName : toName + '.md';
        setLoading(true); setError(null);
        try {
            await MoveNote(renameTarget, toID);
            if (selectedNote?.ID === renameTarget) {
                const updated = await ReadNote(toID);
                setSelectedNote(updated);
                setEditBody(updated.Body);
                currentEtag.current = updated.ETag;
                setDirty(false);
            }
            await refreshTree();
            setRenameTarget(null);
            setRenameTo('');
        } catch (e: unknown) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [renameTarget, renameTo, selectedNote, refreshTree]);

    const handleEditorChange = useCallback((val: string) => {
        setEditBody(val);
        setDirty(true);
        updatePreview(val);
    }, [updatePreview]);

    return (
        <div id="app-shell">
            {/* Toolbar */}
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
                    {loading ? 'Working…' : 'Open Vault'}
                </button>
                {selectedNote && (
                    <button
                        id="save-btn"
                        onClick={saveNote}
                        disabled={loading || !dirty}
                        title="Save current note (Ctrl+S)"
                    >
                        {dirty ? 'Save*' : 'Saved'}
                    </button>
                )}
                {error && <span id="error-msg">{error}</span>}
            </div>

            <div id="main-content">
                {/* Note tree */}
                <aside id="note-tree">
                    <div id="new-note-bar">
                        <input
                            type="text"
                            placeholder="new-note.md"
                            value={newNoteName}
                            onChange={e => setNewNoteName(e.target.value)}
                            onKeyDown={e => e.key === 'Enter' && createNote()}
                        />
                        <button onClick={createNote} disabled={loading}>+</button>
                    </div>
                    {notes.length === 0 ? (
                        <p className="tree-empty">No notes found.</p>
                    ) : (
                        <ul>
                            {notes.map(n => (
                                <li
                                    key={n.ID}
                                    className={selectedNote?.ID === n.ID ? 'active' : ''}
                                    title={n.ID}
                                >
                                    <span className="note-title" onClick={() => selectNote(n)}>
                                        {n.Title || n.ID}
                                    </span>
                                    <span className="note-actions">
                                        <button
                                            className="icon-btn"
                                            title="Rename"
                                            onClick={() => { setRenameTarget(n.ID); setRenameTo(n.Title || ''); }}
                                        >✎</button>
                                        <button
                                            className="icon-btn danger"
                                            title="Delete"
                                            onClick={() => setDeleteTarget(n.ID)}
                                        >✕</button>
                                    </span>
                                </li>
                            ))}
                        </ul>
                    )}
                </aside>

                {/* Split editor + preview */}
                <div id="editor-split">
                    {/* Source editor pane */}
                    <div id="editor-pane">
                        {selectedNote ? (
                            <CodeMirror
                                value={editBody}
                                extensions={[markdown()]}
                                theme={oneDark}
                                height="100%"
                                style={{ height: '100%', fontSize: '14px' }}
                                onChange={handleEditorChange}
                            />
                        ) : (
                            <div id="editor-placeholder">
                                <p>Select a note from the tree to edit it.</p>
                            </div>
                        )}
                    </div>

                    {/* Live preview pane */}
                    <div
                        id="preview-pane"
                        className="markdown-preview"
                        dangerouslySetInnerHTML={{ __html: previewHtml || (selectedNote ? '' : '<p class="preview-placeholder">Preview will appear here.</p>') }}
                    />
                </div>
            </div>

            {/* Delete confirmation dialog */}
            {deleteTarget && (
                <div id="dialog-overlay">
                    <div id="dialog">
                        <p>Delete <strong>{deleteTarget}</strong>? This cannot be undone.</p>
                        <div id="dialog-actions">
                            <button className="danger" onClick={confirmDelete}>Delete</button>
                            <button onClick={() => setDeleteTarget(null)}>Cancel</button>
                        </div>
                    </div>
                </div>
            )}

            {/* Rename dialog */}
            {renameTarget && (
                <div id="dialog-overlay">
                    <div id="dialog">
                        <p>Rename <strong>{renameTarget}</strong> to:</p>
                        <input
                            type="text"
                            value={renameTo}
                            onChange={e => setRenameTo(e.target.value)}
                            onKeyDown={e => e.key === 'Enter' && confirmRename()}
                            autoFocus
                        />
                        <div id="dialog-actions">
                            <button onClick={confirmRename}>Rename</button>
                            <button onClick={() => { setRenameTarget(null); setRenameTo(''); }}>Cancel</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default App;
