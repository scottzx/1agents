# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Workspace.kind = `tmp`（单次/临时对话）** — 扩展原 `workforce | project` 两档：
  - 创建 ephemeral 会话时 mint 真实 `projects` 行：`id=tmp-<sessionId>`，`kind=tmp`，`path=/tmp/1agents-chat/...`（seed `.grok/config.toml` + `AGENTS.md` + `Claude.md`）。
  - 有真实 WorkspaceId + pwd；UI 隐藏路径（面包屑显示「单次对话」）。
  - **任务区** = workforce ∪ tmp；**项目区**仅 `kind=project`。
  - 副 pane（文件/任务）绑定该 tmp workspace 的 path，不再误用上一个项目。
  - 前端 picker 哨兵仍为 `oneshot`（提交时转 create tmp）。

### Changed

- **Terminology migration (Epic #184 / 名称定义表 §0)** — align storage, API types, UI kinds, and docs with the glossary cutover:
  - Meta DB table `tasks` → **`project_items`** (idempotent Open migration; no dual-read of the old table name).
  - Go/TS discriminator **`ItemType`** (`task` | `requirement` | `bug` | `discussion`); `task` is only an enum value. Primary entity name **`ProjectItem`** (Go `Task` / frontend service names remain as transitional aliases).
  - Workspace `projects.kind`: **`workforce` | `project`**. Legacy `assistant` is remapped on Open and on write; Chinese UI still shows **「助理」**.
  - Creation wizard tiers: **助理 / 项目 / `template_project`** (not persisted as kind). Historical `professional-project` / `generic-project` are no longer wizard wire ids.
  - **executor × assignee** matrix validated in North Task API (`agent` / `function` / `human`); `function` mirrors `FunctionType` onto `assignee` and keeps `fn:<type>` labels. Field name stays `executor` (not AIWorkforce).
  - Docs (agentsOS, issue-model, project-model, app-sdk-contract, README) point at 名称定义表 §0; obsolete “task may leave project” claims are rejected.

### Notes

- Chat message kind `assistant_text` is unchanged.
- HTTP paths remain `/api/agent/project-items`; CLI remains `1agents project-items`.
- Package name `taskapi` is intentionally retained for this cutover.

### Changed (M6 follow-up · #197 / #198)

- **Go/TS true names**: `ProjectItem` is now the struct definition site; `Task` / `TaskType` remain deprecated aliases with removal target after M6 call-site migration.
- **Single executor matrix**: `meta.NormalizeExecutorAssignment` is the shared entry for `taskapi.DispatchTask` and HTTP POST/PATCH `/api/agent/project-items` (illegal combos return 4xx).
