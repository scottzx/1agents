# Third-Party Attributions

This repository absorbs material from external open-source projects via the
absorption pipeline (#188, RFC §5: `docs/features/inbox-context-engine/design.md`).

Two tracks:

- **Track A — read-only upstream reference** (`modules/*`, pinned submodules): the
  upstream source is vendored as a git submodule at a fixed SHA, carries its own
  `LICENSE`, and is used only to diff/pull updates. It is never modified here.
- **Track B — transformed artifacts** (`backend/internal/agent/skills/*`,
  `backend/internal/agent/roles/*`): upstream `SKILL.md` files transformed into
  our format. Each transformed file records its origin in frontmatter
  (`source` / `license` / `upstream_sha`); the per-item ledger is
  `backend/internal/agent/.absorbed.json`.

## Upstream Projects

| Project | Submodule | License | Upstream | Absorbed as |
|---|---|---|---|---|
| **superpowers** (Jesse Vincent / obra) | `modules/superpowers` | MIT | https://github.com/obra/superpowers | skills (process methodologies) |
| **gstack** (Garry Tan) | `modules/gstack` | MIT | https://github.com/garrytan/gstack | role (`cso`) + skill (`design-shotgun`) |
| **HarnessKit** (RealZST) | `modules/HarnessKit` | Apache-2.0 | https://github.com/RealZST/HarnessKit | controlled product fork for extension management |

The full license texts ship with the respective submodules. HarnessKit's fork
baseline, patch ownership, upstream sync policy, and protected-artwork exclusions
are documented in `modules/HarnessKit/UPSTREAM.md` and
`modules/HarnessKit/ASSET-LICENSES.md`.

## Notes

- gstack command `SKILL.md` files are auto-generated and partly code-backed (shell
  preambles, `/browse` Chromium tooling). Code-backed capabilities are **not**
  vendored — they are meant to be remapped onto our own browser/computer-use
  abilities (RFC §10). Only a curated, format-clean subset is absorbed; see the
  manifest in `backend/internal/agent/absorb.go` (`DefaultAbsorbManifest`).
- To pull upstream updates: `git submodule update --remote modules/superpowers
  modules/gstack`, then re-run `go run ./cmd/absorb-upstream` from `backend/`. Only
  items whose transformed output changed are rewritten (incremental, ledger-backed).
