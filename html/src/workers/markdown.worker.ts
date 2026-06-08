// Markdown web worker.
//
// Runs `marked.parse()` off the main thread so opening / switching a large
// README doesn't stall the UI. The main thread sends a {id, content} request
// and receives a {id, html} response; the id is used by the caller to discard
// stale responses when newer requests supersede them.

/// <reference lib="webworker" />

import { marked } from 'marked';

interface ParseRequest {
    id: number;
    content: string;
}

interface ParseResponse {
    id: number;
    html: string;
}

const ctx: DedicatedWorkerGlobalScope = self as unknown as DedicatedWorkerGlobalScope;

ctx.addEventListener('message', (e: MessageEvent<ParseRequest>) => {
    const { id, content } = e.data;
    let html = '';
    try {
        // `async: false` because we are already on a worker thread — we want
        // to block this thread (not the main one) and post the result back.
        html = marked.parse(content, { async: false }) as string;
    } catch (err) {
        html = `<pre class="md-parse-error">Markdown parse error: ${String(err)}</pre>`;
    }
    const response: ParseResponse = { id, html };
    ctx.postMessage(response);
});

export {};
