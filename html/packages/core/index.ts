// Public entry for @1agents/core.
//
// Framework-agnostic frontend core: wire protocol, API/relay services, the
// platform bridge, and shared data types. Consumed as TypeScript source.
//
// Subpath imports are first-class and preferred for tree-shaking, e.g.:
//   import { backendTarget } from '@1agents/core/services/apiClient';
//   import type { ConnectionState } from '@1agents/core/protocol/types';
//   import { initPlatformBridge } from '@1agents/core/platform/bridge';
//
// This barrel re-exports the stable surface for consumers that want a single
// entry point (e.g. the planned Taro mini-program / RN app).

export * from './types';
export * from './protocol/types';
export * from './platform/bridge';
