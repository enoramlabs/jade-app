import { useState, useCallback, useRef, useEffect } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { oneDark } from '@codemirror/theme-one-dark';
import {
    OpenVault, ListNotes, ReadNote,
    CreateNote, UpdateNote, DeleteNote, MoveNote,
    RenderMarkdown, Backlinks, ResolveWikilink,
} from '../wailsjs/go/main/App';
import type { NoteMeta, Note } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import './App.css';

// Shape of the structured error JSON returned by the Wails bridge on conflict.
interface AppConflictError {
    code: 'CONFLICT';
    message: string;
    currentContent: string;
    currentEtag: string;
}

function parseConflictError(raw: unknown): AppConflictError | null {
    try {
        const obj = JSON.parse(String(raw));
        if (obj && obj.code === 'CONFLICT') return obj as AppConflictError;
    } catch { /* not JSON */ }
    return null;
}

function App() {
    const [vaultPath, setVaultPath] = useState('');
    const [notes, setNotes] = useState<NoteMeta[]>([]);
    const [selectedNote, setSelectedNote] = useState<Note | null>(null);
    const [editBody, setEditBody] = useState('');
    const [previewHtml, setPreviewHtml] = useState('');
    const [backlinks, setBacklinks] = useState<NoteMeta[]>([]);
    const [dirty, setDirty] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [newNoteName, setNewNoteName] = useState('');
    const [renameTarget, setRenameTarget] = useState<string | null>(null);
    const [renameTo, setRenameTo] = useState('');
    const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
    const [conflict, setConflict] = useState<AppConflictError | null>(null);
    const pendingBody = useRef(''); // body the user was trying to save when conflict occurred
    const currentEtag = useRef('');
    const previewDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);
    const previewRef = useRef<HTMLDivElement | null>(null);

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
            setBacklinks([]);
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
            // Fetch backlinks for this note.
            const bl = await Backlinks(note.ID).catch(() => [] as NoteMeta[]);
            setBacklinks(bl ?? []);
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
            const conflictErr = parseConflictError(e);
            if (conflictErr) {
                pendingBody.current = editBody;
                setConflict(conflictErr);
            } else {
                setError(String(e));
            }
        } finally {
            setLoading(false);
        }
    }, [selectedNote, editBody, refreshTree]);

    // Keep Mine: force-save using a fresh ETag obtained from the conflict info.
    const resolveKeepMine = useCallback(async () => {
        if (!selectedNote || !conflict) return;
        setLoading(true); setError(null);
        try {
            // Use the fresh ETag from the conflict (current on-disk state) to overwrite.
            const updated = await UpdateNote(
                selectedNote.ID, pendingBody.current, null, conflict.currentEtag,
            );
            setSelectedNote(updated);
            setEditBody(updated.Body);
            setDirty(false);
            currentEtag.current = updated.ETag;
            setConflict(null);
            await refreshTree();
        } catch (e: unknown) {
            setError(String(e));
            setConflict(null);
        } finally {
            setLoading(false);
        }
    }, [selectedNote, conflict, refreshTree]);

    // Keep Theirs: reload the on-disk version, discard local edits.
    const resolveKeepTheirs = useCallback(async () => {
        if (!selectedNote || !conflict) return;
        if (!confirm('Discard your edits and reload from disk?')) return;
        setEditBody(conflict.currentContent);
        currentEtag.current = conflict.currentEtag;
        setDirty(false);
        setConflict(null);
        // Reload the full note so selectedNote ETag is fresh.
        const fresh = await ReadNote(selectedNote.ID).catch(() => null);
        if (fresh) {
            setSelectedNote(fresh);
            setEditBody(fresh.Body);
            currentEtag.current = fresh.ETag;
            const html = await RenderMarkdown(fresh.Body).catch(() => '');
            setPreviewHtml(html);
        }
    }, [selectedNote, conflict]);

    // Cancel: dismiss dialog, stay in dirty state with user's pending edits.
    const resolveCancel = useCallback(() => {
        setEditBody(pendingBody.current);
        setDirty(true);
        setConflict(null);
    }, []);

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
            setBacklinks([]);
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
                setBacklinks([]);
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

    // After preview HTML updates, mark unresolved wikilinks and add embed placeholders.
    useEffect(() => {
        const container = previewRef.current;
        if (!container) return;

        // Mark wikilinks as resolved or broken based on current notes list.
        const noteIds = notes.map(n => n.ID.toLowerCase());
        container.querySelectorAll<HTMLElement>('[data-wikilink]').forEach(el => {
            const target = el.getAttribute('data-wikilink') ?? '';
            const targetLower = target.toLowerCase();
            const resolved = noteIds.some(id =>
                id === targetLower ||
                id === targetLower + '.md' ||
                id.split('/').pop() === targetLower + '.md'
            );
            if (resolved) {
                el.classList.remove('wikilink-broken');
                el.classList.add('wikilink-resolved');
            } else {
                el.classList.remove('wikilink-resolved');
                el.classList.add('wikilink-broken');
            }
        });

        // Replace embed placeholders with a visible label showing the target.
        container.querySelectorAll<HTMLElement>('[data-embed]').forEach(el => {
            if (!el.dataset.embedRendered) {
                const target = el.getAttribute('data-embed') ?? '';
                el.dataset.embedRendered = '1';
                el.textContent = `📄 ${target}`;
                el.classList.add('wikilink-embed-placeholder');
            }
        });
    }, [previewHtml, notes]);

    // Click delegation on the preview pane: intercept wikilink clicks.
    const handlePreviewClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
        const target = (e.target as HTMLElement).closest('[data-wikilink]') as HTMLElement | null;
        if (!target) return;
        e.preventDefault();
        const wikilinkTarget = target.getAttribute('data-wikilink');
        if (!wikilinkTarget) return;
        ResolveWikilink(wikilinkTarget)
            .then(id => {
                if (!id) return; // unresolved — do nothing (already styled broken)
                const meta = notes.find(n => n.ID === id);
                if (meta) selectNote(meta);
            })
            .catch(() => {});
    }, [notes, selectNote]);

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

                {/* Editor area: split pane + backlinks panel */}
                <div id="editor-area">
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
                            ref={previewRef}
                            onClick={handlePreviewClick}
                            dangerouslySetInnerHTML={{ __html: previewHtml || (selectedNote ? '' : '<p class="preview-placeholder">Preview will appear here.</p>') }}
                        />
                    </div>

                    {/* Backlinks panel */}
                    {selectedNote && (
                        <div id="backlinks-panel">
                            <div id="backlinks-header">Backlinks</div>
                            {backlinks.length === 0 ? (
                                <p className="backlinks-empty">No backlinks</p>
                            ) : (
                                <ul id="backlinks-list">
                                    {backlinks.map(bl => (
                                        <li key={bl.ID}>
                                            <button
                                                className="backlink-item"
                                                onClick={() => selectNote(bl)}
                                                title={bl.ID}
                                            >
                                                {bl.Title || bl.ID}
                                            </button>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>
                    )}
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

            {/* Conflict resolution dialog */}
            {conflict && (
                <div id="dialog-overlay">
                    <div id="conflict-dialog">
                        <h3>Edit conflict detected</h3>
                        <p>This note was modified externally while you were editing it.</p>
                        <div id="conflict-versions">
                            <div className="conflict-version">
                                <h4>Your version</h4>
                                <pre className="conflict-preview">{pendingBody.current}</pre>
                            </div>
                            <div className="conflict-version">
                                <h4>Their version (on disk)</h4>
                                <pre className="conflict-preview">{conflict.currentContent}</pre>
                            </div>
                        </div>
                        <div id="dialog-actions">
                            <button onClick={resolveKeepMine} disabled={loading}>Keep mine</button>
                            <button onClick={resolveKeepTheirs} disabled={loading}>Keep theirs</button>
                            <button onClick={resolveCancel}>Cancel</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default App;
