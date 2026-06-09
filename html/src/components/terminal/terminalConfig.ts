import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './xterm';

const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const path = window.location.pathname.replace(/[/]+$/, '');
export const wsUrl = [protocol, '//', window.location.host, path, '/ws', window.location.search].join('');
export const tokenUrl = [window.location.protocol, '//', window.location.host, path, '/token'].join('');

export const clientOptions = {
    rendererType: 'webgl',
    disableLeaveAlert: false,
    disableResizeOverlay: false,
    enableZmodem: false,
    enableTrzsz: false,
    enableSixel: false,
    closeOnDisconnect: false,
    isWindows: false,
    unicodeVersion: '11',
} as ClientOptions;

export const flowControl = {
    limit: 100000,
    highWater: 10,
    lowWater: 4,
} as FlowControl;

export const lightTermTheme = {
    foreground: '#1f2328',
    background: '#fafafa',
    cursor: '#1f2328',
    selectionBackground: '#0969da',
    selectionForeground: '#ffffff',
    selectionInactiveBackground: '#e2e8f0',
    black: '#1f2328',
    red: '#cf222e',
    green: '#1a7f37',
    yellow: '#9a6700',
    blue: '#0969da',
    magenta: '#8250df',
    cyan: '#1b7c83',
    white: '#57606a',
    brightBlack: '#6e7781',
    brightRed: '#d1242f',
    brightGreen: '#2da44e',
    brightYellow: '#b48600',
    brightBlue: '#2188ff',
    brightMagenta: '#a371f7',
    brightCyan: '#31929a',
    brightWhite: '#1f2328',
} as ITheme;

export const darkTermTheme = {
    foreground: '#d2d2d2',
    background: '#0d1117',
    cursor: '#adadad',
    selectionBackground: '#2f81f7',
    selectionForeground: '#ffffff',
    black: '#000000',
    red: '#d81e00',
    green: '#5ea702',
    yellow: '#cfae00',
    blue: '#427ab3',
    magenta: '#89658e',
    cyan: '#00a7aa',
    white: '#dbded8',
    brightBlack: '#686a66',
    brightRed: '#f54235',
    brightGreen: '#99e343',
    brightYellow: '#fdeb61',
    brightBlue: '#84b0d8',
    brightMagenta: '#bc94b7',
    brightCyan: '#37e6e8',
    brightWhite: '#f1f1f0',
} as ITheme;

export const baseTermOptions = {
    fontFamily: 'JetBrains Mono, Consolas, Liberation Mono, Menlo, monospace',
    allowProposedApi: true,
    minimumContrastRatio: 4.5,
} as ITerminalOptions;

export const isMobileDevice = () =>
    /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) ||
    window.innerWidth <= 768;
