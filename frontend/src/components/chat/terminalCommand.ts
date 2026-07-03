// Build the command line for a terminal/execute tool from its parsed input,
// covering the common shapes: Claude Code Bash ({command:"ls -la"}), ACP
// terminal ({command:"ls", args:["-la"]}), and array-form commands. Returns
// undefined when no command is present. Durable — the input is in history, so
// the terminal block survives the post-turn reload.
export function terminalCommandLine(args: Record<string, unknown>): string | undefined {
    const base =
        typeof args.command === 'string'
            ? args.command
            : Array.isArray(args.command)
              ? args.command.filter(x => typeof x === 'string').join(' ')
              : typeof args.cmd === 'string'
                ? args.cmd
                : undefined;
    if (!base) return undefined;
    if (Array.isArray(args.args) && args.args.length > 0) {
        const rest = args.args
            .filter(x => typeof x === 'string' || typeof x === 'number')
            .map(String)
            .join(' ');
        if (rest) return `${base} ${rest}`;
    }
    return base.trim() || undefined;
}
