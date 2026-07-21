/**
 * Local-operator host detection.
 *
 * Used by happy-cli / LocalMachinePanel bypass: opening the operator UI via a
 * LAN address (e.g. http://192.168.5.200:8085/) should behave the same as
 * localhost — show 本机 Relay management, not the remote-client pairing UI.
 */

function isPrivateIPv4(hostname: string): boolean {
    const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(hostname);
    if (!m) return false;
    const a = Number(m[1]);
    const b = Number(m[2]);
    const c = Number(m[3]);
    const d = Number(m[4]);
    if ([a, b, c, d].some(n => n > 255)) return false;
    if (a === 10) return true; // 10.0.0.0/8
    if (a === 172 && b >= 16 && b <= 31) return true; // 172.16.0.0/12
    if (a === 192 && b === 168) return true; // 192.168.0.0/16
    if (a === 169 && b === 254) return true; // link-local
    if (a === 127) return true; // loopback range
    return false;
}

function isPrivateIPv6(hostname: string): boolean {
    // Strip brackets from [fe80::1] style hosts.
    const h = hostname.replace(/^\[|\]$/g, '').toLowerCase();
    if (h === '::1') return true;
    // Unique local (fc00::/7) and link-local (fe80::/10)
    if (h.startsWith('fc') || h.startsWith('fd')) return true;
    if (h.startsWith('fe80:')) return true;
    return false;
}

/** True when hostname is pure loopback (same-machine browser ↔ server). */
export function isLoopbackHost(hostname?: string): boolean {
    const h = (hostname ?? (typeof window !== 'undefined' && window.location ? window.location.hostname : ''))
        .trim()
        .toLowerCase()
        .replace(/^\[|\]$/g, '');
    if (!h) return false;
    if (h === 'localhost' || h === '0.0.0.0' || h === '::1') return true;
    // 127.0.0.0/8
    const m = /^127\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(h);
    if (m && [m[1], m[2], m[3]].every(p => Number(p) <= 255)) return true;
    return false;
}

/** True when hostname is loopback or a private/LAN address. */
export function isLocalOperatorHost(hostname?: string): boolean {
    const h = (hostname ?? (typeof window !== 'undefined' && window.location ? window.location.hostname : ''))
        .trim()
        .toLowerCase();
    if (!h) return false;
    if (isLoopbackHost(h)) return true;
    if (h === '0.0.0.0' || h === '::1' || h === '[::1]') {
        return true;
    }
    if (isPrivateIPv4(h)) return true;
    if (isPrivateIPv6(h)) return true;
    return false;
}
