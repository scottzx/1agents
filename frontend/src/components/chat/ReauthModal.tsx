// Re-auth modal — opened automatically when the bridge pushes `auth_required`
// (so the user lands on the form the moment the agent demands credentials)
// AND manually from the session auth badge's 重新认证 / 登录 button.
//
// Per-method forms are inline-expanded (no separate page); the modal keeps
// the user's input across auth_failed retries so a bad password doesn't
// wipe their typing. OAuth methods get a "在浏览器中打开" button that pops
// agent.authUrl in a new tab; on return the user pastes the callback code
// into the inline code field (UI-only — the agent decides what to do with it).

import { h, Fragment, Component } from 'preact';
import type { AuthMethod } from '@1agents/core/protocol/types';
import { t, type Lang } from '../../i18n';
import * as ui from '../../stores/uiStore';

interface ReauthModalProps {
    methods: AuthMethod[];
    /** Bridge-side reason (e.g. "Token expired"). */
    message?: string;
    /** True while the modal is awaiting auth_completed / auth_failed. */
    submitting: boolean;
    /** Error returned by the bridge (code='auth_failed') — kept across retries. */
    errorMessage?: string;
    onClose: () => void;
    onSubmit: (methodId: string, credentials?: Record<string, string>) => void;
}

interface ReauthModalState {
    /** id of the method whose form is currently expanded (default = first). */
    expandedId: string | null;
    /** Per-method credential values, keyed by method id. Persists across retries. */
    credentials: Record<string, Record<string, string>>;
}

/**
 * Class component (per codebase convention). Per-method credential values
 * live in component state so a `setField` doesn't re-render the whole modal
 * tree on every keystroke.
 */
export class ReauthModal extends Component<ReauthModalProps, ReauthModalState> {
    constructor(props: ReauthModalProps) {
        super(props);
        const firstMethod = props.methods[0];
        this.state = {
            expandedId: firstMethod ? firstMethod.id : null,
            credentials: {},
        };
    }

    setExpanded = (id: string) => {
        this.setState({ expandedId: id });
    };

    setField = (methodId: string, field: string, value: string) => {
        const cur = this.state.credentials[methodId] ?? {};
        this.setState({
            credentials: { ...this.state.credentials, [methodId]: { ...cur, [field]: value } },
        });
    };

    handleSubmit = (method: AuthMethod) => {
        const values = this.state.credentials[method.id] ?? {};
        this.props.onSubmit(method.id, values);
    };

    render() {
        const { methods, message, submitting, errorMessage, onClose } = this.props;
        const { expandedId } = this.state;
        const language: Lang = ui.language.value;
        const hasMethods = methods.length > 0;

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('chat.auth.modal.title', language)}</span>
                        <button
                            class="ws-modal-close"
                            onClick={onClose}
                            aria-label={t('chat.auth.modal.cancel', language)}
                        >
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body reauth-modal-body">
                        {message && <div class="reauth-modal-banner">{message}</div>}
                        {errorMessage && (
                            <div class="reauth-modal-error" role="alert">
                                {t('chat.auth.failed', language, { message: errorMessage })}
                            </div>
                        )}
                        {hasMethods ? (
                            <Fragment>
                                <p class="reauth-modal-prompt">{t('chat.auth.modal.choosePrompt', language)}</p>
                                <ul class="reauth-modal-methods" role="list">
                                    {methods.map(method => {
                                        const expanded = expandedId === method.id;
                                        const label = method.name || t('chat.auth.methodLabel.placeholder', language);
                                        const typeLabel = t(`chat.auth.method.${method.type}`, language);
                                        return (
                                            <li
                                                key={method.id}
                                                class={`reauth-modal-method ${
                                                    expanded ? 'reauth-modal-method--expanded' : ''
                                                }`}
                                            >
                                                <button
                                                    type="button"
                                                    class="reauth-modal-method__head"
                                                    onClick={() => this.setExpanded(method.id)}
                                                    aria-expanded={expanded}
                                                >
                                                    <span class="reauth-modal-method__name">{label}</span>
                                                    <span class="reauth-modal-method__type">{typeLabel}</span>
                                                </button>
                                                {expanded && (
                                                    <MethodForm
                                                        method={method}
                                                        submitting={submitting}
                                                        language={language}
                                                        onFieldChange={(field, value) =>
                                                            this.setField(method.id, field, value)
                                                        }
                                                        values={this.state.credentials[method.id] ?? {}}
                                                        onSubmit={() => this.handleSubmit(method)}
                                                    />
                                                )}
                                            </li>
                                        );
                                    })}
                                </ul>
                            </Fragment>
                        ) : (
                            <p class="reauth-modal-prompt">{t('chat.auth.modal.noMethods', language)}</p>
                        )}
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onClose}>
                            {t('chat.auth.modal.cancel', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}

/**
 * The credential form for one auth method. Three shapes keyed on
 * `method.type`; OAuth also gets a "在浏览器中打开" link to agent.authUrl.
 */
interface MethodFormProps {
    method: AuthMethod;
    submitting: boolean;
    language: Lang;
    values: Record<string, string>;
    onFieldChange: (field: string, value: string) => void;
    onSubmit: () => void;
}

function MethodForm({ method, submitting, language, values, onFieldChange, onSubmit }: MethodFormProps) {
    return (
        <div class="reauth-modal-method__form">
            {method.type === 'oauth' && (
                <div class="reauth-modal-oauth">
                    {method.authUrl && (
                        <a
                            class="reauth-modal-oauth__link"
                            href={method.authUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                        >
                            {t('chat.auth.modal.openBrowser', language)}
                        </a>
                    )}
                    <label class="reauth-modal-label">
                        {t('chat.auth.modal.codeLabel', language)}
                        <input
                            class="reauth-modal-input"
                            type="text"
                            placeholder={t('chat.auth.modal.codePlaceholder', language)}
                            value={values.code ?? ''}
                            onInput={(e: Event) => onFieldChange('code', (e.target as HTMLInputElement).value)}
                            autoComplete="off"
                        />
                    </label>
                </div>
            )}
            {method.type === 'api_key' && (
                <label class="reauth-modal-label">
                    {t('chat.auth.modal.apiKeyLabel', language)}
                    <input
                        class="reauth-modal-input"
                        type="password"
                        placeholder={t('chat.auth.modal.apiKeyPlaceholder', language)}
                        value={values.apiKey ?? ''}
                        onInput={(e: Event) => onFieldChange('apiKey', (e.target as HTMLInputElement).value)}
                        autoComplete="off"
                    />
                </label>
            )}
            {method.type === 'credentials' && (
                <>
                    <label class="reauth-modal-label">
                        {t('chat.auth.modal.usernameLabel', language)}
                        <input
                            class="reauth-modal-input"
                            type="text"
                            placeholder={t('chat.auth.modal.usernamePlaceholder', language)}
                            value={values.username ?? ''}
                            onInput={(e: Event) => onFieldChange('username', (e.target as HTMLInputElement).value)}
                            autoComplete="username"
                        />
                    </label>
                    <label class="reauth-modal-label">
                        {t('chat.auth.modal.passwordLabel', language)}
                        <input
                            class="reauth-modal-input"
                            type="password"
                            placeholder={t('chat.auth.modal.passwordPlaceholder', language)}
                            value={values.password ?? ''}
                            onInput={(e: Event) => onFieldChange('password', (e.target as HTMLInputElement).value)}
                            autoComplete="current-password"
                        />
                    </label>
                </>
            )}
            <button class="ws-modal-confirm reauth-modal-submit" type="button" onClick={onSubmit} disabled={submitting}>
                {submitting ? t('chat.auth.modal.submitting', language) : t('chat.auth.modal.submit', language)}
            </button>
        </div>
    );
}
