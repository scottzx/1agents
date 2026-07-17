import { h } from 'preact';
import { t, type Lang } from '../../i18n';
import { parseDiffLines } from './parseDiff';

export interface DiffPanelProps {
    file: string;
    content: string;
    loading: boolean;
    language: Lang;
    onClose: () => void;
    /** Optional cache key — parent may pass pre-parsed lines; otherwise we parse+memo here. */
    cachedKey?: string;
}

// Module-level memo: same content string → same lines without re-parse.
let _memoKey = '';
let _memoLines: ReturnType<typeof parseDiffLines> = [];

function getParsed(content: string) {
    if (content === _memoKey) return _memoLines;
    _memoKey = content;
    _memoLines = parseDiffLines(content);
    return _memoLines;
}

export function DiffPanel({ file, content, loading, language, onClose }: DiffPanelProps) {
    const parsedLines = getParsed(content);

    return (
        <div class="git-diff-panel" onClick={e => e.stopPropagation()}>
            <div class="git-diff-header">
                <span class="git-diff-title">{file}</span>
                <button class="git-diff-close-btn" onClick={onClose} title={t('git.diff.close', language)}>
                    ×
                </button>
            </div>
            {loading ? (
                <div class="git-diff-loading">
                    <div class="git-spinner" />
                    <span>{t('git.diff.loading', language)}</span>
                </div>
            ) : parsedLines.length > 0 ? (
                <div class="git-diff-wrapper">
                    <div class="git-diff-table">
                        {parsedLines.map((line, idx) => {
                            const lineCls = `diff-line-${line.type}`;
                            return (
                                <div key={idx} class={`git-diff-row ${lineCls}`}>
                                    <div class="diff-num diff-num-old">{line.oldLineNum}</div>
                                    <div class="diff-num diff-num-new">{line.newLineNum}</div>
                                    <div class="diff-char">
                                        {line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' '}
                                    </div>
                                    <div class="diff-text">
                                        {line.type === 'add' || line.type === 'del'
                                            ? line.text.substring(1)
                                            : line.text}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </div>
            ) : (
                <div class="git-diff-empty">{t('git.diff.empty', language)}</div>
            )}
        </div>
    );
}
