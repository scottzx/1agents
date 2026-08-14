import { signal } from '@preact/signals';

export interface TurnFocusRequest {
    sessionId: string;
    turnId: string;
    aliases?: string[];
    nonce: number;
}

export const turnFocusRequest = signal<TurnFocusRequest | null>(null);

export function requestTurnFocus(sessionId: string, turnId: string, aliases: string[] = []): void {
    turnFocusRequest.value = { sessionId, turnId, aliases, nonce: Date.now() };
}
