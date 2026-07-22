# happy-cli fork 同步 runbook

> 配套:[集成骨架](happy-integration-skeleton.md) · [agent 收敛路线图](agent-convergence-roadmap.md)

## 关系

`modules/happy-cli` 是 git submodule,作为**只读蓝图 + 库**:

| remote | URL | 用途 |
|---|---|---|
| `origin` | `github.com/scottzx/happy-cli`(我们的 fork) | submodule 跟踪点 |
| `upstream` | `github.com/slopus/happy`(happy 上游) | 同步源 |

**铁律:1agents_app 的胶水全在 `adapter/`,从不改 `modules/happy-cli/` 源码。** 这是同步便宜的前提。

## 为什么同步便宜

因为我们零改动 submodule 源码,同步就是直接合上游,**理想零冲突**。唯二的耦合点:

1. **`ctx` 契约形状** —— happy-cli `src/modules/common/loadRpcAdapter.ts` 注入给
   `register(ctx)` 的对象。由 [`adapter/rpc/ctxContract.d.ts`](../../adapter/rpc/ctxContract.d.ts) 钉住。
2. **Tier-1 导出面** —— `agent/core`、`agent/transport`、`agent/acp/{AcpBackend,AcpSessionManager}`
   等被 `adapter/agent/` 当库消费的模块的导出签名(M2/M3 才相关)。

## 同步步骤

```bash
cd modules/happy-cli
git fetch upstream
git merge upstream/main          # 理想零冲突;有冲突说明上游动了我们消费的接口面
# 解决(若有)→ 推到我们的 fork
git push origin main

# 回 1agents_app 记录新的 submodule 指针
cd ../..
git add modules/happy-cli
git commit -m "chore(happy-cli): sync upstream slopus/happy"
```

## 同步后复核清单

- [ ] `ctx` 契约:对比 happy-cli `loadRpcAdapter.ts` 注入对象 vs `adapter/rpc/ctxContract.d.ts`,
      字段/签名有变则同步更新 `.d.ts` 并检查 `adapter/rpc`、`adapter/chat` 用法。
- [ ] Tier-1 导出面(若 `adapter/agent/` 已实现):确认消费的导出未改名/改签名。
- [ ] Spike B parity:`HAPPY_RPC_ADAPTER_ENTRY` 指向 `adapter/rpc/index.mjs`,跑一遍 relay 聊天不回归。
- [ ] `git submodule status modules/happy-cli` 指针已更新并提交。

## 首次拉取(新克隆)

```bash
git submodule update --init --recursive modules/happy-cli
cd modules/happy-cli && git remote get-url upstream \
  || git remote add upstream https://github.com/slopus/happy.git
```
