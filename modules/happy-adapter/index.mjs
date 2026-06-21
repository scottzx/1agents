/**
 * 兼容 shim —— 真正实现已重定位到结构化的 adapter/ 接缝(见 ../../adapter/)。
 *
 * 本文件保留只为兼容旧的 `HAPPY_RPC_ADAPTER_ENTRY` 路径(指向 modules/happy-adapter/index.mjs)。
 * 新部署请把 HAPPY_RPC_ADAPTER_ENTRY 指向 adapter/rpc/index.mjs。
 *
 * 行为不变:re-export adapter 根入口的 register(ctx)。
 */
export { register, default } from '../../adapter/rpc/index.mjs';
