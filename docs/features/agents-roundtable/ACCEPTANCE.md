# Agents 圆桌 · design.md §7 验收

**切片:** #251 切片 7 / 任务 #258；布局补丁 #260 / #263  
**真源:** [design.md §7](./design.md#7-acceptance-ship-criteria) · [design.md §6 UI](./design.md#6-ui)  
**自动化:** `TestE2E_Smoke_DesignSection7` + 相关 unit tests  
**手工:** 下方 UI 清单（入口、过程折叠、**底栏裁判嵌入 Chat**）

---

## 1. 一键跑自动化（可复现）

在仓库根目录：

```bash
# 推荐：验收脚本（后端 §7 E2E + 注册表 + 前端 stage 单元）
./scripts/roundtable-acceptance.sh

# 或逐步：
cd backend && go test ./internal/roundtable/ -count=1 -run 'TestE2E_Smoke_DesignSection7|TestAgentsRoundtableManifestRegistered'
cd frontend && npx --yes tsx --test src/components/roundtable/stage.test.ts
```

期望：全部 PASS；`TestE2E_Smoke_DesignSection7` 日志中出现 `§7.1`…`§7.8 PASS`。  
§7.9 / 布局 L1–L5 为**前端手工**（见 §3b），不在上述 Go E2E 内。

> 自动化使用 `StaticSeatPrompter`，**不依赖** 真 1acp / 真 Grok 二进制。  
> 真 agent 端到端（可选）见 §4。

---

## 2. design.md §7 对照（含 #260 布局）

| # | 验收项 | 自动化 | 手工 UI |
|---|--------|--------|---------|
| 1 | 发现中心 → 应用（或更多）能打开圆桌并创建 room | Manifest `agents-roundtable` 注册 + `CreateRoom` | 见 §3 步骤 1–2 |
| 2 | 一局 **6** 个 Grok Build session（1 裁判 + 5 职能） | R2 后 6×`session_id` + chat index | 侧栏可见 6 会话名 |
| 3 | R1：与裁判多轮后确认 Brief（输入走底栏裁判 Composer） | 2×chat + `ConfirmBrief` → `waiting_r2` | 步骤 3–4 |
| 4 | R2：五席**隔离**各一条发言 + 裁判 Summary₂ | 隔离注入审计 + 5 speech + Summary₂ | 步骤 5 |
| 5 | R3：各席 **resume**；上下文含 R2 全文 + Summary₂；Summary₃ | 同 `acp_session_id` + 公开包 + `done` | 步骤 6 |
| 6 | 主 UI **默认只见正文**；过程可折叠 | turn `content_text` 绑定；前端 `TurnCard` 默认 `open=false` | 步骤 7 |
| 7 | 刷新后 room 可恢复；未结束席位可继续 resume | `GetRoom` 恢复 state/brief/turns/acp | 步骤 8 |
| 8 | Summary 能区分职能来源；研发与产品可区分 | Summary₃ 含「产品」「研发」；seed 契约不同 | 读终稿正文 |
| 9 | **布局（#260）**：底栏=裁判嵌入 Chat；时间线不嵌裁判；无简易 `chatText` 底栏 | 见 §3b（前端布局） | 步骤 3、§3b |

---

## 3. 手工清单（入口 → 终稿）

前置：

1. 使用**含 roundtable 路由**的后端二进制（源码 `server.go` 注册 `/api/roundtable/rooms`）。若 `POST /api/roundtable/rooms` 返回 404，请 `make backend` 后重启 daemon。
2. 本地已起 `1agents`（含 frontend dist）与 1acp bridge。默认监听见 `~/.1agents/daemon.json`（常见 `127.0.0.1:38080`）。

| 步骤 | 操作 | 期望 |
|------|------|------|
| 1 入口 | 打开发现中心 → **应用**（或「更多应用」）→ 卡片 **Agents 圆桌** | 进入启动向导；展示固定 6 席编制 |
| 2 建房 | 填可选议题草稿 → **开始** | 创建 room，`state=drafting_brief`，进入 R1 |
| 3 R1 对话 | 在**底部固定裁判嵌入 Chat**的 Composer 中与裁判多轮澄清（至少 2 轮） | 底栏可见历史+实时流+typing；时间线可出现用户/裁判 `chat` 正文摘要；**无**旧版简易 `chatText` 底栏；**时间线不再**嵌第二份裁判 Chat 卡 |
| 4 确认 Brief | 点确认 Brief / 开始圆桌，填 title/question/constraints/success_criteria | `state=waiting_r2`；侧栏显示 Brief |
| 5 R2 | 触发 R2（开始首轮） | 五职能各 1 条发言 + 裁判 Summary₂；席位条 done/error；裁判流仍在底栏（不在时间线再嵌裁判卡） |
| 6 R3 | 触发 R3（次轮/终稿） | 各席 resume 后再发；终稿 Summary₃；`state=done` |
| 7 正文 UI | 浏览时间线；有 process 的卡片点「查看过程」 | **默认**仅 `content_text`；过程折叠；展开后可见 process_ref |
| 8 刷新恢复 | 浏览器刷新（或重开应用） | 回到同一 room（localStorage `oneagents.roundtable.activeRoomId`）；Brief/turns/总结仍在；底栏裁判会话可恢复 |

### 3b. 布局清单（#260 · design §6）

> **一句话：** 底栏 = 裁判嵌入 Chat；时间线不嵌裁判。  
> **与 #256：** #256 = 时间线壳（阶段条 / 席位条 / turn 卡 / 侧栏）；#260 = 壳底部固定裁判会话。

| # | 验收项 | 期望 |
|---|--------|------|
| L1 | 底栏固定 ChatUI 嵌入区 | 绑定裁判 `seat.session_id`；历史 + 实时流 + typing；sticky/fixed，不随时间线滚走 |
| L2 | 时间线 / 席位区无裁判第二份嵌入卡 | 裁判 speaking 时**只**在底栏出现嵌入 Chat，不在 turn/speaking 卡再嵌一份 |
| L3 | 无旧版自定义简易底栏 | 无 `chatText` 纯文本 + 简易 send；R1 输入走底栏裁判 Composer |
| L4 | 复用 Chat 嵌入组件 | 使用 #261 `EmbeddedChat`（或等价 MessageList + typing + Composer），非自绘气泡 |
| L5 | 职能席（可选 / 正交） | panelist 实时可在时间线各自嵌入；**绝不**占底栏；P0 以裁判底栏为准 |

**实现对照（代码路径，便于自查）：**

| 区域 | 组件 / 约定 |
|------|-------------|
| 时间线壳 | `RoundtableRoom` + `StageBar` / `SeatBar` / `TurnCard`（#256） |
| 底栏裁判 | 主栏底部 sticky `EmbeddedChat`，`sessionId = referee.session_id`，R1 `readOnly=false` |
| 禁止 | 时间线对 `role=referee` 再渲染 `SpeakingSeatCard` / 第二份嵌入；`rt-composer` + `chatText` 简易底栏 |

---

### API 等价路径（无 UI 时）

```bash
BASE=http://127.0.0.1:38080

# 建房
ROOM=$(curl -sS -X POST "$BASE/api/roundtable/rooms" \
  -H 'Content-Type: application/json' \
  -d '{"title":"§7 手工冒烟"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
echo "room=$ROOM"

# R1 chat（需真 bridge 时才有真实模型回复）
curl -sS -X POST "$BASE/api/roundtable/rooms/$ROOM/chat" \
  -H 'Content-Type: application/json' \
  -d '{"text":"议题：两周验证协作工具 PMF"}' | head -c 400; echo

# 确认 Brief
curl -sS -X POST "$BASE/api/roundtable/rooms/$ROOM/brief" \
  -H 'Content-Type: application/json' \
  -d '{"title":"协作 PMF","question":"如何两周验证？","constraints":"3人","success_criteria":"5种子用户"}'

# R2 / R3（真 agent，耗时与费用取决于 harness）
curl -sS -X POST "$BASE/api/roundtable/rooms/$ROOM/r2"
curl -sS -X POST "$BASE/api/roundtable/rooms/$ROOM/r3"

# 刷新等价：GET
curl -sS "$BASE/api/roundtable/rooms/$ROOM" | python3 -m json.tool | head -80
```

---

## 4. 真 agent 可选冒烟（非门禁）

当需要验证真实 1acp resume / Grok Build 进程时：

1. 确认 `ACPX_PORT`（或 daemon 配置）指向运行中的 bridge。
2. 走 §3 手工或 §3 API 路径。
3. 在 R2 后检查 6 个 chat session 的 `agent_type=grok-build`。
4. R3 后比对各席 `acp_session_id` 与 R2 一致（GET seats / room）。

失败时：把 HTTP 状态、room id、state、failed_roles 记入任务评论。

---

## 5. 失败点记录模板

```
§7.N FAIL: <简述>
证据: <测试名 / HTTP 码 / 日志摘录 / 截图路径>
影响: 阻塞 / 不阻塞
```

任务 #258 评论应粘贴完整 §7 条 PASS/FAIL 表。  
任务 #260 / #263：另附 §3b 布局 L1–L5 PASS/FAIL（与 design §6「底栏=裁判嵌入 Chat，时间线不嵌裁判」一致）。
