import { bind } from 'decko';
import { Component, h } from 'preact';
import { Xterm, XtermOptions } from './xterm';

import '@xterm/xterm/css/xterm.css';
import { Modal } from '../modal';

interface Props extends XtermOptions {
    id: string;
    onMobileDetect?: (isMobile: boolean) => void;
    onKeyboardStateChange?: (visible: boolean) => void;
}

interface State {
    modal: boolean;
    isMobile: boolean;
    hiddenInputValue: string;
    inputLeft?: string;
    inputTop?: string;
}

export class Terminal extends Component<Props, State> {
    private container: HTMLElement;
    private xterm: Xterm;
    private hiddenInput: HTMLTextAreaElement | null = null;
    private touchStartY = 0;
    private isScrolling = false;
    private hasScrolled = false;
    private isComposing = false;

    constructor(props: Props) {
        super();
        this.xterm = new Xterm(props, this.showModal);
        this.state = {
            modal: false,
            isMobile: false,
            hiddenInputValue: ' ',
            inputLeft: '0px',
            inputTop: '0px',
        };
    }

    async componentDidMount() {
        const isMobile =
            /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) ||
            window.innerWidth <= 768;
        this.setState({ isMobile });
        this.props.onMobileDetect?.(isMobile);

        await this.xterm.refreshToken();
        this.xterm.open(this.container);
        this.xterm.connect();
        window.xterm = this.xterm;
    }

    componentWillUnmount() {
        this.xterm.dispose();
        delete window.xterm;
    }

    componentDidUpdate(prevProps: Props) {
        if (
            prevProps.termOptions &&
            this.props.termOptions &&
            prevProps.termOptions.theme !== this.props.termOptions.theme &&
            this.props.termOptions.theme
        ) {
            this.xterm.setTheme(this.props.termOptions.theme);
        }
    }

    @bind
    handleTouchStart(e: TouchEvent) {
        if (e.touches.length === 1) {
            this.touchStartY = e.touches[0].clientY;
            this.isScrolling = true;
            this.hasScrolled = false;
        }
    }

    @bind
    handleTouchMove(e: TouchEvent) {
        if (!this.isScrolling || e.touches.length !== 1) return;
        const currentY = e.touches[0].clientY;
        const deltaY = currentY - this.touchStartY;
        const lineThreshold = 24; // 触控移动 24px 触发一次滚动
        if (Math.abs(deltaY) >= lineThreshold) {
            // 每次滚动精准挪动 2 行，实现非常可控和平滑的阅读体验
            const lines = deltaY > 0 ? 2 : -2;
            if (this.xterm) {
                this.xterm.scrollLines(-lines);
            }
            this.touchStartY = currentY;
            this.hasScrolled = true;
        }
    }

    @bind
    handleTouchEnd() {
        this.isScrolling = false;
    }

    @bind
    handleOverlayClick(e: MouseEvent) {
        if (this.hasScrolled) return;

        const rect = e.currentTarget ? (e.currentTarget as HTMLElement).getBoundingClientRect() : { left: 0, top: 0 };
        const x = e.clientX - rect.left;
        const y = e.clientY - rect.top;

        this.setState(
            {
                inputLeft: `${x}px`,
                inputTop: `${y}px`,
            },
            () => {
                if (this.hiddenInput) {
                    if (document.activeElement === this.hiddenInput) {
                        this.hiddenInput.blur();
                    } else {
                        this.hiddenInput.focus({ preventScroll: true });
                    }
                }
            }
        );
    }

    @bind
    handleCompositionStart() {
        this.isComposing = true;
    }

    @bind
    handleCompositionEnd(e: CompositionEvent) {
        this.isComposing = false;
        const text = e.data;
        if (text) {
            this.xterm.sendData(text);
        }
        this.setState({ hiddenInputValue: ' ' });
    }

    @bind
    handleHiddenInput(e: Event) {
        if (this.isComposing) {
            const value = (e.target as HTMLTextAreaElement).value;
            this.setState({ hiddenInputValue: value });
            return;
        }

        const value = (e.target as HTMLTextAreaElement).value;
        if (value.length === 0) {
            this.xterm.sendData('\x7f'); // Backspace
            this.setState({ hiddenInputValue: ' ' });
            return;
        }

        let typedText = '';
        if (value.startsWith(' ')) {
            typedText = value.substring(1);
        } else {
            typedText = value;
        }

        if (typedText.length > 0) {
            const processedText = typedText.replace(/\n/g, '\r');
            this.xterm.sendData(processedText);
        }

        this.setState({ hiddenInputValue: ' ' });
    }

    @bind
    handleHiddenInputFocus() {
        this.props.onKeyboardStateChange?.(true);
        setTimeout(() => {
            window.scrollTo(0, 0);
            document.body.scrollTop = 0;
            if (document.documentElement) {
                document.documentElement.scrollTop = 0;
            }
        }, 30);
    }

    @bind
    handleHiddenInputBlur() {
        setTimeout(() => {
            this.props.onKeyboardStateChange?.(false);
        }, 100);
    }

    @bind
    async sendQuickKey(key: string) {
        switch (key) {
            case '↑':
                this.xterm.sendData('\x1b[A');
                break;
            case '↓':
                this.xterm.sendData('\x1b[B');
                break;
            case '←':
                this.xterm.sendData('\x1b[D');
                break;
            case '→':
                this.xterm.sendData('\x1b[C');
                break;
            case '粘贴':
                try {
                    const text = await navigator.clipboard.readText();
                    if (text) {
                        this.xterm.sendData(text);
                    }
                } catch (err) {
                    console.error('Failed to read clipboard:', err);
                }
                break;
            case 'Esc':
                this.xterm.sendData('\x1b');
                break;
            case 'Enter':
                this.xterm.sendData('\r');
                break;
            case 'Backspace':
                this.xterm.sendData('\x7f');
                break;
            default:
                break;
        }
    }

    render({ id }: Props, { modal, isMobile, hiddenInputValue, inputLeft, inputTop }: State) {
        return (
            <div style="display: flex; flex-direction: column; height: 100%; width: 100%; position: relative;">
                <div
                    id={id}
                    style="flex: 1; min-height: 0; position: relative;"
                    ref={(c: HTMLDivElement | null) => {
                        this.container = c as HTMLElement;
                    }}
                >
                    {isMobile && (
                        <div
                            class="mobile-terminal-overlay"
                            onTouchStart={this.handleTouchStart}
                            onTouchMove={this.handleTouchMove}
                            onTouchEnd={this.handleTouchEnd}
                            onClick={this.handleOverlayClick}
                        >
                            <textarea
                                ref={el => {
                                    this.hiddenInput = el;
                                }}
                                class="hidden-terminal-input"
                                style={{
                                    left: inputLeft || '0px',
                                    top: inputTop || '0px',
                                }}
                                value={hiddenInputValue}
                                onInput={this.handleHiddenInput}
                                onFocus={this.handleHiddenInputFocus}
                                onBlur={this.handleHiddenInputBlur}
                                onCompositionStart={this.handleCompositionStart}
                                onCompositionEnd={this.handleCompositionEnd}
                                autoComplete="off"
                                autoCorrect="off"
                                autoCapitalize="off"
                                spellcheck={false}
                            />
                        </div>
                    )}
                    <Modal show={modal}>
                        <label class="file-label">
                            <input onChange={this.sendFile} class="file-input" type="file" multiple />
                            <span class="file-cta">Choose files…</span>
                        </label>
                    </Modal>
                </div>
                {isMobile && (
                    <div class="mobile-input-bar">
                        <div class="mobile-quick-keys">
                            {/* Arrow Up */}
                            <button class="key-btn" title="↑" onClick={() => this.sendQuickKey('↑')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="18 15 12 9 6 15" />
                                </svg>
                            </button>
                            {/* Arrow Down */}
                            <button class="key-btn" title="↓" onClick={() => this.sendQuickKey('↓')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="6 9 12 15 18 9" />
                                </svg>
                            </button>
                            {/* Arrow Left */}
                            <button class="key-btn" title="←" onClick={() => this.sendQuickKey('←')}>
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
                            {/* Arrow Right */}
                            <button class="key-btn" title="→" onClick={() => this.sendQuickKey('→')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="9 18 15 12 9 6" />
                                </svg>
                            </button>
                            {/* Paste */}
                            <button class="key-btn" title="粘贴" onClick={() => this.sendQuickKey('粘贴')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="8" y="4" width="12" height="16" rx="2" />
                                    <path d="M8 4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2" />
                                    <path d="M10 2h4a1 1 0 0 1 1 1v2H9V3a1 1 0 0 1 1-1z" />
                                </svg>
                            </button>
                            {/* Esc — keep text, it's clear */}
                            <button class="key-btn key-btn-text" title="Esc" onClick={() => this.sendQuickKey('Esc')}>
                                Esc
                            </button>
                            {/* Backspace / Delete */}
                            <button class="key-btn" title="Backspace" onClick={() => this.sendQuickKey('Backspace')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M21 4H8l-7 8 7 8h13a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2z" />
                                    <line x1="18" y1="9" x2="12" y2="15" />
                                    <line x1="12" y1="9" x2="18" y2="15" />
                                </svg>
                            </button>
                            {/* Enter / Return */}
                            <button class="key-btn" title="Enter" onClick={() => this.sendQuickKey('Enter')}>
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="9 10 4 15 9 20" />
                                    <path d="M20 4v7a4 4 0 0 1-4 4H4" />
                                </svg>
                            </button>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    @bind
    showModal() {
        this.setState({ modal: true });
    }

    @bind
    sendFile(event: Event) {
        this.setState({ modal: false });
        const files = (event.target as HTMLInputElement).files;
        if (files) this.xterm.sendFile(files);
    }
}
