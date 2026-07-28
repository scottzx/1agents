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

    const tryFormatValue = (val: unknown): string => {
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
                    const keys = ['output_for_prompt', 'formatted_output', 'output'];
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
