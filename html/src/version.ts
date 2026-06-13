// Build-time version metadata injected by webpack.DefinePlugin
// (see html/webpack.config.js → buildMeta). The ambient declarations
// live in html/src/global.d.ts; this module re-exports them under
// camelCase names for ergonomic consumption in TypeScript.

export const APP_VERSION: string = __APP_VERSION__;
export const GIT_COMMIT: string = __GIT_COMMIT__;
export const BUILD_TIME: string = __BUILD_TIME__;

/** "0.4.0" or "v20260615-1" — whatever the release pipeline produces. */
export const VERSION: string = __APP_VERSION__;

/**
 * Strip the leading "v" used by git tags so manifest comparisons can
 * treat "v20260615-1" and "20260615-1" as the same version.
 */
export function normalizeVersion(v: string): string {
    return v.replace(/^v/i, '').trim();
}

/**
 * Compare two date-based versions ("YYYYMMDD-N" or "vYYYYMMDD-N").
 * Lexicographic ordering is sufficient for this format.
 * Returns positive if `a` is newer, negative if `b` is newer, 0 if equal.
 */
export function compareVersions(a: string, b: string): number {
    const na = normalizeVersion(a);
    const nb = normalizeVersion(b);
    if (na === nb) return 0;
    return na > nb ? 1 : -1;
}

/** True if `remote` is strictly newer than `local`. */
export function isNewer(remote: string, local: string): boolean {
    return compareVersions(remote, local) > 0;
}
