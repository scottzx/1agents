// Taro (小程序) platform bridge — the weapp/RN host implementation of
// @1agents/core's PlatformBridge. Lives in the miniapp package (not core) so
// core stays free of any @tarojs/* dependency; the app entry injects it via
// core's setPlatformBridge() at launch.

import Taro from '@tarojs/taro';
import { getAccessToken } from '../config';
import type {
  ConnectSocketOptions,
  PlatformBridge,
  PlatformSocket,
  PlatformStorage,
  SocketCloseInfo,
  SocketReadyState,
  UploadResult,
} from '@1agents/core/platform/bridge';

/** Taro synchronous storage. getStorageSync returns '' when missing → map to null. */
const taroStorage: PlatformStorage = {
  get(key) {
    const v = Taro.getStorageSync(key);
    return v === '' || v === undefined || v === null ? null : (v as string);
  },
  set(key, value) {
    Taro.setStorageSync(key, value);
  },
  remove(key) {
    Taro.removeStorageSync(key);
  },
};

export class TaroPlatformBridge implements PlatformBridge {
  readonly storage = taroStorage;

  /**
   * Wrap Taro.request as a minimal Response. Taro already parses a JSON body
   * into `data`; we expose ok/status/json()/text() — the surface the core
   * services use.
   */
  async httpFetch(url: string, init?: RequestInit): Promise<Response> {
    const method = (init?.method || 'GET').toUpperCase() as keyof Taro.request.Method;
    const header: Record<string, string> = { ...((init?.headers as Record<string, string>) || {}) };
    // Attach the access token (authMiddleware mechanism B) — the weapp counterpart
    // of the H5's ra_access_token cookie. No-op when unset or talking to localhost.
    const token = getAccessToken();
    if (token && !header.Authorization) header.Authorization = `Bearer ${token}`;
    const data = (init?.body as string) ?? undefined;
    const res = await Taro.request({ url, method, data, header });
    const status = res.statusCode;
    const ok = status >= 200 && status < 300;
    const responseLike = {
      ok,
      status,
      json: async () => (typeof res.data === 'string' ? JSON.parse(res.data) : res.data),
      text: async () => (typeof res.data === 'string' ? res.data : JSON.stringify(res.data)),
    };
    return responseLike as unknown as Response;
  }

  /**
   * weapp has no DOM `File`; uploads go through `Taro.uploadFile` with a local
   * temp path chosen via the media/file pickers. Wiring that up is part of the
   * later file-management work (#216 §4) — not needed for the Chat MVP.
   */
  async uploadFile(_file: File): Promise<UploadResult> {
    throw new Error('uploadFile is not implemented on the mini-program host yet');
  }

  /** No external browser on weapp; copy the link and tell the user. */
  async openExternal(url: string): Promise<void> {
    await Taro.setClipboardData({ data: url });
    await Taro.showToast({ title: '链接已复制', icon: 'none' });
  }

  connectSocket(url: string, opts?: ConnectSocketOptions): PlatformSocket {
    // weapp WebSocket can't carry the cookie nor reliably custom headers, so the
    // access token rides as a query param (authMiddleware mechanism A) on the
    // upgrade request.
    const token = getAccessToken();
    const finalUrl = token ? `${url}${url.includes('?') ? '&' : '?'}access_token=${encodeURIComponent(token)}` : url;
    return new TaroSocket(finalUrl, opts);
  }
}

/**
 * `PlatformSocket` over `Taro.connectSocket`. `Taro.connectSocket` resolves a
 * SocketTask asynchronously, so this wrapper presents a synchronous socket:
 * handlers registered before the task resolves are queued and attached once it
 * does, and `send` is buffered until the connection opens.
 */
class TaroSocket implements PlatformSocket {
  private task?: Taro.SocketTask;
  private state: SocketReadyState = 'connecting';
  private outbox: string[] = [];
  private readonly cbs = {
    open: [] as Array<() => void>,
    message: [] as Array<(data: string) => void>,
    close: [] as Array<(info: SocketCloseInfo) => void>,
    error: [] as Array<(err: unknown) => void>,
  };

  constructor(url: string, opts?: ConnectSocketOptions) {
    Taro.connectSocket({ url, protocols: opts?.protocols, header: opts?.headers })
      .then(task => {
        this.task = task;
        task.onOpen(() => {
          this.state = 'open';
          for (const msg of this.outbox) task.send({ data: msg });
          this.outbox = [];
          this.cbs.open.forEach(cb => cb());
        });
        task.onMessage(res => {
          const data = typeof res.data === 'string' ? res.data : String(res.data);
          this.cbs.message.forEach(cb => cb(data));
        });
        task.onClose(res => {
          this.state = 'closed';
          this.cbs.close.forEach(cb => cb({ code: res.code, reason: res.reason }));
        });
        task.onError(err => this.cbs.error.forEach(cb => cb(err)));
      })
      .catch(err => {
        this.state = 'closed';
        this.cbs.error.forEach(cb => cb(err));
      });
  }

  get readyState(): SocketReadyState {
    return this.state;
  }

  send(data: string): void {
    if (this.state === 'open' && this.task) {
      this.task.send({ data });
    } else {
      this.outbox.push(data);
    }
  }

  close(code?: number, reason?: string): void {
    this.state = 'closing';
    this.task?.close({ code, reason });
  }

  onOpen(cb: () => void): void {
    this.cbs.open.push(cb);
  }

  onMessage(cb: (data: string) => void): void {
    this.cbs.message.push(cb);
  }

  onClose(cb: (info: SocketCloseInfo) => void): void {
    this.cbs.close.push(cb);
  }

  onError(cb: (err: unknown) => void): void {
    this.cbs.error.push(cb);
  }
}
