import type { RoundtableTurn } from '@1agents/core/services/roundtableService';
import type { EmbeddedChatEvent } from '../chat/EmbeddedChat';

export function isR1ChatTurn(turn: RoundtableTurn): boolean {
    return turn.round === 1 && (turn.kind || '').trim().toLowerCase() === 'chat';
}

export function briefEventFromTurn(turn: RoundtableTurn): EmbeddedChatEvent | null {
    const kind = (turn.kind || '').trim().toLowerCase();
    const content = (turn.content_text || '').trim();
    const proposed =
        kind === 'brief_proposed' ||
        kind === 'system/brief_proposed' ||
        kind === 'system:brief_proposed' ||
        (kind === 'system' && /brief[\s*_：:-]*(?:草案|提案|proposed)/iu.test(content));
    const confirmed =
        kind === 'brief_confirmed' ||
        kind === 'system/brief_confirmed' ||
        kind === 'system:brief_confirmed' ||
        (kind === 'system' &&
            (/(?:已确认|confirmed)[\s\S]{0,24}brief/iu.test(content) ||
                /brief(?:[\s*_：:-]*v?\s*\d+)?[\s*_：:-]*(?:已确认|confirmed)/iu.test(content)));

    if (!proposed && !confirmed) return null;

    const version = content.match(/\bv\s*(\d+)/iu)?.[1];
    const versionLabel = version ? ` v${version}` : '';
    return {
        id: turn.id,
        label: proposed ? `Brief${versionLabel} 已提案` : `Brief${versionLabel} 已确认`,
        detail: proposed ? '在 Inspector 中查看或编辑' : '在 Inspector 中查看确认版本',
        createdAt: turn.created_at,
    };
}

export function r1BriefEvents(turns: RoundtableTurn[]): EmbeddedChatEvent[] {
    return turns.map(briefEventFromTurn).filter((event): event is EmbeddedChatEvent => Boolean(event));
}

export function timelineTurnsWithoutR1Chat(turns: RoundtableTurn[]): RoundtableTurn[] {
    return turns.filter(turn => !isR1ChatTurn(turn) && !briefEventFromTurn(turn));
}
