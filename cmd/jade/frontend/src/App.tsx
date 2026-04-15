import { useState, useCallback, useRef, useEffect } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { markdown } from '@codemirror/lang-markdown';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { MarkdownView } from '@enoramlabs/jade-viewer';
import '@enoramlabs/jade-viewer/styles.css';
import {
    OpenVault, ListNotes, ReadNote,
    CreateNote, UpdateNote, DeleteNote, MoveNote,
    Backlinks, ResolveWikilink, Search,
    GetStartupState, CreateVault, OpenInNewWindow,
} from '../wailsjs/go/main/App';
import type { core, main } from '../wailsjs/go/models';
type NoteMeta = core.NoteMeta;
type Note = core.Note;
type StartupState = main.StartupState;
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

// ---- Welcome screen --------------------------------------------------------

interface WelcomeScreenProps {
    startupError: string;
    recentVaults: string[];
    onVaultOpened: (path: string) => void;
    onError: (msg: string) => void;
}

function WelcomeScreen({ startupError, recentVaults, onVaultOpened, onError }: WelcomeScreenProps) {
    // null = idle; non-null = in progress with a status message
    const [busy, setBusy] = useState<string | null>(null);

    const runWithBusy = useCallback(async (
        label: string,
        fn: () => Promise<{ path: string }>,
    ) => {
        setBusy(label);
        try {
            const info = await fn();
            onVaultOpened(info.path);
        } catch (e: unknown) {
            onError(String(e));
        } finally {
            setBusy(null);
        }
    }, [onVaultOpened, onError]);

    const handleOpenVault = useCallback(() => {
        // Empty string triggers native dir-picker in the Go layer.
        return runWithBusy('Opening vault — scanning notes and building index…', () => OpenVault(''));
    }, [runWithBusy]);

    const handleCreateVault = useCallback(() => {
        return runWithBusy('Creating vault…', () => CreateVault(''));
    }, [runWithBusy]);

    const handleOpenRecent = useCallback((path: string) => {
        return runWithBusy(`Opening ${path} — scanning notes and building index…`, () => OpenVault(path));
    }, [runWithBusy]);

    const loading = busy !== null;

    return (
        <div id="welcome-screen">
            <div id="welcome-card">
                <h1 id="welcome-title">Jade</h1>
                <p id="welcome-subtitle">Local-first Markdown knowledge base</p>

                {startupError && (
                    <div id="welcome-error">
                        <span>{startupError}</span>
                    </div>
                )}

                <div id="welcome-actions">
                    <button
                        className="welcome-btn primary"
                        onClick={handleOpenVault}
                        disabled={loading}
                    >
                        {loading ? 'Working…' : 'Open Vault'}
                    </button>
                    <button
                        className="welcome-btn secondary"
                        onClick={handleCreateVault}
                        disabled={loading}
                    >
                        {loading ? 'Working…' : 'Create Vault'}
                    </button>
                </div>

                {busy && (
                    <div id="welcome-busy">
                        <div className="spinner" aria-hidden="true" />
                        <span>{busy}</span>
                    </div>
                )}

                {recentVaults.length > 0 && (
                    <div id="welcome-recents">
                        <h2 id="welcome-recents-title">Recent Vaults</h2>
                        <ul id="welcome-recents-list">
                            {recentVaults.map(vault => (
                                <li key={vault}>
                                    <button
                                        className="recent-vault-item"
                                        onClick={() => handleOpenRecent(vault)}
                                        disabled={loading}
                                        title={vault}
                                    >
                                        {vault}
                                    </button>
                                </li>
                            ))}
                        </ul>
                    </div>
                )}
            </div>
        </div>
    );
}

// ---- Main editor view ------------------------------------------------------

function App() {
    const [startupLoaded, setStartupLoaded] = useState(false);
    const [vaultPath, setVaultPath] = useState('');
    const [startupError, setStartupError] = useState('');
    const [recentVaults, setRecentVaults] = useState<string[]>([]);

    const [notes, setNotes] = useState<NoteMeta[]>([]);
    const [selectedNote, setSelectedNote] = useState<Note | null>(null);
    const [editBody, setEditBody] = useState('');
    const [backlinks, setBacklinks] = useState<NoteMeta[]>([]);
    const [dirty, setDirty] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [newNoteName, setNewNoteName] = useState('');
    const [renameTarget, setRenameTarget] = useState<string | null>(null);
    const [renameTo, setRenameTo] = useState('');
    const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
    const [searchQuery, setSearchQuery] = useState('');
    const [searchResults, setSearchResults] = useState<NoteMeta[] | null>(null);
    const searchDebounce = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [conflict, setConflict] = useState<AppConflictError | null>(null);
    const pendingBody = useRef('');
    const currentEtag = useRef('');

    // Load startup state on mount to determine welcome vs. editor view.
    useEffect(() => {
        GetStartupState()
            .then((state: StartupState) => {
                setStartupError(state.vaultError ?? '');
                setRecentVaults(state.recent ?? []);
                if (state.vaultPath) {
                    setVaultPath(state.vaultPath);
                    // Populate the note tree.
                    return ListNotes('').then(list => setNotes(list ?? []));
                }
            })
            .catch(() => { /* ignore — welcome screen will be shown */ })
            .finally(() => setStartupLoaded(true));
    }, []);

    // Flush search debounce on unmount.
    useEffect(() => {
        return () => {
            if (searchDebounce.current) clearTimeout(searchDebounce.current);
        };
    }, []);

    const refreshTree = useCallback(async () => {
        const list = await ListNotes('');
        setNotes(list ?? []);
    }, []);

    // Subscribe to vault.changed events emitted by the Go Watch bridge.
    useEffect(() => {
        const off = EventsOn('vault.changed', (evt: { type: string; id: string }) => {
            refreshTree().catch(() => {});

            setSelectedNote(prev => {
                if (prev && prev.ID === evt.id && (evt.type === 'update' || evt.type === 'create')) {
                    ReadNote(evt.id)
                        .then(note => {
                            setSelectedNote(note);
                            setEditBody(note.Body);
                            setDirty(false);
                            currentEtag.current = note.ETag;
                        })
                        .catch(() => {});
                }
                return prev;
            });
        });
        return off;
    }, [refreshTree]);

    // Called by WelcomeScreen after a vault is opened.
    const handleVaultOpened = useCallback(async (path: string) => {
        setVaultPath(path);
        setStartupError('');
        setSelectedNote(null);
        setEditBody('');
        setBacklinks([]);
        setDirty(false);
        setSearchQuery('');
        setSearchResults(null);
        try {
            const list = await ListNotes('');
            setNotes(list ?? []);
        } catch { /* ignore */ }
    }, []);

    const openVault = useCallback(async () => {
        const path = vaultPath.trim();
        if (!path) { setError('Enter a vault path to open.'); return; }
        setLoading(true); setError(null);
        try {
            await OpenVault(path);
            await refreshTree();
            setSelectedNote(null);
            setEditBody('');
            setBacklinks([]);
            setDirty(false);
            setSearchQuery('');
            setSearchResults(null);
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
            const updated = await UpdateNote(selectedNote.ID, editBody, {}, currentEtag.current);
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

    const resolveKeepMine = useCallback(async () => {
        if (!selectedNote || !conflict) return;
        setLoading(true); setError(null);
        try {
            const updated = await UpdateNote(
                selectedNote.ID, pendingBody.current, {}, conflict.currentEtag,
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

    const resolveKeepTheirs = useCallback(async () => {
        if (!selectedNote || !conflict) return;
        if (!confirm('Discard your edits and reload from disk?')) return;
        setEditBody(conflict.currentContent);
        currentEtag.current = conflict.currentEtag;
        setDirty(false);
        setConflict(null);
        const fresh = await ReadNote(selectedNote.ID).catch(() => null);
        if (fresh) {
            setSelectedNote(fresh);
            setEditBody(fresh.Body);
            currentEtag.current = fresh.ETag;
        }
    }, [selectedNote, conflict]);

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
            const note = await CreateNote(id, '', {});
            await refreshTree();
            setSelectedNote(note);
            setEditBody(note.Body);
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
    }, []);

    const handleSearchChange = useCallback((val: string) => {
        setSearchQuery(val);
        if (searchDebounce.current) clearTimeout(searchDebounce.current);
        if (!val.trim()) {
            setSearchResults(null);
            return;
        }
        searchDebounce.current = setTimeout(async () => {
            try {
                const results = await Search(val.trim());
                setSearchResults(results ?? []);
            } catch {
                setSearchResults([]);
            }
        }, 300);
    }, []);

    // Async resolver fed to MarkdownView's resolveWikilink prop. Called by
    // the component once per unique target to decorate links with
    // resolved/broken CSS classes.
    const resolveWikilinkForView = useCallback(async (target: string): Promise<string | null> => {
        try {
            const id = await ResolveWikilink(target);
            return id || null;
        } catch {
            return null;
        }
    }, []);

    // Fired when the user clicks a wikilink in the MarkdownView preview.
    // Resolves the target and, if it maps to a known note, opens it.
    const handleWikilinkClick = useCallback(async (target: string) => {
        try {
            const id = await ResolveWikilink(target);
            if (!id) return;
            const meta = notes.find(n => n.ID === id);
            if (meta) selectNote(meta);
        } catch {
            /* broken link — do nothing */
        }
    }, [notes, selectNote]);

    const handleNewWindow = useCallback(() => {
        OpenInNewWindow('').catch(() => {});
    }, []);

    // Show a blank shell until startup state is loaded.
    if (!startupLoaded) {
        return <div id="app-shell" />;
    }

    // Show welcome screen when no vault is open.
    if (!vaultPath) {
        return (
            <WelcomeScreen
                startupError={startupError}
                recentVaults={recentVaults}
                onVaultOpened={handleVaultOpened}
                onError={msg => setStartupError(msg)}
            />
        );
    }

    return (
        <div id="app-shell">
            {/* Toolbar */}
            <div id="toolbar">
                <span id="vault-path-label" title={vaultPath}>{vaultPath}</span>
                <button id="change-vault-btn" onClick={() => setVaultPath('')} disabled={loading} title="Switch vault">
                    Switch Vault
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
                <button id="new-window-btn" onClick={handleNewWindow} title="Open a new Jade window">
                    New Window
                </button>
                {error && <span id="error-msg">{error}</span>}
            </div>

            <div id="main-content">
                {/* Sidebar: search bar + note tree (or search results) */}
                <aside id="note-tree">
                    {/* Search bar */}
                    <div id="search-bar">
                        <input
                            id="search-input"
                            type="text"
                            placeholder="Search notes…"
                            value={searchQuery}
                            onChange={e => handleSearchChange(e.target.value)}
                            disabled={notes.length === 0 && !searchQuery}
                        />
                        {searchQuery && (
                            <button
                                id="search-clear-btn"
                                onClick={() => handleSearchChange('')}
                                title="Clear search"
                            >✕</button>
                        )}
                    </div>

                    {/* Search results overlay */}
                    {searchResults !== null ? (
                        <div id="search-results">
                            {searchResults.length === 0 ? (
                                <p className="tree-empty">No results for &ldquo;{searchQuery}&rdquo;</p>
                            ) : (
                                <ul>
                                    {searchResults.map(r => (
                                        <li
                                            key={r.ID}
                                            className={selectedNote?.ID === r.ID ? 'active' : ''}
                                            title={r.ID}
                                            onClick={() => selectNote(r)}
                                        >
                                            <span className="note-title">{r.Title || r.ID}</span>
                                            {r.Snippet && (
                                                <span
                                                    className="search-snippet"
                                                    dangerouslySetInnerHTML={{ __html: r.Snippet }}
                                                />
                                            )}
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </div>
                    ) : (
                        <>
                            {/* Normal note tree */}
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
                        </>
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
                                    extensions={[markdown(), EditorView.lineWrapping]}
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

                        {/* Live preview pane — powered by @enoramlabs/jade-viewer */}
                        <div id="preview-pane" className="markdown-preview">
                            {selectedNote ? (
                                <MarkdownView
                                    source={editBody}
                                    onWikilinkClick={handleWikilinkClick}
                                    resolveWikilink={resolveWikilinkForView}
                                />
                            ) : (
                                <p className="preview-placeholder">Preview will appear here.</p>
                            )}
                        </div>
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
