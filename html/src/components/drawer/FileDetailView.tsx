import { h, Component } from 'preact';
import { FsEntry, getFileTag, formatBytes } from '../types';
import { t, type Lang } from '../i18n';

interface FileDetailViewProps {
    selectedFsEntry: FsEntry;
    favoriteFiles: string[];
    detailFullscreen: boolean;
    isEditingDetail: boolean;
    fileContent: string;
    editedContent: string;
    fileLoading: boolean;
    fileSaving: boolean;
    fileSaveMsg: string;
    isImagePreview: boolean;
    imageUrl: string;

    onBackToList: () => void;
    onToggleFavorite: (path: string) => void;
    onCopyContent: () => void;
    onDownloadFile: () => void;
    onRenameFile: () => void;
    onToggleFullscreen: () => void;
    onShareFile: () => void;
    onSaveFile: () => void;
    onToggleEditing: (isEditing: boolean) => void;
    onEditedContentChange: (content: string) => void;
    isStandalone?: boolean;
    onOpenPreview?: (path: string, name: string) => void;
    language: Lang;
}

export class FileDetailView extends Component<FileDetailViewProps> {
    private contentEl: HTMLDivElement | null = null;
    private editorEl: HTMLTextAreaElement | null = null;
    private savedScrollTop: number = 0;

    // ── Markdown worker plumbing ────────────────────────────────────────────
    // Parsing markdown with `marked()` blocks the main thread for hundreds of
    // ms on large files. We offload it to a dedicated worker and only render
    // the HTML for the most recent request (id-keyed) so stale responses from
    // fast file-switches can't overwrite a newer one.
    private _mdWorker: Worker | null = null;
    private _mdRequestId: number = 0;
    private _mdLatestHtml: string = '';
    private _mdLastRenderedPath: string = '';
    private _mdLastRenderedContent: string = '';

    private handleStartEditing = () => {
        const pos = this.contentEl ? this.contentEl.scrollTop : 0;
        this.savedScrollTop = pos;
        this.props.onToggleEditing(true);
    };

    private handleStopEditing = () => {
        const pos = this.editorEl ? this.editorEl.scrollTop : 0;
        this.savedScrollTop = pos;
        this.props.onToggleEditing(false);
    };

    private handleMarkdownClick = (e: MouseEvent) => {
        const target = e.target as HTMLElement;
        const link = target.closest('a');
        if (!link) return;

        const href = link.getAttribute('href');
        if (!href) return;

        // Case 1: Anchor link inside the same file (e.g. #heading-title)
        if (href.startsWith('#')) {
            e.preventDefault();
            const id = decodeURIComponent(href.slice(1));
            const escapedId = id.replace(/"/g, '\\"');
            // Find element inside the markdown container by id or name
            const targetEl = this.contentEl
                ? this.contentEl.querySelector(`[id="${escapedId}"]`) ||
                  this.contentEl.querySelector(`[name="${escapedId}"]`)
                : null;
            if (targetEl) {
                targetEl.scrollIntoView({ behavior: 'smooth' });
            }
            return;
        }

        // Case 2: External web links (http://, https://, //, etc.)
        const isExternal =
            /^(https?:)?\/\//i.test(href) ||
            href.startsWith('mailto:') ||
            href.startsWith('tel:') ||
            href.startsWith('javascript:');
        if (isExternal) {
            if (link.getAttribute('target') !== '_blank') {
                link.setAttribute('target', '_blank');
            }
            return;
        }

        // Case 3: Local file link
        e.preventDefault();

        // Handle potential query and hash parts in the relative link
        const [urlWithoutHash, hashPart] = href.split('#');
        const [pathPart] = urlWithoutHash.split('?');

        // Resolve absolute path relative to the current file's path
        const basePath = this.props.selectedFsEntry.path;

        const resolveRelativePath = (base: string, relative: string): string => {
            if (relative.startsWith('/')) {
                return relative;
            }
            const parts = base.split('/');
            parts.pop(); // Remove filename

            const relParts = relative.split('/');
            for (const part of relParts) {
                if (part === '.' || part === '') {
                    continue;
                } else if (part === '..') {
                    parts.pop();
                } else {
                    parts.push(part);
                }
            }
            return parts.join('/');
        };

        const targetPath = resolveRelativePath(basePath, pathPart);
        if (IS_DESKTOP && this.props.onOpenPreview) {
            const fileName = targetPath.split('/').pop() || targetPath;
            this.props.onOpenPreview(targetPath, fileName);
        } else {
            let targetUrl = `${window.location.origin}${window.location.pathname}?preview=${encodeURIComponent(
                targetPath
            )}`;
            if (hashPart) {
                targetUrl += `#${hashPart}`;
            }
            window.open(targetUrl, '_blank');
        }
    };

    componentDidMount() {
        // Spin up the markdown worker. `new URL(..., import.meta.url)` is the
        // webpack 5-native pattern; ts-loader + webpack will emit a separate
        // worker chunk and bundle `marked` into it.
        // @ts-expect-error import.meta requires module:es2020+ in tsconfig; webpack 5 emits the correct URL at build time.
        this._mdWorker = new Worker(new URL('../../workers/markdown.worker.ts', import.meta.url), {
            type: 'module',
        });
        this._mdWorker.addEventListener('message', this.handleMdWorkerMessage);
        this._mdWorker.addEventListener('error', this.handleMdWorkerError);

        // Kick off an initial parse if the mounted file is markdown.
        this.dispatchMarkdownParse();
    }

    componentWillUnmount() {
        if (this._mdWorker) {
            this._mdWorker.removeEventListener('message', this.handleMdWorkerMessage);
            this._mdWorker.removeEventListener('error', this.handleMdWorkerError);
            this._mdWorker.terminate();
            this._mdWorker = null;
        }
    }

    componentDidUpdate(prevProps: FileDetailViewProps) {
        // Reset scroll position if the file has changed
        if (prevProps.selectedFsEntry.path !== this.props.selectedFsEntry.path) {
            this.savedScrollTop = 0;
            if (this.contentEl) {
                this.contentEl.scrollTop = 0;
            }
            if (this.editorEl) {
                this.editorEl.scrollTop = 0;
            }
            // Clear stale HTML immediately so the previous file's content
            // doesn't flash before the worker returns.
            this._mdLatestHtml = '';
            this._mdLastRenderedPath = '';
            this._mdLastRenderedContent = '';
            this.dispatchMarkdownParse();
            return;
        }

        // Re-parse when the underlying fileContent changes (e.g. after a save).
        if (prevProps.fileContent !== this.props.fileContent) {
            this.dispatchMarkdownParse();
        }

        // Restore scroll position when entering editing mode
        if (this.props.isEditingDetail && !prevProps.isEditingDetail) {
            if (this.editorEl) {
                this.editorEl.scrollTop = this.savedScrollTop;
            }
        }

        // Restore scroll position when exiting editing mode
        if (!this.props.isEditingDetail && prevProps.isEditingDetail) {
            if (this.contentEl) {
                this.contentEl.scrollTop = this.savedScrollTop;
            }
        }
    }

    /**
     * Send the current markdown content to the worker, but only if this file
     * is actually a markdown file (avoids spinning the worker for plain text,
     * code, etc. that never reach the marked branch).
     */
    private dispatchMarkdownParse() {
        const { selectedFsEntry, fileContent } = this.props;
        if (!selectedFsEntry) return;
        if (!selectedFsEntry.name.toLowerCase().endsWith('.md')) return;
        if (!this._mdWorker) return;
        if (this._mdLastRenderedPath === selectedFsEntry.path && this._mdLastRenderedContent === fileContent) {
            return;
        }
        this._mdRequestId++;
        this._mdWorker.postMessage({ id: this._mdRequestId, content: fileContent });
    }

    private handleMdWorkerMessage = (e: MessageEvent<{ id: number; html: string }>) => {
        const { id, html } = e.data;
        // Stale-response guard: only the most recent request updates the DOM.
        if (id !== this._mdRequestId) return;
        this._mdLatestHtml = html;
        this._mdLastRenderedPath = this.props.selectedFsEntry.path;
        this._mdLastRenderedContent = this.props.fileContent;
        this.forceUpdate();
    };

    private handleMdWorkerError = (e: ErrorEvent) => {
        // Worker failed (script error, marked crash, etc.). Fall back to a
        // simple escaped <pre> so the user still sees the content rather
        // than a perpetually empty preview pane.
        console.error('[md-worker] error:', e.message);
        this._mdLatestHtml = `<pre class="md-fallback">${this.escapeHtml(this.props.fileContent)}</pre>`;
        this._mdLastRenderedPath = this.props.selectedFsEntry.path;
        this._mdLastRenderedContent = this.props.fileContent;
        this.forceUpdate();
    };

    /** Minimal HTML escaper for fallback rendering when the worker is unavailable. */
    private escapeHtml(s: string): string {
        return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    render() {
        const {
            selectedFsEntry,
            favoriteFiles,
            detailFullscreen,
            isEditingDetail,
            fileContent,
            editedContent,
            fileLoading,
            fileSaving,
            fileSaveMsg,
            isImagePreview,
            imageUrl,

            onBackToList,
            onToggleFavorite,
            onCopyContent,
            onDownloadFile,
            onRenameFile,
            onToggleFullscreen,
            onSaveFile,
            onShareFile,
            isStandalone,
            language,
        } = this.props;

        const isFav = favoriteFiles.includes(selectedFsEntry.path);
        const tag = getFileTag(selectedFsEntry.name);
        const isImg = tag === 'img';
        const isMd = selectedFsEntry.name.endsWith('.md');
        const isHtml = selectedFsEntry.name.endsWith('.html') || selectedFsEntry.name.endsWith('.htm');
        const isPdf = selectedFsEntry.name.toLowerCase().endsWith('.pdf');
        const isVideo = tag === 'video';
        const isAudio = tag === 'audio';

        return (
            <div class={`fb-detail-view ${detailFullscreen ? 'fullscreen' : ''}`}>
                {/* Detail Header */}
                <div class="fb-detail-header">
                    {!isStandalone && (
                        <button class="fb-detail-back" onClick={onBackToList} title={t('fileDetail.back', language)}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="15 18 9 12 15 6" />
                            </svg>
                        </button>
                    )}
                    <div class="fb-detail-title-wrap">
                        <span class="fb-detail-filename">{selectedFsEntry.name}</span>
                        <span class="fb-detail-path">{selectedFsEntry.path}</span>
                    </div>
                    <div class="fb-detail-actions">
                        {isEditingDetail && fileSaveMsg && (
                            <span
                                class="fb-save-msg"
                                style={{
                                    fontSize: '12.5px',
                                    fontWeight: '600',
                                    color: 'var(--accent-color)',
                                    marginRight: '6px',
                                    alignSelf: 'center',
                                }}
                            >
                                {fileSaveMsg}
                            </span>
                        )}
                        <button
                            class={`fb-icon-btn ${isFav ? 'active-fav' : ''}`}
                            onClick={() => onToggleFavorite(selectedFsEntry.path)}
                            title={isFav ? t('fileDetail.unfavorite', language) : t('fileDetail.favorite', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill={isFav ? 'currentColor' : 'none'}
                                stroke="currentColor"
                                stroke-width="2"
                            >
                                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                            </svg>
                        </button>
                        {(isHtml || isPdf) && (
                            <a
                                class="fb-icon-btn"
                                href={`/api/fs/view/${selectedFsEntry.path
                                    .split('/')
                                    .map(encodeURIComponent)
                                    .join('/')}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                title={t('fileDetail.openInWindow', language)}
                                style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                                    <polyline points="15 3 21 3 21 9" />
                                    <line x1="10" x2="21" y1="14" y2="3" />
                                </svg>
                            </a>
                        )}
                        {!isImg && !isPdf && !isVideo && !isAudio && isEditingDetail && (
                            <button
                                class="fb-icon-btn"
                                onClick={onSaveFile}
                                disabled={fileSaving}
                                title={fileSaving ? t('fileDetail.saving', language) : t('fileDetail.save', language)}
                                style={{ color: 'var(--accent-color)' }}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z" />
                                    <polyline points="17 21 17 13 7 13 7 21" />
                                    <polyline points="7 3 7 8 15 8" />
                                </svg>
                            </button>
                        )}
                        {!isImg && !isPdf && !isVideo && !isAudio && isEditingDetail && (
                            <button
                                class="fb-icon-btn"
                                onClick={this.handleStopEditing}
                                title={t('fileDetail.exitEdit', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
                                    <circle cx="12" cy="12" r="3" />
                                </svg>
                            </button>
                        )}
                        {!isImg && !isPdf && !isVideo && !isAudio && !isEditingDetail && (
                            <button
                                class="fb-icon-btn"
                                onClick={this.handleStartEditing}
                                title={
                                    isHtml ? t('fileDetail.viewSource', language) : t('fileDetail.editCode', language)
                                }
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 20h9" />
                                    <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
                                </svg>
                            </button>
                        )}
                        {!isImg && !isPdf && !isVideo && !isAudio && (
                            <button
                                class="fb-icon-btn"
                                onClick={onCopyContent}
                                title={t('fileDetail.copyContent', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect width="14" height="14" x="8" y="8" rx="2" />
                                    <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                                </svg>
                            </button>
                        )}

                        <button class="fb-icon-btn" onClick={onDownloadFile} title={t('common.download', language)}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                <polyline points="7 10 12 15 17 10" />
                                <line x1="12" x2="12" y1="15" y2="3" />
                            </svg>
                        </button>
                        <button class="fb-icon-btn" onClick={onRenameFile} title={t('common.rename', language)}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
                            </svg>
                        </button>
                        {!IS_DESKTOP && (
                            <button class="fb-icon-btn" onClick={onShareFile} title={t('fileDetail.share', language)}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="18" cy="5" r="3" />
                                    <circle cx="6" cy="12" r="3" />
                                    <circle cx="18" cy="19" r="3" />
                                    <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
                                    <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
                                </svg>
                            </button>
                        )}
                        {!isStandalone && (
                            <button
                                class={`fb-icon-btn ${detailFullscreen ? 'active' : ''}`}
                                onClick={onToggleFullscreen}
                                title={
                                    detailFullscreen
                                        ? t('fileDetail.exitFullscreen', language)
                                        : t('fileDetail.fullscreen', language)
                                }
                            >
                                {detailFullscreen ? (
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M4 14h6v6M20 10h-6V4M14 10l7-7M10 14l-7 7" />
                                    </svg>
                                ) : (
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3" />
                                    </svg>
                                )}
                            </button>
                        )}
                    </div>
                </div>
                {/* Content */}
                <div
                    class="fb-detail-content"
                    ref={el => {
                        this.contentEl = el;
                    }}
                >
                    {fileLoading ? (
                        <div class="fb-loading">
                            <div class="fb-loading-spinner" />
                            <span>{t('fileDetail.loading', language)}</span>
                        </div>
                    ) : isImagePreview ? (
                        <div class="image-preview-container">
                            <img src={imageUrl} alt={selectedFsEntry.name} class="image-preview" />
                        </div>
                    ) : isImg ? (
                        <div class="fb-img-preview">
                            <span class="fb-img-placeholder">🖼 {selectedFsEntry.name}</span>
                        </div>
                    ) : isVideo ? (
                        <div class="fb-video-preview-container">
                            <video
                                class="fb-video-player"
                                controls
                                preload="metadata"
                                src={`/api/fs/view/${selectedFsEntry.path
                                    .split('/')
                                    .map(encodeURIComponent)
                                    .join('/')}`}
                            >
                                {t('fileDetail.videoUnsupported', language)}
                            </video>
                        </div>
                    ) : isAudio ? (
                        <div class="fb-audio-preview-container">
                            <div class="fb-audio-card">
                                <div class="fb-audio-vinyl"></div>
                                <div class="fb-audio-wave">
                                    <span></span>
                                    <span></span>
                                    <span></span>
                                    <span></span>
                                    <span></span>
                                </div>
                                <div class="fb-audio-title">{selectedFsEntry.name}</div>
                                <div class="fb-audio-meta">{formatBytes(selectedFsEntry.size)}</div>
                                <audio
                                    class="fb-audio-player"
                                    controls
                                    preload="metadata"
                                    src={`/api/fs/view/${selectedFsEntry.path
                                        .split('/')
                                        .map(encodeURIComponent)
                                        .join('/')}`}
                                >
                                    {t('fileDetail.audioUnsupported', language)}
                                </audio>
                            </div>
                        </div>
                    ) : isEditingDetail ? (
                        <textarea
                            class="fb-editor"
                            spellcheck={false}
                            value={editedContent}
                            onInput={e => this.props.onEditedContentChange((e.target as HTMLTextAreaElement).value)}
                            ref={el => {
                                this.editorEl = el;
                            }}
                        />
                    ) : isHtml ? (
                        <div class="fb-html-preview-container">
                            <iframe
                                src={`/api/fs/view/${selectedFsEntry.path
                                    .split('/')
                                    .map(encodeURIComponent)
                                    .join('/')}`}
                                class="fb-html-iframe"
                            />
                        </div>
                    ) : isPdf ? (
                        <div class="fb-pdf-preview-container">
                            <iframe
                                src={`/api/fs/view/${selectedFsEntry.path
                                    .split('/')
                                    .map(encodeURIComponent)
                                    .join('/')}`}
                                class="fb-pdf-iframe"
                            />
                        </div>
                    ) : isMd ? (
                        <div
                            class="fb-md-render"
                            dangerouslySetInnerHTML={{ __html: this._mdLatestHtml || this.escapeHtml(fileContent) }}
                            onClick={this.handleMarkdownClick}
                        />
                    ) : (
                        <pre class="fb-code-preview">{fileContent}</pre>
                    )}
                </div>
            </div>
        );
    }
}
