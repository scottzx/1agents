import { h, Component } from 'preact';
import { t, getLang } from '../../i18n';
import { AGENT_TYPE_LABELS, type AgentType, type PermissionMode } from '../types';
import { AgentTypePicker } from './AgentTypePicker';
import { AgentAvatar } from './AgentAvatar';
import { PermissionModePicker } from './PermissionModePicker';

/**
 * User-selectable roles at creation (issue #72 "创建即声明角色"). 'pmo' is
 * derived from 'pm' in the cross-project (default/builtin) workspace — see
 * createPMSession — so it is not offered directly here. Empty value = general.
 */
type CreateRole = '' | 'pm' | 'executor' | 'verifier';

const ROLE_OPTIONS: { value: CreateRole; labelKey: string }[] = [
    { value: '', labelKey: 'newchat.role.general' },
    { value: 'pm', labelKey: 'newchat.role.pm' },
    { value: 'executor', labelKey: 'newchat.role.executor' },
    { value: 'verifier', labelKey: 'newchat.role.verifier' },
];

interface SessionCreateModalProps {
    workspaceId: string;
    workspaceName: string;
    defaultAgent: AgentType;
    onCancel: () => void;
    onSubmit: (name: string, agentType: AgentType, permissionMode: PermissionMode, role: string) => void;
    onPickAgent?: (onChange: (t: AgentType) => void) => void;
}

/**
 * Modal for "new chat session" — sibling to the implicit "new terminal"
 * flow that fires when the sidebar `+` is pressed. Reached via the
 * new "新建聊天" entry in the sidebar's `+` dropdown.
 */
export class SessionCreateModal extends Component<SessionCreateModalProps> {
    state = {
        name: '',
        agentType: this.props.defaultAgent,
        permissionMode: 'approve-reads' as PermissionMode,
        role: '' as CreateRole,
    };

    componentDidUpdate(prevProps: SessionCreateModalProps) {
        if (prevProps.defaultAgent !== this.props.defaultAgent && this.state.agentType === prevProps.defaultAgent) {
            this.setState({ agentType: this.props.defaultAgent });
        }
    }

    render() {
        const { workspaceName, onCancel, onSubmit } = this.props;
        const { name, agentType, permissionMode, role } = this.state;
        const lang = getLang();
        return (
            <div class="ws-modal-overlay" onClick={onCancel}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('modal.sessionSetup.legacy.title', lang, { workspace: workspaceName })}</span>
                        <button class="ws-modal-close" onClick={onCancel}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">{t('modal.sessionSetup.nameOptional', lang)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.sessionSetup.namePlaceholder', lang)}
                            value={name}
                            onInput={(e: Event) => this.setState({ name: (e.target as HTMLInputElement).value })}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit(name, agentType, permissionMode, role);
                            }}
                            autoFocus
                        />
                        <label class="ws-modal-label">{t('sessionSetup.agent.label', lang)}</label>
                        <AgentTypePicker value={agentType} onChange={v => this.setState({ agentType: v })} />
                        <p class="ws-modal-hint">
                            {t('modal.sessionSetup.fixedHint', lang, {
                                agent: AGENT_TYPE_LABELS[agentType] ?? agentType,
                            })}
                        </p>
                        <label class="ws-modal-label">{t('newchat.role.aria', lang)}</label>
                        <div class="ws-modal-role-row">
                            <AgentAvatar agentType={agentType} />
                            <select
                                class="ws-modal-select"
                                value={role}
                                onChange={(e: Event) =>
                                    this.setState({ role: (e.target as HTMLSelectElement).value as CreateRole })
                                }
                            >
                                {ROLE_OPTIONS.map(o => (
                                    <option key={o.value} value={o.value}>
                                        {t(o.labelKey, lang)}
                                    </option>
                                ))}
                            </select>
                        </div>
                        <p class="ws-modal-hint">{t('newchat.role.pmHint', lang)}</p>
                        <label class="ws-modal-label">{t('chat.permission.mode.label', lang)}</label>
                        <PermissionModePicker
                            value={permissionMode}
                            onChange={v => this.setState({ permissionMode: v })}
                            variant="select"
                        />
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onCancel}>
                            {t('modal.sessionSetup.legacy.cancel', lang)}
                        </button>
                        <button
                            class="ws-modal-confirm"
                            onClick={() => onSubmit(name, agentType, permissionMode, role)}
                        >
                            {t('modal.sessionSetup.legacy.create', lang)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
