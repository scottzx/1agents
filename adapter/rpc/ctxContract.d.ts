/**
 * ctx 契约 —— happy-cli 注入给 adapter `register(ctx)` 的对象形状。
 *
 * **耦合锁定点**:这是 adapter 与 happy-cli 之间唯一的接口面。每次同步 upstream
 * (见 docs/happy-cli-fork-sync.md)后,须复核本文件是否仍与 happy-cli
 * `src/modules/common/loadRpcAdapter.ts` 的注入对象一致。
 *
 * adapter 只通过 ctx 触达 happy-cli 能力,绝不直接 import happy-cli 内部模块。
 */

export interface AdapterCtx {
  /** 注册一个 machine-scoped RPC handler(happy-cli 自动加 scope 前缀)。 */
  registerHandler(method: string, handler: (params: any) => Promise<any>): void;
  /** happy-server 中转地址。 */
  serverUrl: string;
  /** 本机 machine token(Bearer)。 */
  token: string;
  /** base64(encrypt(machineKey, variant, body)) —— 中转看不到明文。 */
  encrypt(body: unknown): string;
  /** 用 machineKey 解密;失败返回 null。 */
  decrypt(b64: string): unknown | null;
  /** 复用 happy-cli 的 file logger;缺省时 adapter 回退 console.error。 */
  log?(msg: string, ...args: unknown[]): void;
}

/** 每个子模块导出的注册函数签名。 */
export type RegisterFn = (ctx: AdapterCtx) => void | Promise<void>;
