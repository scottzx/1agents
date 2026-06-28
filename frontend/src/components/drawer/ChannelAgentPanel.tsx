import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { apiFetch } from '@1agents/core/services/apiClient';
import type { AgentType } from '../types';
import { AgentTypePicker } from '../chat/AgentTypePicker';
import { showToast } from '../../stores/uiStore';
import { t, type Lang } from '../../i18n';

// ChannelAgentPanel renders the #277 Phase 3 per-channel agent binding UI for one
// project (= one 1agents workspace). It lists the project's cc-connect channels
// and lets the user switch each channel's bound agent type. Switching writes back
// via POST /api/cc-connect/channels, which hot-reloads the engine (no restart).

interface ChannelBinding {
    index: number;
    type: string;
    agent: string;
    inherited: boolean;
    workDir?: string;
}

interface ProjectChannels {
    project: string;
    workDir: string;
    defaultAgent: string;
    channels: ChannelBinding[];
}

interface ChannelAgentPanelProps {
    /** cc-connect project name == workspace display name. */
    projectName: string;
    language: Lang;
}

export function ChannelAgentPanel({ projectName, language }: ChannelAgentPanelProps) {
    const [data, setData] = useState<ProjectChannels | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [savingIndex, setSavingIndex] = useState<number | null>(null);

    const load = useCallback(async () => {
        if (!projectName) {
            setData(null);
            setLoading(false);
            setError(null);
            return;
        }
        setLoading(true);
        setError(null);
        try {
            const res = await apiFetch(`/cc-connect/channels?project=${encodeURIComponent(projectName)}`);
            if (!res.ok) {
                // 404 = project has no cc-connect channels yet; treat as empty, not an error.
                if (res.status === 404) {
                    setData(null);
                    return;
                }
                throw new Error(await res.text());
            }
            setData((await res.json()) as ProjectChannels);
        } catch (err) {
            setError(err instanceof Error ? err.message : String(err));
        } finally {
            setLoading(false);
        }
    }, [projectName]);

    useEffect(() => {
        load();
    }, [load]);

    const setChannelAgent = async (index: number, agent: AgentType) => {
        setSavingIndex(index);
        try {
            const res = await apiFetch('/cc-connect/channels', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ project: projectName, index, agent }),
            });
            if (!res.ok) throw new Error(await res.text());
            // The POST returns the refreshed binding set; render it directly.
            setData((await res.json()) as ProjectChannels);
            showToast(t('drawer.channelAgent.toast.updated', language));
        } catch (err) {
            showToast(t('drawer.channelAgent.toast.failed', language));
            console.error('[ChannelAgentPanel] set channel agent failed', err);
        } finally {
            setSavingIndex(null);
        }
    };

    if (loading) {
        return <div class="channel-agent-empty">{t('drawer.channelAgent.loading', language)}</div>;
    }
    if (error) {
        return <div class="channel-agent-empty channel-agent-error">{error}</div>;
    }
    if (!data || data.channels.length === 0) {
        return <div class="channel-agent-empty">{t('drawer.channelAgent.empty', language)}</div>;
    }

    return (
        <Fragment>
            <div class="channel-agent-intro">{t('drawer.channelAgent.intro', language, { project: data.project })}</div>
            <div class="channel-agent-list">
                {data.channels.map(ch => (
                    <div class="channel-agent-card bento-card" key={ch.index}>
                        <div class="channel-agent-card-head">
                            <span class="channel-agent-card-title">{ch.type}</span>
                            {ch.inherited && (
                                <span class="channel-agent-badge">{t('drawer.channelAgent.inherited', language)}</span>
                            )}
                        </div>
                        <AgentTypePicker
                            value={ch.agent as AgentType}
                            disabled={savingIndex === ch.index}
                            onChange={agent => setChannelAgent(ch.index, agent)}
                        />
                    </div>
                ))}
            </div>
        </Fragment>
    );
}
