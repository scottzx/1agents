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

// Open a rendered diagram full-size in a pan/zoom viewer. The inline diagram is
// height-capped for readability (see `.mermaid-block` in the SCSS), so this is
// how the user inspects a large one in detail: pinch / wheel to zoom, drag to
// pan, toolbar for ±/reset. Built imperatively to match the diagrams, which
// live outside the preact tree (innerHTML SVG).
function openLightbox(svgMarkup: string): void {
    const overlay = document.createElement('div');
    overlay.className = 'mermaid-lightbox-overlay';

    // `stage` is the transform target (translate + scale); the diagram lives
    // inside it. transform-origin is 0 0 so the zoom-around-focus math below
    // stays simple.
    const stage = document.createElement('div');
    stage.className = 'mermaid-lightbox-stage';
    stage.innerHTML = svgMarkup;

    const closeBtn = document.createElement('button');
    closeBtn.type = 'button';
    closeBtn.className = 'mermaid-lightbox-close';
    closeBtn.setAttribute('aria-label', 'Close');
    closeBtn.textContent = '✕';

    const toolbar = document.createElement('div');
    toolbar.className = 'mermaid-lightbox-toolbar';
    const mkBtn = (label: string, title: string) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.textContent = label;
        b.title = title;
        b.setAttribute('aria-label', title);
        return b;
    };
    const zoomOutBtn = mkBtn('−', 'Zoom out');
    const resetBtn = mkBtn('⏹', 'Reset');
    const zoomInBtn = mkBtn('＋', 'Zoom in');
    toolbar.append(zoomOutBtn, resetBtn, zoomInBtn);

    overlay.append(stage, toolbar, closeBtn);

    // ── pan / zoom state ──
    let scale = 1;
    let tx = 0;
    let ty = 0;
    const MIN = 0.1;
    const MAX = 12;
    const apply = () => {
        stage.style.transform = `translate(${tx}px, ${ty}px) scale(${scale})`;
    };
    // Zoom by `factor` while keeping the point (fx, fy) — in overlay/viewport
    // pixels — fixed under the cursor or pinch midpoint.
    const zoomAt = (fx: number, fy: number, factor: number) => {
        const next = Math.min(MAX, Math.max(MIN, scale * factor));
        const k = next / scale;
        tx = fx - (fx - tx) * k;
        ty = fy - (fy - ty) * k;
        scale = next;
        apply();
    };
    const fitCenter = () => {
        const sw = stage.offsetWidth;
        const sh = stage.offsetHeight;
        const vw = overlay.clientWidth;
        const vh = overlay.clientHeight;
        if (!sw || !sh) return;
        scale = Math.min((vw * 0.9) / sw, (vh * 0.82) / sh, 2) || 1;
        tx = (vw - sw * scale) / 2;
        ty = (vh - sh * scale) / 2;
        apply();
    };

    // Pin the SVG to its natural (viewBox) pixel size so `stage` has a stable
    // box to transform — mermaid otherwise renders it at width:100%.
    const svg = stage.querySelector('svg');
    if (svg) {
        const vb = (svg.getAttribute('viewBox') || '').split(/[\s,]+/).map(Number);
        if (vb[2] && vb[3]) {
            svg.style.width = `${vb[2]}px`;
            svg.style.height = `${vb[3]}px`;
            svg.style.maxWidth = 'none';
        }
    }

    // ── gestures: 1 pointer pans, 2 pinch-zoom; wheel zooms ──
    const pts = new Map<number, { x: number; y: number }>();
    let pinchDist = 0;
    const spread = () => {
        const [a, b] = [...pts.values()];
        return Math.hypot(a.x - b.x, a.y - b.y);
    };
    const midpoint = () => {
        const [a, b] = [...pts.values()];
        return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
    };
    stage.addEventListener('pointerdown', e => {
        pts.set(e.pointerId, { x: e.clientX, y: e.clientY });
        if (pts.size === 2) pinchDist = spread();
        stage.setPointerCapture?.(e.pointerId);
        e.preventDefault();
    });
    stage.addEventListener('pointermove', e => {
        const prev = pts.get(e.pointerId);
        if (!prev) return;
        if (pts.size >= 2) {
            pts.set(e.pointerId, { x: e.clientX, y: e.clientY });
            const d = spread();
            if (pinchDist > 0) {
                const m = midpoint();
                zoomAt(m.x, m.y, d / pinchDist);
            }
            pinchDist = d;
        } else {
            tx += e.clientX - prev.x;
            ty += e.clientY - prev.y;
            pts.set(e.pointerId, { x: e.clientX, y: e.clientY });
            apply();
        }
        e.preventDefault();
    });
    const release = (e: PointerEvent) => {
        pts.delete(e.pointerId);
        if (pts.size < 2) pinchDist = 0;
    };
    stage.addEventListener('pointerup', release);
    stage.addEventListener('pointercancel', release);
    overlay.addEventListener(
        'wheel',
        e => {
            e.preventDefault();
            zoomAt(e.clientX, e.clientY, e.deltaY < 0 ? 1.12 : 1 / 1.12);
        },
        { passive: false }
    );

    const cx = () => overlay.clientWidth / 2;
    const cy = () => overlay.clientHeight / 2;
    zoomInBtn.addEventListener('click', e => {
        e.stopPropagation();
        zoomAt(cx(), cy(), 1.25);
    });
    zoomOutBtn.addEventListener('click', e => {
        e.stopPropagation();
        zoomAt(cx(), cy(), 0.8);
    });
    resetBtn.addEventListener('click', e => {
        e.stopPropagation();
        fitCenter();
    });
    // Keep toolbar / diagram interactions from reaching the backdrop dismiss.
    toolbar.addEventListener('click', e => e.stopPropagation());
    stage.addEventListener('click', e => e.stopPropagation());

    // ── dismiss ──
    const close = () => {
        overlay.remove();
        document.removeEventListener('keydown', onKey);
    };
    const onKey = (e: KeyboardEvent) => {
        if (e.key === 'Escape') close();
    };
    overlay.addEventListener('click', close); // backdrop
    closeBtn.addEventListener('click', e => {
        e.stopPropagation();
        close();
    });
    document.addEventListener('keydown', onKey);

    document.body.appendChild(overlay);
    // Fit once layout is known (offsetWidth/Height need a frame).
    requestAnimationFrame(fitCenter);
}

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
            block.classList.add('is-rendered');
            // Click to open full-size. Assigned (not addEventListener) so a
            // theme re-render replaces the handler instead of stacking one.
            block.onclick = () => {
                const node = block.querySelector('svg');
                if (node) openLightbox(node.outerHTML);
            };
        } catch {
            // Parse/render error: leave the <pre> source fallback, flag it, and
            // mark it done so we don't loop on the same bad input at this theme.
            block.dataset.renderedTheme = theme;
            block.classList.add('has-error');
        }
    }
}
