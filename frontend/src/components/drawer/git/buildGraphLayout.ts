import type { GraphCommit, GraphRow } from './types';

/**
 * Trunk-anchored lane assignment: main/master first-parent chain on lane 0.
 * Pure function — cache by commits identity.
 */
export function buildGraphLayout(commits: GraphCommit[]): { rows: GraphRow[]; maxLanes: number } {
    const byHash = new Map(commits.map(c => [c.hash, c]));

    const mainTip =
        commits.find(c => c.refs.some(r => r === 'main' || r === 'master')) ||
        commits.find(c => c.refs.some(r => r.endsWith('/main') || r.endsWith('/master'))) ||
        commits[0];

    const onMain = new Set<string>();
    let cur: GraphCommit | undefined = mainTip;
    while (cur && !onMain.has(cur.hash)) {
        onMain.add(cur.hash);
        cur = cur.parents[0] ? byHash.get(cur.parents[0]) : undefined;
    }

    const lanes: (string | null)[] = [];
    const firstFree = (from: number): number => {
        let i = from;
        while (i < lanes.length && lanes[i] !== null) i++;
        while (lanes.length <= i) lanes.push(null);
        return i;
    };

    const rows: GraphRow[] = [];

    commits.forEach(commit => {
        const isOnMain = onMain.has(commit.hash);

        const incomingLanes: number[] = [];
        lanes.forEach((hh, i) => {
            if (hh === commit.hash) incomingLanes.push(i);
        });

        let nodeLane: number;
        if (isOnMain) nodeLane = 0;
        else if (incomingLanes.length) nodeLane = Math.min(...incomingLanes);
        else nodeLane = firstFree(1);
        while (lanes.length <= nodeLane) lanes.push(null);

        const aboveSnap = lanes.slice();

        incomingLanes.forEach(i => {
            lanes[i] = null;
        });

        const parentLanes: number[] = [];
        commit.parents.forEach((p, idx) => {
            if (onMain.has(p)) {
                lanes[0] = p;
                if (!parentLanes.includes(0)) parentLanes.push(0);
                return;
            }
            const existing = lanes.findIndex(h => h === p);
            if (existing >= 0) {
                if (!parentLanes.includes(existing)) parentLanes.push(existing);
                return;
            }
            let lane: number;
            if (idx === 0 && !isOnMain) lane = nodeLane;
            else lane = firstFree(1);
            lanes[lane] = p;
            if (!parentLanes.includes(lane)) parentLanes.push(lane);
        });

        const belowSnap = lanes.slice();

        const aboveLanes: number[] = [];
        aboveSnap.forEach((hh, i) => {
            if (hh !== null) aboveLanes.push(i);
        });
        const belowLanes: number[] = [];
        belowSnap.forEach((hh, i) => {
            if (hh !== null) belowLanes.push(i);
        });

        rows.push({
            ...commit,
            nodeLane,
            onMain: isOnMain,
            isMerge: commit.parents.length > 1,
            incomingLanes,
            parentLanes,
            aboveLanes,
            belowLanes,
        });
    });

    let hi = 0;
    rows.forEach(r => {
        [r.nodeLane, ...r.aboveLanes, ...r.belowLanes].forEach(L => {
            if (L > hi) hi = L;
        });
    });

    return { rows, maxLanes: hi + 1 };
}
