/**
 * Pretty-print tool output when it is JSON (object/array); otherwise return
 * the original text. Handles leading/trailing whitespace and double-encoded
 * JSON strings that some agents emit as output.
 *
 * If the JSON result contains priority keys like `output_for_prompt`, `formatted_output`,
 * or `output`, extract and format that key's value.
 */
export function formatToolOutput(raw: string): string {
    const text = raw.trim();
    if (!text) return raw;

    const unwrapInnerContentValue = (val: unknown): unknown => {
        if (val === null || val === undefined) return val;
        let target = val;
        if (typeof val === 'string') {
            const trimmed = val.trim();
            try {
                const parsedInner = JSON.parse(trimmed);
                if (parsedInner !== null && typeof parsedInner === 'object' && !Array.isArray(parsedInner)) {
                    target = parsedInner;
                }
            } catch {
                // Not a JSON string, leave as string
            }
        }
        if (typeof target === 'object' && target !== null && !Array.isArray(target)) {
            const obj = target as Record<string, unknown>;
            const innerKeys = [
                'content',
                'content_concise',
                'raw_output',
                'output_for_prompt',
                'formatted_output',
                'output',
            ];
            for (const k of innerKeys) {
                if (k in obj && obj[k] !== undefined && obj[k] !== null) {
                    return obj[k];
                }
            }
        }
        return val;
    };

    const tryFormatValue = (rawVal: unknown): string => {
        const val = unwrapInnerContentValue(rawVal);
        if (val === null || val === undefined) return '';
        if (typeof val === 'object') {
            return JSON.stringify(val, null, 2);
        }
        if (typeof val === 'string') {
            const trimmed = val.trim();
            try {
                const parsed = JSON.parse(trimmed);
                if (parsed !== null && typeof parsed === 'object') {
                    return JSON.stringify(parsed, null, 2);
                }
            } catch {
                // Not a JSON string, return raw string
            }
            return val;
        }
        return String(val);
    };

    const tryPretty = (s: string): string | null => {
        try {
            const parsed = JSON.parse(s);
            if (parsed !== null && typeof parsed === 'object') {
                if (!Array.isArray(parsed)) {
                    // 1. Specific "type" rules
                    const typeKeyMap: Record<string, string[]> = {
                        GrepSearch: ['file_matches'],
                        ReadFile: ['FileContent', 'content', 'content_concise', 'raw_output'],
                    };
                    if (typeof parsed.type === 'string' && parsed.type in typeKeyMap) {
                        const candidateKeys = typeKeyMap[parsed.type];
                        for (const key of candidateKeys) {
                            if (key in parsed && parsed[key] !== undefined && parsed[key] !== null) {
                                return tryFormatValue(parsed[key]);
                            }
                        }
                    }

                    // 2. Priority fallback keys: output_for_prompt -> formatted_output -> FileContent -> content -> content_concise -> raw_output -> output
                    const keys = [
                        'output_for_prompt',
                        'formatted_output',
                        'FileContent',
                        'content',
                        'content_concise',
                        'raw_output',
                        'output',
                    ];
                    for (const key of keys) {
                        if (key in parsed && parsed[key] !== undefined && parsed[key] !== null) {
                            return tryFormatValue(parsed[key]);
                        }
                    }
                }
                return JSON.stringify(parsed, null, 2);
            }
            return null;
        } catch {
            return null;
        }
    };

    const direct = tryPretty(text);
    if (direct !== null) return direct;

    // Some runtimes wrap the whole payload in an extra JSON string layer.
    if ((text.startsWith('"') && text.endsWith('"')) || (text.startsWith("'") && text.endsWith("'"))) {
        try {
            const unwrapped = JSON.parse(text);
            if (typeof unwrapped === 'string') {
                const nested = tryPretty(unwrapped.trim());
                if (nested !== null) return nested;
            }
        } catch {
            // keep original
        }
    }

    return raw;
}
