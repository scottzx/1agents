if (process.env.NODE_ENV === 'development') {
    require('preact/debug');
}
import 'whatwg-fetch';
import { h, render } from 'preact';
import { App } from './components/app';
import './apps'; // register installable-app mount-point views (Epic #317)
import { initPlatformBridge } from '@1agents/core/platform/bridge';
import { installRelayFetch } from '@1agents/core/services/relay/installRelayFetch';
import './style/index.scss';

// 同步包装 window.fetch:让 1skills / cc-connect 嵌入面板的取数在 relay 模式下也走中转。
// 必须在 embed 模块脚本(template.html 的 <script type="module">,deferred)执行前装好。
installRelayFetch();

if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('./sw.js');
}

// Swap in the host-specific platform bridge (no-op on the web). Fire-and-forget:
// the only bridge op wired so far (file upload) is identical across hosts, so
// rendering need not wait for the desktop bridge to load.
void initPlatformBridge();

render(<App />, document.body);
