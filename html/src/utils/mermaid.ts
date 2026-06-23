// Lazy mermaid renderer for markdown ```mermaid blocks.
//
// `renderMarkdown` emits each diagram as an inert
// `<div class="mermaid-block" data-mermaid="<encoded source>">…</div>` carrying
// the raw source. The mermaid library (~hundreds of KB) is only fetched the
// first time a real diagram needs drawing, via dynamic import → its own webpack
// chunk, so chats without diagrams pay nothing.
//
// Call renderMermaidBlocks() after the markdown HTML is in the DOM (and, for
// streamed assistant text, only once streaming has finished — a half-arrived
// diagram is a parse error). Each block remembers the theme it was drawn with
// so a light/dark toggle re-renders it from the preserved source.

type MermaidApi = {
    initialize(config: Record<string, unknown>): void;
    render(id: string, src: string): Promise<{ svg: string }>;
};

let mermaidPromise: Promise<MermaidApi> | null = null;
let initializedTheme: 'light' | 'dark' | null = null;
let idSeq = 0;

async function getMermaid(theme: 'light' | 'dark'): Promise<MermaidApi> {
    if (!mermaidPromise) {
        mermaidPromise = import('mermaid').then(m => m.default as unknown as MermaidApi);
    }
    const mermaid = await mermaidPromise;
    // mermaid.initialize is cheap and idempotent; re-run it when the theme
    // changes so subsequent renders pick up the new palette.
    if (initializedTheme !== theme) {
        mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
            theme: theme === 'dark' ? 'dark' : 'default',
        });
        initializedTheme = theme;
    }
    return mermaid;
}

/**
 * Find `.mermaid-block` placeholders under `root` and draw any that haven't
 * been rendered at the current theme. Idempotent and safe to call repeatedly:
 * blocks already drawn at `theme` are skipped. On a load or parse failure the
 * raw-source fallback is left in place (marked `.has-error`).
 */
export async function renderMermaidBlocks(root: HTMLElement | null, theme: 'light' | 'dark'): Promise<void> {
    if (!root) return;
    const pending = Array.from(root.querySelectorAll<HTMLElement>('.mermaid-block')).filter(
        b => b.dataset.renderedTheme !== theme
    );
    if (pending.length === 0) return;

    let mermaid: MermaidApi;
    try {
        mermaid = await getMermaid(theme);
    } catch {
        return; // library failed to load — keep the source fallbacks visible
    }

    for (const block of pending) {
        const src = block.dataset.mermaid ? decodeURIComponent(block.dataset.mermaid) : '';
        if (!src.trim()) continue;
        try {
            const { svg } = await mermaid.render(`mmd-${idSeq++}`, src);
            block.innerHTML = svg;
            block.dataset.renderedTheme = theme;
            block.classList.remove('has-error');
        } catch {
            // Parse/render error: leave the <pre> source fallback, flag it, and
            // mark it done so we don't loop on the same bad input at this theme.
            block.dataset.renderedTheme = theme;
            block.classList.add('has-error');
        }
    }
}
