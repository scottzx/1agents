# Turn 生命周期性能测试报告

**测试日期**：2026-07-29

**测试环境**：macOS，本机 1agents + 1acp Bridge，SQLite WAL

**测试目标**：确认引入持久化 Turn 生命周期后，是否造成 Session 启动或 Prompt 首字时间的明显回归，并使用真实 Codex 和 Grok 作为对照组。

## 1. 结论

新增 Turn 生命周期确实增加了同步工作，但当前开销只有毫秒级，不是 Session 启动或真实模型首字变慢的主要原因。

- Session 启动的 `ensure_session → session_ready` 路径不创建 Turn，因此没有新增 Turn 数据库事务。
- Prompt 进入执行前新增 `Create(queued)` 和 `Transition(running)` 两次 SQLite 事务。
- 生产数据中，Turn 从创建到 running 的 p50 为 2.01ms，p95 为 4.99ms。
- 真实 Codex/Grok 的首字耗时为数秒，约比 Turn 落库开销高三个数量级。
- Terminal 后历史加载约为 1–9ms，不阻塞首字。
- Grok 预热命中时，`session_ready` 可缩短至 11–16ms；Agent 预热和模型响应才是用户感知延迟的主要变量。

因此，当前结果判定为：

> Turn 带来了轻微且合理的性能损耗，但没有造成用户可感知的 Session 启动变慢，也不是当前 Grok/Codex 首字慢的原因。

## 2. 测试口径

测试将链路拆成三个独立阶段：

1. **Session 启动**
   - WebSocket connect
   - `ensure_session`
   - `session_ready`
2. **Turn 启动**
   - 发送 Prompt
   - 创建 queued Turn
   - 转换为 running
   - 收到首个事件和首个 `text_delta`
3. **Turn 结束与持久化**
   - 收到 `turn_terminal`
   - 请求 `get_history`
   - 校验历史数量和 Turn ID

真实 Agent 对照使用相同条件：

- 同一个 1acp Bridge
- 同一个工作目录
- 相同 Prompt：只回复 `OK`，不调用工具
- 相同 host-managed `turnId/requestId`
- 相同 `turn_terminal → get_history → delete_session` 流程
- 唯一变量为 `agentType`：`codex` 或 `grok-build`

## 3. Turn 数据库路径

Prompt 到 Agent 的前置路径为：

```text
浏览器发送 Prompt
  → 1agents 创建 queued Turn
  → SQLite transaction commit
  → queued → running
  → SQLite transaction commit
  → 注入 turnId/requestId
  → 转发给 1acp
  → 1acp 保存 Prompt + Turn snapshot
  → Agent 开始处理
```

`AgentTurnStore.Create` 事务执行：

1. 验证 Session 存在且 Project 匹配。
2. 按 `session_id + client_request_id` 做幂等查询。
3. 插入 `agent_turns`。
4. 插入生命周期记录。
5. 提交事务。

`AgentTurnStore.Transition` 事务执行：

1. 查询 Turn 当前状态。
2. 校验 `queued → running`。
3. 更新 Turn。
4. 插入生命周期记录。
5. 提交事务。

这两次事务是 Prompt 执行前最明确的新增同步成本。

## 4. 生产 Turn 时序

生产数据库共统计到 19 条 Turn，其中 18 条为立即开始执行的样本。

使用 `created_at → started_at` 估算创建并转换为 running 的耗时：

| 指标 | 时间 |
|---|---:|
| min | 0.97ms |
| p50 | 2.01ms |
| avg | 2.17ms |
| p95 | 4.99ms |
| max | 4.99ms |

SQLite 配置：

- `journal_mode=WAL`
- `synchronous=NORMAL`
- `wal_autocheckpoint=1000`

`created_at` 在部分校验和幂等查询之后才设置，因此完整的 Turn 前置成本会略高于上表，但仍然处于个位数毫秒，而不是秒级。

## 5. Mock Agent 基准

使用无真实模型网络和推理延迟的 mock ACP Agent，连续运行 10 个隔离 Session。

| 指标 | min | p50 | avg | max |
|---|---:|---:|---:|---:|
| WebSocket connect | 1.19ms | 3.34ms | 6.58ms | 35.26ms |
| `ensure_session → session_ready` | 202.90ms | 226.04ms | 230.19ms | 252.21ms |
| Prompt → 首字 | 2.14ms | 2.58ms | 3.10ms | 5.56ms |
| Prompt → Terminal | 1004.57ms | 1014.22ms | 1015.08ms | 1020.08ms |
| Terminal → History | 0.64ms | 1.16ms | 1.91ms | 4.79ms |

所有样本的历史条数均为 2。

Prompt 到 Terminal 约 1 秒是 runtime 已存在的 post-success drain，不是新增 Turn SQLite 延迟。去掉真实模型后，Prompt 到首字只有约 2–6ms，与生产 Turn 创建到 running 的统计一致。

## 6. Codex 对照组

### 6.1 同一 Session 连续两轮

| 阶段 | 第一轮 | 第二轮，同 Session |
|---|---:|---:|
| Session 创建到 `session_ready` | 5.91s | 无需重新创建 |
| Prompt 到首个 Turn 事件 | 115.93ms | 2.11s |
| Prompt 到首字 `OK` | 5.76s | 2.11s |
| Prompt 到 `turn_terminal` | 7.34s | 3.38s |
| Terminal 后历史读取 | 2.00ms | 5.41ms |
| 历史条数 | 2 | 4 |
| `turnId == runtimeRequestId` | 是 | 是 |
| 状态 | completed | completed |

Codex 第二轮明显更快，说明 Session/Agent 复用有效。

### 6.2 三个独立 Session

| 样本 | `session_ready` | 首字 | Terminal | Terminal 后历史读取 |
|---|---:|---:|---:|---:|
| Codex 1 | 8.85s | 6.40s | 7.94s | 3.36ms |
| Codex 2 | 2.03s | 7.30s | 8.36s | 8.65ms |
| Codex 3 | 2.38s | 10.26s | 11.48s | 2.44ms |

包含首个双 Turn Session 在内，Codex 首轮首字区间为 5.76–10.26s。即使 Session 在约 2 秒 ready，模型首字仍可能需要 6–10 秒，说明主要耗时不在 Turn SQLite 操作。

## 7. Grok 对照组

### 7.1 预热命中，同一 Session 连续两轮

| 阶段 | 第一轮 | 第二轮，同 Session |
|---|---:|---:|
| `session_ready` | 11.34ms | 无需重新创建 |
| Prompt 到首字 `OK` | 7.64s | 8.60s |
| Prompt 到 Terminal | 8.65s | 9.95s |
| Terminal 后历史读取 | 2.12ms | 1.68ms |
| 历史条数 | 2 | 4 |
| `turnId == runtimeRequestId` | 是 | 是 |
| 状态 | completed | completed |

另一个预热命中样本：

- `session_ready`：16.43ms
- Prompt 到首字：约 14.25s
- Prompt 到 Terminal：约 15.47s
- Terminal 后历史读取：约 2.55ms
- 历史：2 条
- `turnId == runtimeRequestId`

预热将 Agent Session 初始化缩短到了 11–16ms，但模型首字仍需要 7–14 秒。

### 7.2 超时离群样本

第一次真实 Grok 测试中：

- Grok 进程/Session 初始化约 4.4 秒完成。
- Prompt 已写入 SessionRecord。
- 180 秒内没有收到 Assistant 文本或 Terminal。
- 后续两个 Grok Session 样本均成功。

这个离群值不能归因于 Turn 落库，因为 Prompt 和 Turn 已进入运行阶段。它更可能来自 Grok 模型请求、网络、Agent 内部任务或终态交付的瞬时稳定性问题。

当前样本量不足以计算可靠失败率，但应该把 Grok 超时作为独立稳定性指标持续监控，而不是与 Turn 落库延迟混合统计。

## 8. 1acp 持久化分析

Happy path 下，Turn snapshot 没有额外增加一轮 Prompt 前 SessionRecord 保存。

1acp 原本就会在 Prompt 发出前：

1. 调用 `recordPromptSubmission`。
2. 更新 SessionRecord。
3. 调用 `sessionStore.save(record)`。

引入 Turn 后，只是把 User message ID 和 `acpx.turn_results[requestId]` 合并写入原有保存。

Terminal 时会再次保存最终 Turn snapshot，但它位于 Turn 结束阶段，不阻塞首字。

新增成本主要是：

- JSON 对象中增加一个 `turn_results` 条目。
- 原有 JSON 文件保存时多序列化这部分数据。

而不是额外增加一次 Prompt 前同步写盘。

## 9. 性能预算建议

| 路径 | 当前结果 | 建议预算 |
|---|---:|---:|
| Turn 创建并进入 running | p95 ≈ 5ms | p95 < 10ms |
| Terminal 后历史可见 | 1–9ms | p95 < 20ms |
| Grok 预热命中到 ready | 11–16ms | p95 < 100ms |
| Agent 冷启动 | 2–9s | 单独统计 |
| 模型首字 | 2–14s，另有一次 Grok 超时 | 单独统计延迟与稳定性 |

建议增加以下埋点：

- `ws_prompt_received_at`
- `turn_created_at`
- `turn_running_at`
- `prompt_forwarded_at`
- `first_text_delta_at`
- `turn_terminal_received_at`
- `turn_terminal_persisted_at`

这些时间点可以直接区分数据库、Bridge、Agent 和模型四类延迟。

## 10. 长期风险

`acpx.turn_results` 会随着 Session Turn 数量增长，而 SessionRecord 当前是整份 JSON 重写：

```text
每次保存成本 ≈ O(Session 历史大小 + Turn snapshot 数量)
```

原有 `messages` 已经随历史增长，因此 Turn snapshot 只是增加了一份较小索引。短期不是问题；若未来存在几百或几千轮的超长 Session，应补充：

- 100 / 1,000 / 10,000 Turn 的序列化基准
- Session JSON 文件大小监控
- `sessionStore.save` p95/p99
- 将 `turn_results` 拆成 append-only 存储的收益评估

目前不建议为了节省 2–5ms，直接合并 `Create(queued)` 和 `Transition(running)`。两阶段设计保留了幂等、排队、生命周期审计和崩溃恢复的明确语义，其价值高于个位数毫秒。

## 11. 测试清理

测试使用独立的 `benchmark-*` Session ID。测试结束后：

- 所有 benchmark Session 均已关闭。
- 7 个测试 Session JSON 已按精确路径删除。
- 未修改或删除用户 Session。
- 未保留 benchmark 事件流文件。
