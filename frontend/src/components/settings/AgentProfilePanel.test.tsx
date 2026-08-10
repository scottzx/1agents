import { h } from 'preact';
import render from 'preact-render-to-string';
import test from 'node:test';
import assert from 'node:assert/strict';

import { AgentProfilePicker } from '../chat/AgentProfilePicker';
import { AgentProfilePanel, compatibleProvidersForRuntime, RuntimeOptionFields } from './AgentProfilePanel';
import type { ProviderItem } from './LlmProviderPanel';
import { agentProfiles, profileAvailability, profilesError, runtimeDefinitions } from '../../stores/agentProfileStore';
import type { RuntimeDefinition } from '../../stores/agentProfileStore';

test('AgentProfilePicker SSR only offers active dynamic profiles', () => {
    runtimeDefinitions.value = [
        { id: 'grok-build', label: 'Grok', supported_endpoint_families: ['openai'], installed: true },
    ];
    agentProfiles.value = [
        {
            id: 'deepseek-build',
            name: 'DeepSeek Build',
            runtime_id: 'grok-build',
            provider_id: 'deepseek',
            model_id: 'deepseek-v4-flash',
            revision: 2,
            status: 'active',
        },
        {
            id: 'old-build',
            name: 'Old Build',
            runtime_id: 'grok-build',
            revision: 1,
            status: 'archived',
        },
        {
            id: 'blocked-build',
            name: 'Blocked Build',
            runtime_id: 'grok-build',
            revision: 1,
            status: 'active',
        },
    ];
    profileAvailability.value = { 'blocked-build': 'runtime is not installed' };
    const html = render(<AgentProfilePicker value="deepseek-build" onChange={() => {}} allowLegacy />);
    assert.match(html, /DeepSeek Build · Grok · deepseek-v4-flash/);
    assert.doesNotMatch(html, /Old Build/);
    assert.match(html, /disabled[^>]*>Blocked Build · Grok · 不可用：runtime is not installed/);
});

test('AgentProfilePanel SSR exposes archived restore and visible error state', () => {
    profilesError.value = 'provider unavailable';
    agentProfiles.value = [
        {
            id: 'old-build',
            name: 'Old Build',
            runtime_id: 'grok-build',
            revision: 3,
            status: 'archived',
        },
    ];
    const html = render(<AgentProfilePanel />);
    assert.match(html, /provider unavailable/);
    assert.match(html, /已归档/);
    assert.match(html, /恢复/);
    profilesError.value = '';
});

test('Profile form filters endpoint families and renders runtime option schema', () => {
    const runtime: RuntimeDefinition = {
        id: 'grok-build',
        label: 'Grok',
        supported_endpoint_families: ['openai'],
        installed: true,
        option_schema: [
            { key: 'thinking', label: 'Thinking', type: 'boolean' as const },
            { key: 'mode', label: 'Mode', type: 'select' as const, choices: ['fast', 'deep'] },
            { key: 'suffix', label: 'Suffix', type: 'string' as const },
        ],
    };
    const provider = (id: string, family: 'openai' | 'anthropic', status?: string): ProviderItem => ({
        id,
        name: id,
        protocol: family,
        base_url: `https://${id}.test/v1`,
        api_key: '',
        model: 'model-a',
        status,
        endpoints: [{ family, protocol: `${family}_test`, base_url: `https://${id}.test/v1` }],
    });
    const providers = [
        provider('openai-provider', 'openai'),
        provider('anthropic-provider', 'anthropic'),
        provider('archived-provider', 'openai', 'archived'),
    ];
    assert.deepEqual(
        compatibleProvidersForRuntime(providers, runtime).map(provider => provider.id),
        ['openai-provider']
    );
    const html = render(<RuntimeOptionFields runtime={runtime} options={{ mode: 'deep' }} onChange={() => {}} />);
    assert.match(html, /Thinking/);
    assert.match(html, /<select/);
    assert.match(html, /deep/);
    assert.match(html, /Suffix/);
});
