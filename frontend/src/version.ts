// Build-time version metadata injected by webpack.DefinePlugin
// (see frontend/webpack.config.js → buildMeta). The ambient declarations
// live in frontend/src/global.d.ts; this module re-exports them under
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

/** Release tags from auto-release: YYYYMMDD-N (optionally with a leading v). */
const DATE_BASED = /^\d{8}-\d+$/;

export function isDateBasedVersion(v: string): boolean {
    return DATE_BASED.test(normalizeVersion(v));
}

/**
 * Compare two versions. Date-based tags ("YYYYMMDD-N" / "vYYYYMMDD-N")
 * use lexicographic order (sufficient for this format).
 *
 * Local dev builds often bake a short git SHA / "dev" / "unknown" into
 * APP_VERSION — those are not comparable as dates, so any remote
 * date-based tag is treated as newer (and vice versa: a non-date remote
 * never upgrades a date-based local).
 *
 * Returns positive if `a` is newer, negative if `b` is newer, 0 if equal.
 */
export function compareVersions(a: string, b: string): number {
    const na = normalizeVersion(a);
    const nb = normalizeVersion(b);
    if (na === nb) return 0;
    if (!na || !nb) return na ? 1 : -1;

    const aDate = DATE_BASED.test(na);
    const bDate = DATE_BASED.test(nb);
    if (aDate && !bDate) return 1;
    if (!aDate && bDate) return -1;

    return na > nb ? 1 : -1;
}

/** True if `remote` is strictly newer than `local`. */
export function isNewer(remote: string, local: string): boolean {
    return compareVersions(remote, local) > 0;
}
