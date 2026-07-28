import { signal } from '@preact/signals';

export interface TurnFocusRequest {
    sessionId: string;
    turnId: string;
    nonce: number;
}

export const turnFocusRequest = signal<TurnFocusRequest | null>(null);

export function requestTurnFocus(sessionId: string, turnId: string): void {
    turnFocusRequest.value = { sessionId, turnId, nonce: Date.now() };
}
