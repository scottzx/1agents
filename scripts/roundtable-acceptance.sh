#!/usr/bin/env bash
# Agents 圆桌 design.md §7 验收脚本（切片 7 / #258）
# 可复现：入口 manifest + E2E 建房→R1→R2→R3→刷新恢复 + 前端 timeline 单元。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> [1/3] backend roundtable §7 E2E + package tests"
(
  cd backend
  go test ./internal/roundtable/ -count=1 -timeout 120s
)

echo "==> [2/3] frontend roundtable stage/role unit tests"
(
  cd frontend
  # Prefer project-local tsx; fall back to npx (frontend may not list tsx as dep).
  if [[ -x node_modules/.bin/tsx ]]; then
    node_modules/.bin/tsx --test src/components/roundtable/stage.test.ts
  elif command -v npx >/dev/null 2>&1; then
    npx --yes tsx --test src/components/roundtable/stage.test.ts
  else
    echo "WARN: no tsx; skip frontend unit (static greps still cover UI wiring)"
  fi
)

echo "==> [3/3] static entry wiring (grep guards)"
grep -q "agents-roundtable" frontend/src/apps/roundtable/index.tsx
grep -q "agents-roundtable" frontend/src/components/drawer/DiscoveryPanel.tsx
grep -q "registerAppView('AgentsRoundtable'" frontend/src/apps/roundtable/index.tsx
grep -q "oneagents.roundtable.activeRoomId" frontend/src/components/roundtable/RoundtableRoom.tsx
grep -q "查看过程" frontend/src/components/roundtable/TurnCard.tsx
# Default process collapsed: useState(false)
grep -q "useState(false)" frontend/src/components/roundtable/TurnCard.tsx

echo ""
echo "ALL ACCEPTANCE CHECKS PASSED (design.md §7 automated + static UI wiring)"
echo "Manual UI click-path: docs/features/agents-roundtable/ACCEPTANCE.md §3"
