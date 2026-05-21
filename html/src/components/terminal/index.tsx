import { bind } from 'decko';
import { Component, h } from 'preact';
import { Xterm, XtermOptions } from './xterm';

import '@xterm/xterm/css/xterm.css';
import { Modal } from '../modal';

interface Props extends XtermOptions {
    id: string;
    onBottomNavFocus?: (focused: boolean) => void;
}

interface State {
    modal: boolean;
    isMobile: boolean;
    mobileInput: string;
}

export class Terminal extends Component<Props, State> {
    private container: HTMLElement;
    private xterm: Xterm;

    constructor(props: Props) {
        super();
        this.xterm = new Xterm(props, this.showModal);
        this.state = {
            modal: false,
            isMobile: false,
            mobileInput: '',
        };
    }

    async componentDidMount() {
        const isMobile =
            /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) ||
            window.innerWidth <= 768;
        this.setState({ isMobile });

        await this.xterm.refreshToken();
        this.xterm.open(this.container);
        this.xterm.connect();
    }

    componentWillUnmount() {
        this.xterm.dispose();
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
    handleMobileInput(event: Event) {
        const value = (event.target as HTMLInputElement).value;
        this.setState({ mobileInput: value });
    }

    @bind
    handleMobileKeyDown(event: KeyboardEvent) {
        if (event.key === 'Enter') {
            event.preventDefault();
            this.sendMobileInput();
        }
    }

    @bind
    sendMobileInput() {
        const { mobileInput } = this.state;
        if (mobileInput) {
            this.xterm.sendData(mobileInput + '\r');
            this.setState({ mobileInput: '' });
        }
    }

    @bind
    sendQuickKey(key: string) {
        switch (key) {
            case 'Tab':
                this.xterm.sendData('\t');
                break;
            case 'Ctrl+C':
                this.xterm.sendData('\x03');
                break;
            case 'Esc':
                this.xterm.sendData('\x1b');
                break;
            case 'Enter':
                this.xterm.sendData('\r');
                break;
            case 'Space':
                this.xterm.sendData(' ');
                break;
            case 'Backspace':
                this.xterm.sendData('\x7f');
                break;
            default:
                break;
        }
    }

    render({ id }: Props, { modal, isMobile, mobileInput }: State) {
        return (
            <div
                class="terminal-wrapper"
                style="display: flex; flex-direction: column; height: 100%; width: 100%; position: relative;"
            >
                <div
                    id={id}
                    style="flex: 1; min-height: 0;"
                    ref={(c: HTMLDivElement | null) => {
                        this.container = c as HTMLElement;
                    }}
                >
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
                            {['Tab', 'Ctrl+C', 'Esc', 'Enter', 'Space', 'Backspace'].map(key => (
                                <button key={key} class="key-btn" onClick={() => this.sendQuickKey(key)}>
                                    {key}
                                </button>
                            ))}
                        </div>
                        <div class="mobile-input-row">
                            <input
                                type="text"
                                value={mobileInput}
                                onInput={this.handleMobileInput}
                                onKeyDown={this.handleMobileKeyDown}
                                onFocus={() => this.props.onBottomNavFocus?.(true)}
                                onBlur={() => this.props.onBottomNavFocus?.(false)}
                                placeholder="在此输入以同步到终端..."
                                class="mobile-terminal-input"
                                autocorrect="off"
                                autocapitalize="none"
                                autocomplete="off"
                                spellcheck={false}
                            />
                            <button class="mobile-terminal-send" onClick={this.sendMobileInput}>
                                发送
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
