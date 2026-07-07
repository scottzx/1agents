import { h } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as wsStore from '../../../stores/workspaceStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type GoldSummary, type SourceRecordRow } from '@1agents/core/services/sourceService';
import { AGENT_OPTIONS } from '../TaskList/constants';
import { DataGrid } from '../TaskList/DataGrid';
import { RecordModal } from './SourceDetail';
import {
    buildSourceColumns,
    buildGroupOptions,
    compareSource,
    sourceDefaultCompare,
    sourceGroupValue,
    renderSourceCell,
    searchText,
} from './sourceGrid';

// 数据融合 (gold) — the cross-source fused view. Where silver keeps each source's
// table verbatim, gold unifies them: one person (contacts) resolved across
// feishu/email/phone channels, one conversation stream (messages), one calendar
// (events). Cards → drill mirrors the 已治理数据 zone, reusing the schema-free grid.

function goldDomainLabel(domain: string, language: Lang): string {
    const key = `datasource.gold.domain.${domain}`;
    const val = t(key, language);
    return val !== key ? val : domain;
}

function fmtTime(ms: number, language: Lang): string {
    if (!ms) return '';
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

// GoldZone — the fused-domain overview cards (联系人/消息/日历).
export function GoldZone({ onOpen }: { onOpen: (domain: string, title: string) => void }) {
    const language = ui.language.value;
    // null = still loading — renders nothing instead of flashing the empty hint.
    const [rows, setRows] = useState<GoldSummary[] | null>(null);
    const [error, setError] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .goldSummary()
            .then(all => active && setRows(all))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, []);

    return (
        <div class="datasource-detail">
            <div class="fscard-zone">
                <div class="fscard-zone-title">{t('datasource.gold.title', language)}</div>
                {error && <div class="fscard-error">{error}</div>}
                {rows !== null && rows.length === 0 && !error && (
                    <div class="contacts-empty">{t('datasource.gold.zoneEmpty', language)}</div>
                )}
                {rows !== null && rows.length > 0 && (
                    <div class="bento-grid fscard-data-grid">
                        {rows.map(r => {
                            const title = goldDomainLabel(r.domain, language);
                            return (
                                <button
                                    key={r.domain}
                                    class="bento-card fscard-data-card"
                                    onClick={() => onOpen(r.domain, title)}
                                >
                                    <div class="bento-zone-header">
                                        <span class="bento-card-title">{title}</span>
                                        <span class="fscard-data-count">
                                            {t('datasource.data.records', language, { n: r.count })}
                                        </span>
                                    </div>
                                    <div class="bento-zone-footer">
                                        <span class="fscard-data-time">
                                            {r.lastUpdated > 0
                                                ? `${t('datasource.data.lastFetched', language)} ${fmtTime(r.lastUpdated, language)}`
                                                : ''}
                                        </span>
                                        <span class="fscard-data-open">{t('datasource.data.open', language)} →</span>
                                    </div>
                                </button>
                            );
                        })}
                    </div>
                )}
            </div>
        </div>
    );
}

// PromoteTodoDialog — pick a target workspace + executor (me / an agent), then
// turn the fused to-do into a task via POST /api/data/gold/todos/promote.
function PromoteTodoDialog({
    todo,
    onClose,
    onDone,
}: {
    todo: { id: string; title: string };
    onClose: () => void;
    onDone: (msg: string) => void;
}) {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const [workspaceId, setWorkspaceId] = useState(workspaces[0]?.id ?? '');
    const [assignee, setAssignee] = useState('user');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const submit = async () => {
        if (!workspaceId) {
            setError(t('datasource.gold.promoteNoWorkspace', language));
            return;
        }
        setBusy(true);
        setError('');
        try {
            const res = await sourceService.promoteTodo({ id: todo.id, workspaceId, assignee });
            onDone(
                res.alreadyLinked
                    ? t('datasource.gold.promoteAlready', language)
                    : t('datasource.gold.promoteOk', language)
            );
            onClose();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="ds-record-overlay" onClick={onClose}>
            <div
                class="ds-record-modal ds-promote-modal"
                role="dialog"
                aria-modal="true"
                onClick={(e: Event) => e.stopPropagation()}
            >
                <button class="ds-record-close" aria-label={t('contacts.close', language)} onClick={onClose}>
                    ×
                </button>
                <h3 class="ds-record-title">{t('datasource.gold.promoteTitle', language)}</h3>
                <div class="ds-promote-todo-title">{todo.title}</div>

                <div class="form-group">
                    <label>{t('datasource.gold.promoteWorkspace', language)}</label>
                    <select
                        value={workspaceId}
                        onChange={(e: Event) => setWorkspaceId((e.target as HTMLSelectElement).value)}
                    >
                        {workspaces.length === 0 && <option value="">—</option>}
                        {workspaces.map(w => (
                            <option key={w.id} value={w.id}>
                                {w.name}
                            </option>
                        ))}
                    </select>
                </div>

                <div class="form-group">
                    <label>{t('datasource.gold.promoteAssignee', language)}</label>
                    <select
                        value={assignee}
                        onChange={(e: Event) => setAssignee((e.target as HTMLSelectElement).value)}
                    >
                        <option value="user">{t('datasource.gold.promoteAsUser', language)}</option>
                        {AGENT_OPTIONS.map(a => (
                            <option key={a} value={a}>
                                {a}
                            </option>
                        ))}
                    </select>
                </div>

                {error && <div class="contacts-error">{error}</div>}
                <div class="ds-promote-actions">
                    <button class="submit-task-btn" disabled={busy} onClick={submit}>
                        {busy ? t('datasource.gold.promoting', language) : t('datasource.gold.promote', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}

// GoldDetail — one fused domain's 多维表格 (contacts / messages / events / todos).
export function GoldDetail({ domain, title }: { domain: string; title: string }) {
    const language = ui.language.value;
    const [rows, setRows] = useState<SourceRecordRow[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const selected = useSignal<SourceRecordRow | null>(null);
    const [promoting, setPromoting] = useState<{ id: string; title: string } | null>(null);

    useEffect(() => {
        let active = true;
        setLoading(true);
        setError('');
        sourceService
            .goldRecords(domain, 1000)
            .then(r => active && setRows(r))
            .catch(e => active && setError((e as Error).message))
            .finally(() => active && setLoading(false));
        return () => {
            active = false;
        };
    }, [domain]);

    const columns = useMemo(() => buildSourceColumns(rows, language), [rows, language]);
    const groupOptions = useMemo(() => buildGroupOptions(columns, language), [columns, language]);
    const colSig = columns.map(c => c.key).join('|');

    const q = search.trim().toLowerCase();
    const filtered = q ? rows.filter(r => searchText(r).includes(q)) : rows;

    return (
        <div class="datasource-detail">
            <div class="datasource-subhead">
                <span class="datasource-subhead-title">{title}</span>
                <span class="datasource-subhead-count">{t('datasource.records', language, { n: rows.length })}</span>
                <input
                    class="contacts-search datasource-search"
                    placeholder={t('contacts.searchPlaceholder', language)}
                    value={search}
                    onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                />
            </div>
            {error && <div class="contacts-error">{error}</div>}

            <DataGrid<SourceRecordRow>
                key={colSig}
                persistKey={`gold:cols:${domain}`}
                rows={filtered}
                totalCount={rows.length}
                columns={columns}
                groupOptions={groupOptions}
                getRowKey={r => `${r.collection}:${r.uid}`}
                loading={loading}
                emptyAll={t('datasource.gold.empty', language)}
                emptyFiltered={t('contacts.emptyFiltered', language)}
                compare={compareSource}
                defaultCompare={sourceDefaultCompare}
                groupValue={sourceGroupValue}
                rowClass={r => `task-row${r.deleted ? ' ds-row-deleted' : ''}`}
                onOpenRow={r => (selected.value = r)}
                renderCell={(r, col, helpers) => renderSourceCell(r, col, helpers, language)}
            />

            {selected.value && (
                <RecordModal
                    record={selected.value}
                    onClose={() => (selected.value = null)}
                    onPromote={
                        domain === 'todos'
                            ? () => {
                                  const r = selected.value!;
                                  setPromoting({ id: r.uid, title: (r.preview || r.uid).split('\n')[0].slice(0, 80) });
                                  selected.value = null;
                              }
                            : undefined
                    }
                />
            )}

            {promoting && (
                <PromoteTodoDialog
                    todo={promoting}
                    onClose={() => setPromoting(null)}
                    onDone={msg => {
                        ui.showToast(msg);
                        setPromoting(null);
                    }}
                />
            )}
        </div>
    );
}
