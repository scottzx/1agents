if (process.env.NODE_ENV === 'development') {
    require('preact/debug');
}
import 'whatwg-fetch';
import { h, render } from 'preact';
import { App } from './components/app';
import { initPlatformBridge } from './core/platform/bridge';
import './style/index.scss';

if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('./sw.js');
}

// Swap in the host-specific platform bridge (no-op on the web). Fire-and-forget:
// the only bridge op wired so far (file upload) is identical across hosts, so
// rendering need not wait for the desktop bridge to load.
void initPlatformBridge();

render(<App />, document.body);
