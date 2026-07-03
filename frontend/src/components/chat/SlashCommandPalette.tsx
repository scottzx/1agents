import { h } from 'preact';
import type { AvailableCommand } from '@1agents/core/protocol/types';

// Autocomplete palette for agent-advertised slash commands. Appears above the
// composer when the input is a single `/token` (no space yet). Selection only
// fills the input — execution is a normal prompt send (the agent parses the
// leading `/command`), so no wire action is involved.

/** Parse the active slash query from the composer text, or null if inactive. */
export function slashQuery(text: string): string | null {
    // Active only while typing the command itself: a leading '/', word chars,
    // and no whitespace yet (once a space is typed the user is writing args).
    const m = /^\/([\w-]*)$/.exec(text);
    return m ? m[1] : null;
}

export function filterCommands(commands: AvailableCommand[], query: string): AvailableCommand[] {
    if (!query) return commands;
    const q = query.toLowerCase();
    // Prefix matches first, then substring — the command you're typing ranks top.
    const prefix: AvailableCommand[] = [];
    const substr: AvailableCommand[] = [];
    for (const c of commands) {
        const name = c.name.toLowerCase();
        if (name.startsWith(q)) prefix.push(c);
        else if (name.includes(q)) substr.push(c);
    }
    return [...prefix, ...substr];
}

interface SlashCommandPaletteProps {
    commands: AvailableCommand[];
    activeIndex: number;
    onPick: (command: AvailableCommand) => void;
    onHover: (index: number) => void;
}

export function SlashCommandPalette({ commands, activeIndex, onPick, onHover }: SlashCommandPaletteProps) {
    if (commands.length === 0) return null;
    return (
        <div class="chat-slash-palette" role="listbox">
            {commands.map((cmd, i) => (
                <button
                    type="button"
                    key={cmd.name}
                    role="option"
                    aria-selected={i === activeIndex}
                    class={`chat-slash-item${i === activeIndex ? ' active' : ''}`}
                    // Keep textarea focus: pick on mousedown, before blur fires.
                    onMouseDown={(e: MouseEvent) => {
                        e.preventDefault();
                        onPick(cmd);
                    }}
                    onMouseEnter={() => onHover(i)}
                >
                    <span class="chat-slash-name">/{cmd.name}</span>
                    {cmd.description && <span class="chat-slash-desc">{cmd.description}</span>}
                </button>
            ))}
        </div>
    );
}
