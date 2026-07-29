import type { Milestone } from './types';

export interface SemVer {
    major: number;
    minor: number;
    patch: number;
}

const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

export function parseSemVer(value: string): SemVer | null {
    const match = SEMVER_PATTERN.exec(value);
    if (!match) return null;
    const parts = match.slice(1).map(Number);
    if (parts.some(part => !Number.isSafeInteger(part))) return null;
    return { major: parts[0], minor: parts[1], patch: parts[2] };
}

export function compareSemVer(a: Milestone, b: Milestone): number {
    const aVersion = parseSemVer(a.version || '');
    const bVersion = parseSemVer(b.version || '');
    if (!aVersion || !bVersion) return 0;
    return aVersion.major - bVersion.major || aVersion.minor - bVersion.minor || aVersion.patch - bVersion.patch;
}

export function splitMilestones(milestones: Milestone[]): {
    versions: Milestone[];
    legacy: Milestone[];
} {
    const versions: Milestone[] = [];
    const legacy: Milestone[] = [];
    for (const milestone of milestones) {
        if (!milestone.isLegacy && parseSemVer(milestone.version || '')) versions.push(milestone);
        else legacy.push(milestone);
    }
    // The roadmap is newest-first. Position is deliberately ignored for
    // versioned milestones: SemVer is their only display ordering contract.
    versions.sort((a, b) => compareSemVer(b, a));
    legacy.sort((a, b) => a.position - b.position);
    return { versions, legacy };
}
