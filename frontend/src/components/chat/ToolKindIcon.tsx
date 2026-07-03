import { h } from 'preact';

// ACP tool `kind` → a small icon shown next to the tool name (Phase 6). Kinds:
// read/edit/delete/move/search/execute/think/fetch/switch_mode/other. Purely
// decorative — the tool name badge carries the text.

const PATHS: Record<string, string> = {
    read: 'M4 4h11l5 5v11H4z M15 4v5h5',
    edit: 'M12 20h9 M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z',
    delete: 'M4 7h16 M10 11v6 M14 11v6 M6 7l1 13h10l1-13 M9 7V4h6v3',
    move: 'M5 9l-3 3 3 3 M9 5l3-3 3 3 M15 19l-3 3-3-3 M19 9l3 3-3 3 M2 12h20 M12 2v20',
    search: 'M11 19a8 8 0 1 1 0-16 8 8 0 0 1 0 16z M21 21l-4.3-4.3',
    execute: 'M4 4l16 8-16 8z',
    think: 'M12 3a7 7 0 0 0-4 12.7V18h8v-2.3A7 7 0 0 0 12 3z M9 21h6',
    fetch: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z M2 12h20 M12 2c3 3 3 17 0 20 M12 2c-3 3-3 17 0 20',
    switch_mode: 'M4 8h12 M4 8l3-3 M4 8l3 3 M20 16H8 M20 16l-3-3 M20 16l-3 3',
    other: 'M12 8v.01 M12 12v4',
};

// Infer an ACP kind from the tool name so the icon survives the post-turn
// history reload (history carries the tool name, not the ACP `kind`). Matches
// on lowercased substrings — covers Claude Code + common shell tool names.
export function deriveToolKind(toolName: string): string | undefined {
    const n = toolName.toLowerCase();
    if (/(multiedit|str_replace|\bedit\b|\bwrite\b|create_file|apply_patch|notebook)/.test(n)) return 'edit';
    if (/(read|\bcat\b|open_file|view)/.test(n)) return 'read';
    if (/(grep|glob|search|find|ripgrep|\brg\b)/.test(n)) return 'search';
    if (/(bash|shell|\brun\b|execute|command|terminal|exec)/.test(n)) return 'execute';
    if (/(webfetch|websearch|\bfetch\b|curl|http)/.test(n)) return 'fetch';
    if (/(delete|remove|\brm\b|unlink)/.test(n)) return 'delete';
    if (/(\bmove\b|rename|\bmv\b)/.test(n)) return 'move';
    return undefined;
}

export function ToolKindIcon({ kind }: { kind?: string }) {
    if (!kind) return null;
    const d = PATHS[kind];
    if (!d) return null;
    return (
        <svg
            class="chat-tool-kind-icon"
            data-kind={kind}
            viewBox="0 0 24 24"
            width="13"
            height="13"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
        >
            {d.split(' M').map((seg, i) => (
                <path key={i} d={i === 0 ? seg : `M${seg}`} />
            ))}
        </svg>
    );
}
