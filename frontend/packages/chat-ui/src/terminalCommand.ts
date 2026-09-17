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
