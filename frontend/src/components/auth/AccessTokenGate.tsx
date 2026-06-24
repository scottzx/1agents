import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface AccessTokenGateProps {
    onAuthenticated: () => void;
    language: Lang;
}

interface AccessTokenGateState {
    token: string;
    loading: boolean;
    error: string;
}

export class AccessTokenGate extends Component<AccessTokenGateProps, AccessTokenGateState> {
    constructor(props: AccessTokenGateProps) {
        super(props);
        this.state = {
            token: '',
            loading: false,
            error: '',
        };
    }

    handleSubmit = async (e: Event) => {
        e.preventDefault();
        const { token } = this.state;
        if (!token.trim()) return;

        this.setState({ loading: true, error: '' });

        try {
            const res = await fetch('/api/access/verify', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token: token.trim() }),
            });

            if (!res.ok) {
                this.setState({
                    loading: false,
                    error: t('auth.invalidToken', this.props.language),
                });
                return;
            }

            const data = await res.json();
            if (data.ok) {
                this.props.onAuthenticated();
            } else {
                this.setState({
                    loading: false,
                    error: data.error || t('auth.invalidToken', this.props.language),
                });
            }
        } catch {
            this.setState({
                loading: false,
                error: t('auth.networkError', this.props.language),
            });
        }
    };

    handleInput = (e: Event) => {
        this.setState({ token: (e.target as HTMLInputElement).value, error: '' });
    };

    render() {
        const { token, loading, error } = this.state;
        const { language } = this.props;

        return (
            <div class="access-gate-overlay">
                <div class="access-gate-card">
                    <div class="access-gate-icon">
                        <svg
                            width="36"
                            height="36"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                        </svg>
                    </div>
                    <h2 class="access-gate-title">{t('auth.title', language)}</h2>
                    <p class="access-gate-desc">
                        {t('auth.descLine1', language)} {t('auth.descLine2', language)}
                    </p>
                    <form onSubmit={this.handleSubmit}>
                        <input
                            class="access-gate-input"
                            type="password"
                            placeholder={t('auth.placeholder', language)}
                            value={token}
                            onInput={this.handleInput}
                            autocomplete="off"
                            autoFocus
                        />
                        {error && <div class="access-gate-error">{error}</div>}
                        <button class="access-gate-btn" type="submit" disabled={loading || !token.trim()}>
                            {loading ? t('auth.submitting', language) : t('auth.submit', language)}
                        </button>
                    </form>
                </div>
            </div>
        );
    }
}
