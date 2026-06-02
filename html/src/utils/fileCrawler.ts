import { FsEntry } from '../components/types';
import { fsService } from '../services/fsService';

/** Dirs to skip during recursive crawl */
const IGNORE_DIRS = new Set([
    'node_modules',
    'dist',
    'build',
    '__pycache__',
    'vendor',
    '.git',
    '.bun',
    '.yarn',
    '.pnpm',
    '.cache',
    '.vscode',
    '.idea',
]);

/** Recursively fetch all files under relPath, ignoring heavy dirs */
export async function crawlDirRecursive(
    relPath: string,
    crawlId: number,
    getCrawlCounter: () => number
): Promise<FsEntry[]> {
    if (crawlId !== getCrawlCounter()) return [];
    try {
        const entries = await fsService.list(relPath);
        if (crawlId !== getCrawlCounter()) return [];
        const results: FsEntry[] = [];
        await Promise.all(
            entries.map(async e => {
                if (crawlId !== getCrawlCounter()) return;
                if (e.isDir) {
                    if (IGNORE_DIRS.has(e.name)) return;
                    const sub = await crawlDirRecursive(e.path, crawlId, getCrawlCounter);
                    results.push(...sub);
                } else {
                    results.push(e);
                }
            })
        );
        return results;
    } catch {
        return [];
    }
}
