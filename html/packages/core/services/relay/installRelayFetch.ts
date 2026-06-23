/**
 * 嵌入面板(1skills / cc-connect)在 relay 模式下的取数修复。
 *
 * 「技能管理」「模块管理」是独立打包的 embed bundle(custom element
 * <skills-panel> / <cc-connect-panel>),内部用自己的 fetch('/api/skills')、
 * fetch('/api/v1/...') 取数,不经过宿主的 apiFetch()。direct 模式同源可达没问题,
 * 但 relay 模式下 H5 由中转/CDN 托管、同源没有后端,这些请求就打空 → 面板没数据。
 *
 * 这里在宿主启动时一次性包装全局 window.fetch:仅当 backendTarget 为 relay、
 * 且请求命中这两个子模块的同源 API 路径前缀时,改走与 apiFetch 相同的中转通道
 * (proxyApi → 已配对的本地节点);其余请求一律透传,不改变 direct/none 行为。
 *
 * 必须在 embed 模块脚本执行前同步安装(见 index.tsx),否则拦截不到。
 */
import { backendTarget } from '../apiClient';
import { proxyApi } from './relayClient';

// 两个子模块的数据路由前缀,对应后端 server.go 里的反代路由。命中即经中转转发。
const RELAY_PREFIXES = [
    '/api/skills',
    '/api/mcp/',
    '/api/slash-commands',
    '/api/marketplace/',
    '/api/scan/',
    '/api/settings',
    '/api/health',
    '/1skills/',
    '/cc-connect/',
    '/api/v1/',
    '/assets/',
];

function shouldRelay(pathname: string): boolean {
    // embed 脚本本身的加载不能改路由(由中转/CDN 作为静态文件提供)。
    if (pathname.startsWith('/api/embed/')) return false;
    return RELAY_PREFIXES.some(p => pathname === p || pathname.startsWith(p));
}

export function installRelayFetch(): void {
    const native = window.fetch.bind(window);

    window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const t = backendTarget.value;
        // 仅 relay 模式介入;direct/probing/none 保持原样。
        if (t.mode !== 'relay') return native(input, init);

        const isRequest = typeof Request !== 'undefined' && input instanceof Request;
        const rawUrl = typeof input === 'string' ? input : isRequest ? input.url : String(input);

        let url: URL;
        try {
            url = new URL(rawUrl, window.location.origin);
        } catch {
            return native(input, init);
        }
        // 只拦同源的子模块 API;跨域、非命中路径都透传。
        if (url.origin !== window.location.origin || !shouldRelay(url.pathname)) {
            return native(input, init);
        }

        // 方法/头/体:Request 对象优先,其次 init。
        const method = (init?.method || (isRequest ? input.method : 'GET')).toUpperCase();

        let headers: Record<string, string> | undefined;
        const rawHeaders = init?.headers ?? (isRequest ? input.headers : undefined);
        if (rawHeaders) {
            headers = {};
            if (rawHeaders instanceof Headers) {
                rawHeaders.forEach((v, k) => (headers![k] = v));
            } else if (Array.isArray(rawHeaders)) {
                for (const [k, v] of rawHeaders) headers[k] = v;
            } else {
                headers = { ...(rawHeaders as Record<string, string>) };
            }
        }

        let body: string | undefined;
        if (init?.body !== null && init?.body !== undefined) {
            body = typeof init.body === 'string' ? init.body : String(init.body);
        } else if (isRequest && method !== 'GET' && method !== 'HEAD') {
            body = await input.clone().text();
        }

        // 子模块发出的已是完整路径(/api/skills、/1skills/...),原样透传给中转,
        // 不要再补 /api 前缀(apiFetch 补前缀是因为其入参不含 /api)。
        const r = await proxyApi(t.socket, t.machine, url.pathname + url.search, { method, body, headers });
        return new Response(r.body ?? '', {
            status: r.status ?? (r.success ? 200 : 502),
            headers: { 'content-type': 'application/json' },
        });
    };
}
