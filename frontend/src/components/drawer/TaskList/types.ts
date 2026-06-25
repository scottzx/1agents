// Re-export shim — the task/issue-model domain types moved to
// @1agents/core/types/task so the core taskService and the 小程序 client can
// share them. Import from '@1agents/core/types' (or this path) in new code.
export * from '@1agents/core/types/task';
