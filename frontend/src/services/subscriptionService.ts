// Re-export shim — lives in core/services (Phase 0 carve). Import from
// '@1agents/core/services/subscriptionService' directly in new code; this
// preserves the src-relative import path used by settings components.
export * from '@1agents/core/services/subscriptionService';
