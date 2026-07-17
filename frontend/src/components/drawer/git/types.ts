export type {
    FileStatus,
    GitStatus,
    WorktreeEntry,
    GraphCommit,
    CommitFileEntry,
    BranchEntry,
} from '../../../services/gitService';

export interface GraphRow {
    hash: string;
    short: string;
    parents: string[];
    refs: string[];
    author: string;
    time: number;
    message: string;
    nodeLane: number;
    onMain: boolean;
    isMerge: boolean;
    incomingLanes: number[];
    parentLanes: number[];
    aboveLanes: number[];
    belowLanes: number[];
}

export type DiffLineType = 'ctx' | 'add' | 'del' | 'hunk' | 'header';

export interface DiffLine {
    oldLineNum: number | '';
    newLineNum: number | '';
    type: DiffLineType;
    text: string;
}
