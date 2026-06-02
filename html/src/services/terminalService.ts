import { TmuxWindow } from '../components/types';

export const terminalService = {
    async list(): Promise<TmuxWindow[]> {
        const res = await fetch('/api/terminal/list');
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return data.windows || [];
    },

    async create(workspaceId: string, cwd: string): Promise<void> {
        const res = await fetch('/api/terminal/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ workspaceId, cwd }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async switch(windowIndex: number): Promise<void> {
        const res = await fetch('/api/terminal/switch', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ windowIndex }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async kill(windowIndex: number): Promise<void> {
        const res = await fetch('/api/terminal/kill', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ windowIndex }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async getMouse(): Promise<boolean> {
        const res = await fetch('/api/terminal/mouse');
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return !!data.mouse;
    },

    async setMouse(mouse: boolean): Promise<boolean> {
        const res = await fetch('/api/terminal/mouse', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mouse }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return !!data.mouse;
    }
};
