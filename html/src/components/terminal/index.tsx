import { bind } from 'decko';
import { Component, h } from 'preact';
import { Xterm, XtermOptions } from './xterm';

import '@xterm/xterm/css/xterm.css';
import { Modal } from '../modal';
import { t, type Lang } from '../i18n';

interface Props extends XtermOptions {
    id: string;
    isMobile: boolean;
    onMobileDetect?: (isMobile: boolean) => void;
    onKeyboardStateChange?: (visible: boolean) => void;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
    language: Lang;
}

interface SpeechResultEvent {
    resultIndex: number;
    results: {
        length: number;
        [index: number]: {
            isFinal: boolean;
            [index: number]: {
                transcript: string;
            };
        };
    };
}

interface SpeechErrorEvent {
    error: string;
}

interface SpeechRecognitionInstance {
    continuous: boolean;
    interimResults: boolean;
    lang: string;
    onstart: () => void;
    onresult: (event: SpeechResultEvent) => void;
    onerror: (event: SpeechErrorEvent) => void;
    onend: () => void;
    start: () => void;
    abort: () => void;
}

interface State {
    modal: boolean;
    showInputPanel: boolean;
    panelInputValue: string;
    isRecording: boolean;
    speechText: string;
    speechError: string;
    activeSubMenu: 'commands' | 'directions' | null;
}

export class Terminal extends Component<Props, State> {
    private container: HTMLElement;
    private xterm: Xterm;
    private panelInputRef: HTMLTextAreaElement | null = null;
    private recognition: SpeechRecognitionInstance | null = null;
    private isUnmounted = false;
    private speechStartValue = '';
    private touchStartY = 0;
    private isScrolling = false;
    private hasScrolled = false;

    constructor(props: Props) {
        super();
        this.xterm = new Xterm(props, this.showModal);
        this.state = {
            modal: false,
            showInputPanel: false,
            panelInputValue: '',
            isRecording: false,
            speechText: '',
            speechError: '',
            activeSubMenu: null,
        };
    }

    async componentDidMount() {
        this.props.onMobileDetect?.(this.props.isMobile);

        await this.xterm.refreshToken();
        if (this.isUnmounted || !this.container) return;
        this.xterm.open(this.container);
        this.xterm.connect();
        window.xterm = this.xterm;
    }

    componentWillUnmount() {
        this.isUnmounted = true;
        this.cleanupSpeech();
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
            // 每次滚动精准挪动 1 行，实现极佳的阅读行控制体验
            const lines = deltaY > 0 ? 1 : -1;
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
    handleOverlayClick() {
        if (this.hasScrolled) return;
        this.toggleInputPanel();
    }

    @bind
    toggleInputPanel() {
        this.setState(
            prevState => ({ showInputPanel: !prevState.showInputPanel }),
            () => {
                if (this.state.showInputPanel && this.panelInputRef) {
                    this.panelInputRef.focus();
                }
                // Fit xterm to new space
                setTimeout(() => {
                    this.xterm.fit();
                }, 150);
            }
        );
    }

    @bind
    handlePanelInputChange(e: Event) {
        this.setState({ panelInputValue: (e.target as HTMLTextAreaElement).value });
    }

    @bind
    handlePanelInputKeyDown(e: KeyboardEvent) {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            this.sendPanelInput();
        }
    }

    @bind
    sendPanelInput() {
        const text = this.state.panelInputValue;
        if (text) {
            this.xterm.sendData(text + '\r');
            this.setState({ panelInputValue: '' }, () => {
                this.panelInputRef?.focus();
            });
        }
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
            case 'paste':
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

    @bind
    toggleSubMenu(menu: 'commands' | 'directions') {
        this.setState(prevState => ({
            activeSubMenu: prevState.activeSubMenu === menu ? null : menu,
        }));
    }

    @bind
    toggleSpeech() {
        if (this.state.isRecording) {
            this.stopSpeech();
            return;
        }

        const SpeechRecognition =
            (
                window as unknown as {
                    SpeechRecognition?: new () => SpeechRecognitionInstance;
                    webkitSpeechRecognition?: new () => SpeechRecognitionInstance;
                }
            ).SpeechRecognition ||
            (
                window as unknown as {
                    SpeechRecognition?: new () => SpeechRecognitionInstance;
                    webkitSpeechRecognition?: new () => SpeechRecognitionInstance;
                }
            ).webkitSpeechRecognition;
        if (!SpeechRecognition) {
            this.setState({ speechError: t('terminal.speech.unsupported', this.props.language) });
            setTimeout(() => this.setState({ speechError: '' }), 4000);
            return;
        }

        try {
            this.recognition = new SpeechRecognition();
            this.recognition.continuous = true;
            this.recognition.interimResults = true;

            const lang = this.props.language;
            this.recognition.lang = lang;

            this.recognition.onstart = () => {
                this.speechStartValue = this.state.panelInputValue;
                this.setState({
                    isRecording: true,
                    speechText: '',
                    speechError: '',
                });
            };

            this.recognition.onresult = (event: SpeechResultEvent) => {
                const finalParts: string[] = [];
                let interimText = '';

                const isChinese = lang.toLowerCase().startsWith('zh');
                const period = t(isChinese ? 'terminal.period.zh' : 'terminal.period.en', this.props.language);

                for (let i = 0; i < event.results.length; ++i) {
                    const result = event.results[i];
                    const transcript = result[0].transcript.trim();
                    if (result.isFinal) {
                        if (transcript) {
                            if (finalParts.length > 0) {
                                const prev = finalParts[finalParts.length - 1];
                                const endsWithPunct = /[.,!?;:。，？！、：；\s]$/.test(prev);
                                if (!endsWithPunct) {
                                    finalParts[finalParts.length - 1] = prev + period;
                                }
                            }
                            finalParts.push(transcript);
                        }
                    } else {
                        interimText += transcript;
                    }
                }

                if (interimText.trim() && finalParts.length > 0) {
                    const lastFinal = finalParts[finalParts.length - 1];
                    const endsWithPunct = /[.,!?;:。，？！、：；\s]$/.test(lastFinal);
                    if (!endsWithPunct) {
                        finalParts[finalParts.length - 1] = lastFinal + period;
                    }
                }

                let currentText = finalParts.join(' ');
                if (interimText.trim()) {
                    if (currentText) {
                        currentText += ' ' + interimText.trim();
                    } else {
                        currentText = interimText.trim();
                    }
                }

                const updatedValue = (this.speechStartValue + (this.speechStartValue ? ' ' : '') + currentText).trim();
                this.setState({
                    speechText: currentText,
                    panelInputValue: updatedValue,
                });
            };

            this.recognition.onerror = (event: SpeechErrorEvent) => {
                console.error('Speech recognition error:', event.error);
                let errMsg = t('terminal.speech.error', this.props.language);
                if (event.error === 'not-allowed') {
                    errMsg = t('terminal.speech.micDenied', this.props.language);
                } else if (event.error === 'no-speech') {
                    this.cleanupSpeech();
                    return;
                } else if (event.error === 'network') {
                    errMsg = t('terminal.speech.network', this.props.language);
                }
                this.setState({ speechError: errMsg });
                setTimeout(() => this.setState({ speechError: '' }), 4000);
                this.cleanupSpeech();
            };

            this.recognition.onend = () => {
                if (this.state.isRecording) {
                    this.stopSpeech();
                }
            };

            this.recognition.start();
        } catch (err) {
            console.error('Failed to start speech recognition:', err);
            this.setState({ speechError: t('terminal.speech.startFailed', this.props.language) });
            setTimeout(() => this.setState({ speechError: '' }), 4000);
            this.cleanupSpeech();
        }
    }

    private cleanupSpeech() {
        if (this.recognition) {
            try {
                this.recognition.abort();
            } catch (e) {
                // Ignore abort errors
            }
            this.recognition = null;
        }
        this.setState({ isRecording: false });
    }

    @bind
    cancelSpeech() {
        this.cleanupSpeech();
        this.setState({
            panelInputValue: this.speechStartValue || '',
            speechText: '',
            speechError: '',
        });
    }

    @bind
    stopSpeech() {
        this.cleanupSpeech();
        this.setState({ speechText: '', speechError: '' });
    }

    render(
        { id, language, isMobile }: Props,
        { modal, showInputPanel, panelInputValue, isRecording, speechError, activeSubMenu }: State
    ) {
        const isHttps =
            typeof window !== 'undefined' &&
            window.location &&
            (window.location.protocol === 'https:' ||
                window.location.hostname === 'localhost' ||
                window.location.hostname === '127.0.0.1');

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
                        />
                    )}
                    <Modal show={modal}>
                        <label class="file-label">
                            <input onChange={this.sendFile} class="file-input" type="file" multiple />
                            <span class="file-cta">Choose files…</span>
                        </label>
                    </Modal>
                </div>
                {isMobile && showInputPanel && (
                    <div class="mobile-input-panel">
                        <div class="panel-inner">
                            {isHttps && (
                                <button
                                    class={`panel-btn key-btn-mic ${isRecording ? 'recording' : ''}`}
                                    title={t('terminal.action.voice', language)}
                                    onClick={this.toggleSpeech}
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                                        <path d="M19 10v1a7 7 0 0 1-14 0v-1" />
                                        <line x1="12" y1="19" x2="12" y2="23" />
                                        <line x1="8" y1="23" x2="16" y2="23" />
                                    </svg>
                                </button>
                            )}
                            <div class={`panel-textarea-wrapper ${isRecording ? 'recording-active' : ''}`}>
                                <textarea
                                    ref={el => {
                                        this.panelInputRef = el;
                                    }}
                                    class="panel-textarea-inner"
                                    value={panelInputValue}
                                    onInput={this.handlePanelInputChange}
                                    onKeyDown={this.handlePanelInputKeyDown}
                                    placeholder={
                                        isRecording
                                            ? t('terminal.speech.listening', language)
                                            : t('terminal.input.placeholder', language)
                                    }
                                    rows={3}
                                    autoComplete="off"
                                    autoCorrect="off"
                                    autoCapitalize="off"
                                    spellcheck={false}
                                />
                                <button
                                    class="panel-send-inline-btn"
                                    onClick={this.sendPanelInput}
                                    disabled={!panelInputValue.trim()}
                                    title="Send"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <line x1="22" y1="2" x2="11" y2="13" />
                                        <polygon points="22 2 15 22 11 13 2 9 22 2" />
                                    </svg>
                                </button>
                            </div>
                        </div>
                    </div>
                )}
                {isMobile && !showInputPanel && (
                    <div class="mobile-input-bar">
                        {/* Secondary commands submenu rendered above the bottom row */}
                        {activeSubMenu && (
                            <div class="mobile-quick-submenu">
                                {activeSubMenu === 'commands' && (
                                    <div class="submenu-group">
                                        <button
                                            class="key-btn key-btn-command"
                                            title={t('terminal.action.runClaude', language)}
                                            onClick={() => {
                                                this.xterm.sendData('claude\r');
                                            }}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <polyline points="4 17 10 11 4 5" />
                                                <line x1="12" y1="19" x2="20" y2="19" />
                                            </svg>
                                            claude
                                        </button>
                                    </div>
                                )}
                                {activeSubMenu === 'directions' && (
                                    <div class="submenu-group">
                                        {/* Arrow Up */}
                                        <button class="key-btn" title="↑" onClick={() => this.sendQuickKey('↑')}>
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
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
                                                stroke-width="2"
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
                                                stroke-width="2"
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
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <polyline points="9 18 15 12 9 6" />
                                            </svg>
                                        </button>
                                        {/* Backspace / Delete */}
                                        <button
                                            class="key-btn"
                                            title="Backspace"
                                            onClick={() => this.sendQuickKey('Backspace')}
                                        >
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
                                    </div>
                                )}
                            </div>
                        )}
                        <div class="mobile-quick-keys">
                            {/* Toggle 快捷命令 (Quick Commands Toggle) */}
                            <button
                                class={`key-btn key-btn-submenu-toggle ${activeSubMenu === 'commands' ? 'active' : ''}`}
                                title={t('terminal.action.commands', language)}
                                onClick={() => this.toggleSubMenu('commands')}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="4 17 10 11 4 5" />
                                    <line x1="12" y1="19" x2="20" y2="19" />
                                </svg>
                            </button>
                            {/* Toggle 方向键/D-Pad (Direction Keys Toggle) */}
                            <button
                                class={`key-btn key-btn-submenu-toggle ${
                                    activeSubMenu === 'directions' ? 'active' : ''
                                }`}
                                title={t('terminal.action.directions', language)}
                                onClick={() => this.toggleSubMenu('directions')}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 3v18M3 12h18" />
                                    <polyline points="8 7 12 3 16 7" />
                                    <polyline points="8 17 12 21 16 17" />
                                    <polyline points="7 8 3 12 7 16" />
                                    <polyline points="17 8 21 12 17 16" />
                                </svg>
                            </button>
                            {/* Toggle 输入框 (Input Panel Toggle) */}
                            <button
                                class={`key-btn key-btn-input-toggle ${showInputPanel ? 'active' : ''}`}
                                title={t('terminal.action.input', language)}
                                onClick={this.toggleInputPanel}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="2" y="4" width="20" height="16" rx="2" ry="2" />
                                    <path d="M6 8h.01M10 8h.01M14 8h.01M18 8h.01M6 12h.01M18 12h.01M10 12h4" />
                                </svg>
                            </button>
                            {/* Paste */}
                            <button
                                class="key-btn"
                                title={t('terminal.action.paste', language)}
                                onClick={() => this.sendQuickKey('paste')}
                            >
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
                            {/* Esc */}
                            <button class="key-btn" title="Esc" onClick={() => this.sendQuickKey('Esc')}>
                                <span class="key-btn-label">esc</span>
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
                            {/* Tmux Mouse Toggle (Scroll vs Select Mode) */}
                            <button
                                class={`key-btn key-btn-mouse ${this.props.tmuxMouseOn ? 'active' : ''}`}
                                title={t(
                                    this.props.tmuxMouseOn ? 'terminal.mouse.scroll' : 'terminal.mouse.select',
                                    language
                                )}
                                onClick={this.props.onTmuxMouseToggle}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="5" y="2" width="14" height="20" rx="7" />
                                    <path d="M12 6v4" />
                                </svg>
                            </button>
                        </div>
                    </div>
                )}

                {/* Toast speech error if any */}
                {speechError && (
                    <div class="fb-toast speech-toast">
                        <svg
                            viewBox="0 0 24 24"
                            width="16"
                            height="16"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            style="flex-shrink: 0;"
                        >
                            <circle cx="12" cy="12" r="10" />
                            <line x1="12" y1="8" x2="12" y2="12" />
                            <line x1="12" y1="16" x2="12.01" y2="16" />
                        </svg>
                        <span>{speechError}</span>
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
