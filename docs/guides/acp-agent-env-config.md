# ACP Agent 环境变量配置与排错手册

> 适用对象：所有通过 ACP（Agent Client Protocol）接入的 coding agent——claude / codex / pi / cursor / gemini / opencode / qwen / kimi / kilocode 等（参见 `modules/1acp/src/agent-registry.ts`）。
>
> 适用范围：cc-connect daemon → 1acp bridge-server → agent subprocess 这条链路上**所有** env var 的读取、设置、诊断。
>
> 默认约定：**Anthropic 兼容端点统一用 `ANTHROPIC_AUTH_TOKEN` + `ANTHROPIC_BASE_URL`**（Bearer 模式，与 Claude Code 同源）。其他协议端点（OpenAI 兼容 / Google / Azure）见末尾"按协议速查"。

---

## 1. 为什么需要这份文档

我们已经在 1agents 栈里踩过同一类坑至少两次：

- ANTHROPIC_API_KEY 设了，pi 子进程说"Authentication required"——因为 pi 上游 provider 读的是 `ANTHROPIC_AUTH_TOKEN`（或者反过来，claude code 用 Bearer，1acp bridge 启动时设的也是 Bearer）。
- `~/.pi/agent/models.json` 里写了 `apiKey: "$ANTHROPIC_API_KEY"`，但 bridge-server 进程里只有 `ANTHROPIC_AUTH_TOKEN`——pi 用 `$` 插值时找不到这个 env。
- `[projects.agent.options.env]` 写了 KEY，但 agent 是 1acp 路径启动的，cc-connect 注入的 env 在 bridge-server 那一层就断了。

每次有新的 ACP agent 加进来（registry 现在有 18 个），同样的问题都会以不同形式复发。这份文档把"env var 到底在哪一层、怎么读、怎么设、断了怎么查"固化成一份共用知识。

---

## 2. 4 层模型：env var 的藏身处

一个 env var 从"你想设它"到"agent 子进程拿到它"，中间可能经过 4 层。每一层都可能设、覆盖、清空、或干脆不传。

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: shell rc / daemon 启动环境                          │
│   ~/.zshrc · ~/.bashrc · systemd unit · launchd plist         │
│   → 1acp bridge-server 进程 / cc-connect daemon 进程启动时   │
│     的 process.env                                          │
├─────────────────────────────────────────────────────────────┤
│ Layer 2: 1acp bridge-server 内部                             │
│   modules/1acp/src/acp/auth-env.ts:118                       │
│   buildAgentEnvironment(): { ...process.env }                 │
│   → 复制 process.env 到 spawn env,再叠加 authCredentials     │
│   → 也可以叠加 sessionEnv(per-session 注入)                  │
├─────────────────────────────────────────────────────────────┤
│ Layer 3: cc-connect provider proxy                          │
│   modules/cc-connect/agent/claudecode/claudecode.go:1113     │
│   providerEnvLocked() 动态拼 env(K=V 数组)                   │
│   → 只对实现了 ProviderSwitcher interface 的 agent 生效     │
│     (claudecode/codex/opencode/...;PI 不实现)               │
├─────────────────────────────────────────────────────────────┤
│ Layer 4: agent 上游自己的 config                            │
│   ~/.pi/agent/models.json (apiKey 字段,支持 $VAR 插值)       │
│   ~/.claude/settings.json · ~/.codex/config.toml · ...       │
│   → agent 进程启动后再读这一层,优先级最高                   │
└─────────────────────────────────────────────────────────────┘
```

### 关键事实

- **Layer 1 → 2**：bridge-server 启动时 `process.env` 是什么，agent 子进程就继承什么（`auth-env.ts:123` 的 `{ ...process.env }`）。所以**在 shell rc 里 `export` 的 KEY，bridge-server 重启前看不到**——必须重启 bridge-server 才会被读。
- **Layer 2 → 3**：cc-connect 走 1acp 路径时**完全管不到** agent 子进程的 env——它只跟 bridge-server 走 WebSocket。所以**`[projects.agent.options.env]` 对 1acp_claude / 1acp_pi 等路径无效**。
- **Layer 3 → 4**：provider 注入的 env 在 agent 进程启动前；如果 agent 上游又从自己的 config 读了同名 var，**agent config 赢**。

---

## 3. 默认约定（Anthropic 兼容）

| 字段 | env var | 取值示例 |
|---|---|---|
| 端点 URL | `ANTHROPIC_BASE_URL` | `https://api.minimaxi.com/anthropic` |
| 鉴权 token | `ANTHROPIC_AUTH_TOKEN` | `sk-cp-S-v9...`（Bearer，不要 `sk-ant-` 前缀） |
| 模型名 | `ANTHROPIC_MODEL` | `MiniMax-M3` |
| Haiku 别名 | `ANTHROPIC_DEFAULT_HAIKU_MODEL` | `MiniMax-M3` |
| Sonnet 别名 | `ANTHROPIC_DEFAULT_SONNET_MODEL` | `MiniMax-M3` |
| Opus 别名 | `ANTHROPIC_DEFAULT_OPUS_MODEL` | `MiniMax-M3` |

### 为什么默认用 AUTH_TOKEN（Bearer）而不是 API_KEY

Claude Code 上游启动时若 env 里有 `ANTHROPIC_API_KEY`，会**主动 ping `api.anthropic.com` 校验**——对第三方端点会 hang。`ANTHROPIC_AUTH_TOKEN` 是 Bearer，Claude Code 不会去校验，开箱即用。

各 agent 对这两个 var 的支持情况见 §7。

---

## 4. 怎么读：诊断"现在 env 里到底有什么"

按层从外到内查：

```bash
# Layer 1: 当前 shell
printenv | grep -iE "anthropic|openai|api_key|auth_token"

# Layer 1 + bridge-server 进程:用 ps eww 看 daemon 启动时的 env
ps eww -p $(pgrep -f bridge-server | head -1) | tr ' ' '\n' | grep -iE "anthropic|api_key|auth_token"

# Layer 2: 1acp 把什么 env 传给 agent 子进程
# 看 auth-env.ts 的逻辑; 实际传的内容可以通过临时 wrapper 探测:
strace -f -e trace=execve -p <agent_pid> 2>&1 | grep -A20 "envp"

# Layer 3: cc-connect 是否注入了(K=V 形式)
# 没法直接看 providerEnvLocked() 输出; 走 §6 决策树用 agent 自己暴露的能力诊断

# Layer 4: agent 上游 config
cat ~/.pi/agent/models.json
cat ~/.pi/agent/settings.json
cat ~/.claude/settings.json 2>/dev/null
cat ~/.codex/config.toml 2>/dev/null
```

**Tip**：把 `ps eww` 的输出重定向到文件而不是 stdout——env var 的真值会进 shell history。

---

## 5. 怎么写：在哪一层设

### 场景 A：所有 agent 都要用同一个 KEY

放 **Layer 1**，bridge-server 启动环境里 export 一次。**最干净、跨 agent 复用**。

```bash
# ~/.zshrc
export ANTHROPIC_AUTH_TOKEN='sk-cp-S-v9...'
export ANTHROPIC_BASE_URL='https://api.minimaxi.com/anthropic'
export ANTHROPIC_MODEL='MiniMax-M3'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='MiniMax-M3'
export ANTHROPIC_DEFAULT_SONNET_MODEL='MiniMax-M3'
export ANTHROPIC_DEFAULT_OPUS_MODEL='MiniMax-M3'

# 然后重启 bridge-server
kill <bridge-server-pid> && (cd modules/1acp && npm exec tsx bridge-server.js &)
```

### 场景 B：单一项目覆写（不走 1acp 的 agent）

cc-connect 的 `[projects.agent.options.env]` 只对**直接 exec** 子进程的 agent 生效（PI agent 即如此；`modules/cc-connect/agent/pi/session.go:126-132` 会 merge options.env 到 pi 子进程 env）。

```toml
# ~/.cc-connect/config.toml
[[projects]]
name = "minimax-pi"

[projects.agent]
type = "pi"  # 直接 exec,不走 1acp

[projects.agent.options]
work_dir = "/path/to/repo"

[projects.agent.options.env]
ANTHROPIC_BASE_URL = "https://api.minimaxi.com/anthropic"
ANTHROPIC_AUTH_TOKEN = "sk-cp-S-v9..."  # daemon 进程内读得到
```

### 场景 C：单一 agent 覆写，且该 agent 支持自己的 config

放到 agent 上游的 config 里。pi 的 `~/.pi/agent/models.json` 支持 `$VAR` 插值从 env 读，claude code 用 `~/.claude/settings.json` 的 `env` 字段等。

```json
// ~/.pi/agent/models.json:覆盖内置 anthropic provider
{
  "providers": {
    "anthropic": {
      "baseUrl": "https://api.minimaxi.com/anthropic",
      "apiKey": "$ANTHROPIC_AUTH_TOKEN",  // ← 关键:pi 用 $ 插值从 env 读
      "models": [{ "id": "MiniMax-M3" }]
    }
  }
}
```

```json
// ~/.claude/settings.json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-cp-S-v9...",
    "ANTHROPIC_BASE_URL": "https://api.minimaxi.com/anthropic"
  }
}
```

---

## 6. 决策树："X 没到 Y" 怎么查

```
X env var 没在 agent 子进程里生效
│
├─ 1. X 是哪一类?
│   ├─ ANTHROPIC_* / OPENAI_* / GEMINI_* → 继续
│   └─ 自定义 KEY → 直接跳到第 3 步
│
├─ 2. agent 走哪条路径启动?
│   ├─ cc-connect 直接 exec (type="pi"/"claudecode"等)
│   │   → 看 [projects.agent.options.env] 或 daemon process env
│   ├─ 1acp 路径 (type="1acp_claude" 等)
│   │   → 看 bridge-server 启动时的 process.env
│   └─ 其它(系统 systemd / launchd)
│       → 看 daemon unit 文件 / plist
│
├─ 3. var 名对上吗?
│   ├─ bridge-server / daemon 上有 ANTHROPIC_AUTH_TOKEN
│   │   但 agent config 里写 $ANTHROPIC_API_KEY → 改 agent config 改 $VAR
│   ├─ daemon 上有 ANTHROPIC_API_KEY
│   │   但 agent 读 ANTHROPIC_AUTH_TOKEN → 加 export ANTHROPIC_AUTH_TOKEN
│   └─ var 名一致 → 继续
│
├─ 4. agent 上游有没有自己的 config 覆盖?
│   ├─ pi: ~/.pi/agent/models.json(apiKey 字段)+ settings.json(defaultProvider)
│   ├─ claude: ~/.claude/settings.json(env 字段)
│   ├─ codex: ~/.codex/config.toml
│   └─ 检查 agent 是否有 "managed by host" 之类的开关
│       (claude code: CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=1)
│
└─ 5. 最小验证:
    agent subprocess 启动后立即 printenv > /tmp/agent-env.log
    (改 agent 的启动 wrapper 或用 strace execve 抓 envp)
    对照"应有"vs"实际",缺的 var 就是问题点
```

---

## 7. 按 agent 速查：哪个 var 是它真正读的

> 数据来源：pi docs (`pi.dev/docs/latest/{providers,models}`) + 各 agent 源码约定。**校验优先以 agent 自己的文档为准**——本表会过时。

| Agent | 协议 env var | Token env var | 上游 config 路径 | 备注 |
|---|---|---|---|---|
| **claude / claudecode** | `ANTHROPIC_BASE_URL` | `ANTHROPIC_AUTH_TOKEN`（Bearer 推荐） | `~/.claude/settings.json` `env` 字段 | 用 `API_KEY` 时会去 ping api.anthropic.com |
| **codex** | `OPENAI_BASE_URL` 或 codex 自定义 | `OPENAI_API_KEY` | `~/.codex/config.toml` | codex_config.wire_api: `responses` 或 `chat` |
| **pi** | `ANTHROPIC_BASE_URL` (走 anthropic provider) | `ANTHROPIC_API_KEY` 或 `ANTHROPIC_AUTH_TOKEN` | `~/.pi/agent/models.json` | pi 的 `$VAR` 插值读 env,**写哪个 var 引用就用哪个** |
| **cursor** | (cursor 自己管理) | (OAuth 优先) | `~/.cursor/` | 第三方端点支持有限 |
| **gemini** | `GOOGLE_GEMINI_BASE_URL` | `GEMINI_API_KEY` | `~/.gemini/` | Vertex AI 走 `GOOGLE_CLOUD_PROJECT` |
| **opencode** | provider-specific | provider-specific | `~/.config/opencode/config.json` | 通用 provider schema |

### pi 的特殊坑

- pi 的 `piSettings` 结构只读 `defaultProvider` / `defaultModel` / `enabledModels`（`modules/cc-connect/agent/pi/pi.go:374-378`），**不读 provider URL/KEY**——URL/KEY 必须在 `models.json` 里覆盖内置 provider，或者上游 pi CLI 通过 env 注入。
- pi 的 `models.json` 里 `apiKey: "$VAR"` 这种 `$` 引用对**大小写敏感**，env 里也得是大写。
- pi 不认 `ANTHROPIC_BASE_URL` 的小写变体 (`anthropic_base_url`)。

---

## 8. 1acp bridge-server 怎么传 env 给 agent

参考 `modules/1acp/src/acp/auth-env.ts:118-145`：

```typescript
function buildAgentEnvironment(
  authCredentials: Record<string, string> | undefined,
  sessionEnv: Record<string, string> | undefined,
  includeClaudeSettings: boolean,
): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { ...process.env };   // ← 关键:复制 bridge-server 的 env
  if (includeClaudeSettings) {
    applyClaudeSettingsEnvironment(env);                  // claude code 的特殊处理
  }
  const protectedAuthEnvKeys = promotePrefixedAuthEnvironment(env);
  if (authCredentials) {
    for (const [methodId, credential] of Object.entries(authCredentials)) {
      addAuthCredentialEnvKeys(protectedAuthEnvKeys, methodId, credential);
      assignAuthCredentialEnv(env, methodId, credential);   // 注入 authCredentials
    }
  }
  if (sessionEnv) {
    for (const [key, value] of Object.entries(sessionEnv)) {
      // sessionEnv 不会覆盖 auth credentials
      if (typeof value !== "string" || protectedAuthEnvKeys.has(...)) continue;
      assignSessionEnv(env, key, value);
    }
  }
  return env;
}
```

要点：
- **bridge-server 的 `process.env` 永远先复制**（line 123）——这是 Layer 1 → Layer 2 的入口。
- `authCredentials` 是 1acp 自己的"托管凭据"机制，由 registry 配置或运行时指定。
- `sessionEnv` 是 per-session 注入（来自 WebSocket 消息），但**不能覆盖 auth credentials**（line 137）。

---

## 9. cc-connect provider 注入的 env（Layer 3）

参考 `modules/cc-connect/agent/claudecode/claudecode.go:1113-1180`：

```go
func (a *Agent) providerEnvLocked() []string {
    // ...
    if p.BaseURL != "" {
        env = append(env, "ANTHROPIC_BASE_URL="+p.BaseURL)
    }
    if p.APIKey != "" {
        env = append(env, "ANTHROPIC_AUTH_TOKEN="+p.APIKey)
        env = append(env, "ANTHROPIC_API_KEY=")   // ← 显式清空,防止 Claude Code 去校验官方域名
    }
    if p.Model != "" {
        env = append(env, "ANTHROPIC_MODEL="+p.Model)
    }
    // ...
    for k, v := range p.Env {
        env = append(env, k+"="+v)                  // provider 自己的 env map
    }
    return env
}
```

要点：
- **只对实现了 `ProviderSwitcher` interface 的 agent 生效**（claudecode/codex/opencode/...）。**PI 没实现**——PI agent 只能靠 `[projects.agent.options.env]`。
- `API_KEY=""` 这一行是 Claude Code 兼容技巧——清空 API_KEY 让 Claude Code 走 Bearer 分支。
- 多个 provider 之间切换时，`activeIdx` 决定用哪个；运行时换 provider 会改 env。

---

## 10. 常见坑（Top 5）

| # | 现象 | 根因 | 修法 |
|---|---|---|---|
| 1 | "Authentication required" | env var 名不匹配（API_KEY vs AUTH_TOKEN） | §7 速查表 |
| 2 | bridge-server 重启后新 env 没生效 | bridge-server `process.env` 是启动时冻结的 | 改完 shell rc 必须 `kill` bridge-server 重启 |
| 3 | `[projects.agent.options.env]` 设了 KEY，agent 还是报 401 | agent 走 1acp 路径，cc-connect 管不到 | 改 Layer 1（bridge-server 启动 env） |
| 4 | pi 的 `models.json` 写了 `apiKey: "$VAR"` 仍然 401 | env 里实际 var 名大小写不一致 | `printenv | grep -i VAR` 对照 |
| 5 | Claude Code hang 在启动阶段 | daemon 上有 `ANTHROPIC_API_KEY` 触发 api.anthropic.com 校验 | 改用 `ANTHROPIC_AUTH_TOKEN`（Bearer） |

---

## 11. 安全须知

- **永远不要把 KEY 真值写到 git 跟踪的文件里**——`config.toml` / `models.json` / `settings.json` 用 `$VAR` 插值或 `${ENV_VAR}` 引用，KEY 只在 daemon 启动环境里。
- shell history 会捕获 `export KEY=...`——用 `HISTFILE=/dev/null export KEY=...` 或写到一个 `chmod 600` 的 env file 里再 `source`。
- `ps eww` / `ps auxe` / `printenv` / 日志都可能泄漏 KEY——**调试完立刻 rotate**。
- 1acp 的 `authCredentials` 走 `assignAuthCredentialEnv`，但 env 在 bridge-server 内存里也是明文——bridge-server 进程 dump 就能看到。

---

## 12. 相关源码索引

| 关注点 | 文件 | 行号 |
|---|---|---|
| 1acp env 复制 | `modules/1acp/src/acp/auth-env.ts` | 118-145 |
| 1acp agent spawn | `modules/1acp/src/acp/client.ts` | 823-840 |
| 1acp agent registry | `modules/1acp/src/agent-registry.ts` | 43-104 |
| cc-connect PI agent | `modules/cc-connect/agent/pi/session.go` | 126-132 |
| cc-connect PI configEnv | `modules/cc-connect/core/cmdopts.go` | 56-80 |
| cc-connect claudecode providerEnv | `modules/cc-connect/agent/claudecode/claudecode.go` | 1113-1180 |
| cc-connect TOML env 解析 | `modules/cc-connect/config/config.go` | 693 (`resolveEnvInConfig`) |
| pi 上游 env 约定 | `pi.dev/docs/latest/providers` | — |
| pi models.json schema | `pi.dev/docs/latest/models` | — |
