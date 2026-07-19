# @1agents/web

Frontend dist for 1agents. Served via `-static`.

Includes `dist/embed/skills-embed.js` and `dist/embed/cc-connect-embed.js` —
self-contained ESM custom elements (`<skills-panel>`, `<cc-connect-panel>`)
loaded by the host at `/api/embed/*`. Built by `scripts/build-module-embeds.sh`.
