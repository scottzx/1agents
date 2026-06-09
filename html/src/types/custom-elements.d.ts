/**
 * Type declarations for module-side custom elements.
 *
 * The 1agents host renders `<cc-connect-panel>` and `<skills-panel>` as
 * plain JSX. They are defined at runtime by ESM modules served from
 * /api/embed/* (see `html/src/template.html`). Without this file
 * TypeScript treats the tags as unknown IntrinsicElements and refuses
 * to compile.
 *
 * The declared attributes mirror the observed attribute list inside
 * each module's `embed.tsx`; keep both in sync.
 */
import 'preact';

declare module 'preact' {
    namespace JSX {
        interface IntrinsicElements {
            'cc-connect-panel': HTMLAttributes<HTMLElement> & {
                id?: string;
                'auth-token'?: string;
                'server-url'?: string;
                route?: string;
                theme?: 'light' | 'dark' | 'system';
                lang?: string;
                style?: string | Record<string, string | number>;
            };
            'skills-panel': HTMLAttributes<HTMLElement> & {
                id?: string;
                route?: string;
                theme?: 'light' | 'dark';
                lang?: string;
                style?: string | Record<string, string | number>;
            };
        }
    }
}
