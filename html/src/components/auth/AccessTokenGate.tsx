import { h, Component } from 'preact';

interface AccessTokenGateProps {
    onAuthenticated: () => void;
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
                    error: 'Verification failed. Please try again.',
                });
                return;
            }

            const data = await res.json();
            if (data.ok) {
                this.props.onAuthenticated();
            } else {
                this.setState({
                    loading: false,
                    error: data.error || '无效的访问令牌，请重试。',
                });
            }
        } catch {
            this.setState({
                loading: false,
                error: '网络错误，请检查连接后重试。',
            });
        }
    };

    handleInput = (e: Event) => {
        this.setState({ token: (e.target as HTMLInputElement).value, error: '' });
    };

    render() {
        const { token, loading, error } = this.state;

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
                    <h2 class="access-gate-title">需要访问令牌</h2>
                    <p class="access-gate-desc">
                        此设备首次从非本地网络访问，请输入您的访问令牌以继续。
                        令牌仅在首次生成时展示，如已遗失请在设置页面重新生成。
                    </p>
                    <form onSubmit={this.handleSubmit}>
                        <input
                            class="access-gate-input"
                            type="password"
                            placeholder="请输入访问令牌..."
                            value={token}
                            onInput={this.handleInput}
                            autocomplete="off"
                            autoFocus
                        />
                        {error && <div class="access-gate-error">{error}</div>}
                        <button class="access-gate-btn" type="submit" disabled={loading || !token.trim()}>
                            {loading ? '验证中...' : '验证访问'}
                        </button>
                    </form>
                </div>
            </div>
        );
    }
}
