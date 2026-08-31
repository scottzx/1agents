# Third-Party Attributions

This repository contains material derived from external open-source projects.

The transformed artifacts live in `backend/internal/agent/skills/*` and
`backend/internal/agent/roles/*`: upstream `SKILL.md` files converted into our
format and embedded into the binary. Each file records its own origin in
frontmatter (`source` / `license` / `upstream_sha`), which is the authoritative
attribution record.

These artifacts are now **vendored and frozen**. The absorption pipeline that
generated them (`cmd/absorb-upstream` + `internal/agent/absorb.go`, #188 / RFC §5)
has been removed, along with its ledger, so the repo no longer reads from any
upstream checkout at build or run time. Updating an absorbed file is a manual
edit; keep its frontmatter accurate if you do.

## Upstream Projects

| Project | Path | License | Upstream | Absorbed as |
|---|---|---|---|---|
| **superpowers** (Jesse Vincent / obra) | vendored (no checkout) | MIT | https://github.com/obra/superpowers | skills (process methodologies) |
| **gstack** (Garry Tan) | vendored (no checkout) | MIT | https://github.com/garrytan/gstack | role (`cso`) + skill (`design-shotgun`) |
| **HarnessKit** (RealZST) | `modules/HarnessKit` | Apache-2.0 | https://github.com/RealZST/HarnessKit | controlled product fork for extension management |

No part of the build reads superpowers or gstack any more. Untracked reading
copies may sit under `reference_repo/`, but nothing depends on them being there,
and their MIT terms are carried by the frontmatter of each derived file.

HarnessKit ships its full license text with the submodule; its fork baseline,
patch ownership, upstream sync policy, and protected-artwork exclusions are
documented in `modules/HarnessKit/UPSTREAM.md` and
`modules/HarnessKit/ASSET-LICENSES.md`.

## Notes

- gstack command `SKILL.md` files are auto-generated and partly code-backed (shell
  preambles, `/browse` Chromium tooling). Code-backed capabilities were **not**
  vendored — they are meant to be remapped onto our own browser/computer-use
  abilities (RFC §10). Only a curated, format-clean subset was absorbed.
- To take a newer upstream version, clone it somewhere outside the repo, port the
  change by hand, and update the file's `upstream_sha` frontmatter.
