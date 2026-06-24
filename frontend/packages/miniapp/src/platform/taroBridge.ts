// Taro (小程序) platform bridge — the weapp/RN host implementation of
// @1agents/core's PlatformBridge. Lives in the miniapp package (not core) so
// core stays free of any @tarojs/* dependency; the app entry injects it via
// core's setPlatformBridge() at launch.

import Taro from '@tarojs/taro';
import type {
  ConnectSocketOptions,
  PlatformBridge,
  PlatformSocket,
  SocketCloseInfo,
  SocketReadyState,
  UploadResult,
} from '@1agents/core/platform/bridge';

export class TaroPlatformBridge implements PlatformBridge {
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
    return new TaroSocket(url, opts);
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
