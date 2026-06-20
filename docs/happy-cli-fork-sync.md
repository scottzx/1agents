# happy-cli Fork 与上游同步施工图

> 把 happy-cli 抽成独立 fork 仓 `scottzx/happy-cli`，作为 submodule 挂进 1agents_app（C2 电脑客户端），
> 为 adapter（TS sidecar）提供 transport + E2E 加密蓝图，并与上游 `slopus/happy` 保持同步。
>
> 关联文档：[open-vs-closed-boundary](../../1agents_server/docs/open-vs-closed-boundary.md)、
> CSC 架构（C1 手机 happy-app ↔ S 服务器 happy-server ↔ C2 电脑 1agents_app）。

---

## 1. 为什么这么做（决策记录）

| 选项 | 结论 | 理由 |
|---|---|---|
| adapter 放哪 | **C2 / 1agents_app** | adapter 是电脑端加密端点，不是服务器代码；放服务器仓 = 角色错位 |
| 怎么拿 happy-cli | **fork 独立 repo + submodule** | 与现有 cc-connect/1skills/1acp 四件套同构，零学习成本 |
| 为什么不发 npm | **submodule 维护更小** | happy-cli 是胖应用、单一私有消费者；npm 的"一处发布多处消费"回报为零，却要付全部打包税 |
| npm 留给谁 | **只给 wire** | 真契约 / 多消费者 / 要公开 → 已发 `@1agents/wire` |

口诀：**同步用 git（subtree split + submodule），分发才用 npm。**

---

## 2. 难点：fork 的是「monorepo 的子目录」，不是整个 repo

| | 1acp / cc-connect | happy-cli |
|---|---|---|
| upstream 形态 | 独立 repo | `slopus/happy` 里的子目录 `packages/happy-cli` |
| 同步动作 | `git fetch upstream && git merge` | **先 `git subtree split` 切出子目录历史**，再 merge |

`git subtree split` 对同一段上游历史是**确定性**的（合成 commit 哈希稳定），所以每次 re-split 都能 **fast-forward**，这就是可持续 merge 的前提。
> ⚠️ 不要用 `git filter-repo` 做持续同步——它每次重写 SHA、无法 merge，只适合一次性抽取。

---

## 3. 仓库布局（两条分支）

`scottzx/happy-cli`：
```
upstream  ← 纯净 split 结果，只从 slopus/happy 切出来，永不手改
main      ← fork = upstream + 极少量补丁（见 §5），submodule 指向它
```
同步 = `re-split → 推 upstream 分支 → 把 upstream merge 进 main`。冲突只可能落在打过补丁的边界文件上。

---

## 4. 操作步骤

### 4a. 一次性抽取
```bash
# 在一个 slopus/happy 的克隆里
git clone https://github.com/slopus/happy.git happy-mono && cd happy-mono
git subtree split -P packages/happy-cli -b cli-split
git push git@github.com:scottzx/happy-cli.git cli-split:upstream
# 在 scottzx/happy-cli：从 upstream 拉出 main，打 §5 的补丁并提交
```

### 4b. 挂成 submodule（沿用现有 modules/ 惯例）
```bash
cd 1agents_app
git submodule add https://github.com/scottzx/happy-cli.git modules/happy-cli
git commit -m "add happy-cli blueprint submodule"
```

### 4c. 持续同步（上游更新后）
```bash
cd happy-mono && git pull
git subtree split -P packages/happy-cli -b cli-split          # 再切，FF 前进
git push git@github.com:scottzx/happy-cli.git cli-split:upstream
# scottzx/happy-cli：git checkout main && git merge upstream
# 1agents_app：cd modules/happy-cli && git pull && cd ../.. && git add modules/happy-cli && git commit
```
> 自动化要点：保留**常驻的 happy-mono 克隆**（或用 `subtree split --rejoin`），让 split 缓存命中、哈希稳定。

---

## 5. 唯一躲不掉的补丁：用 npm alias 脱离 workspace（源码零改动）

happy-cli 在 monorepo 里用 `workspace:*` 引依赖；独立 build 必须改成真实版本。
**采用 npm alias**：让 package.json 里的 `@slopus/happy-wire` 直接指向已发布的 `@1agents/wire`，
这样 **17 处源码 import（`from '@slopus/happy-wire'`）一行都不用动**，补丁只落在 package.json 一个文件：

```diff
  "dependencies": {
-   "@slopus/happy-wire": "workspace:*"
+   "@slopus/happy-wire": "npm:@1agents/wire@^0.1.0"
  }
```
> `npm:<pkg>@<range>` 是 npm/pnpm 的官方 alias 语法：包名仍叫 `@slopus/happy-wire`，
> 但实际安装 `@1agents/wire`。源码 `import ... from '@slopus/happy-wire'` 照常解析。

**铁律（决定每次 merge 成本）：transport 源码一行不改，补丁只在 package.json 这一行 alias。**
cc-connect→SessionEnvelope 的胶水写在 1agents_app 的 adapter 里（`modules/happy-cli` 之外），永不进 fork。
补丁越少，merge 越便宜——理想情况每次 upstream 合并**零冲突**（除非上游也动了这一行依赖）。

---

## 6. 定时同步 Action 草稿（放 scottzx/happy-cli，**待过目，勿直接部署**）

> 🔒 **对上游 `slopus/happy` 全程只读**：只 `git clone`（读）来做 subtree split，
> **绝不向 slopus/happy 推送、开 issue 或开 PR**。所有写操作（push `upstream` 分支、开 PR）
> 都只发生在你自己的 fork `scottzx/happy-cli` 内部。

```yaml
# .github/workflows/sync-upstream.yml  —— 运行在 scottzx/happy-cli
name: Sync upstream slopus/happy (happy-cli subtree)
on:
  schedule: [{ cron: '0 18 * * *' }]   # 每天一次
  workflow_dispatch:
permissions:
  contents: write          # 仅本仓 scottzx/happy-cli
  pull-requests: write     # 仅本仓 scottzx/happy-cli
jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }                 # 需要完整历史做 merge
      - name: Split upstream happy-cli (READ-ONLY clone of slopus/happy)
        run: |
          # 只读克隆上游，不加 remote、不推送、不开 PR
          git clone --no-tags https://github.com/slopus/happy.git /tmp/mono
          git -C /tmp/mono subtree split -P packages/happy-cli -b cli-split
      - name: Update local upstream branch (in THIS fork only)
        run: |
          git fetch /tmp/mono cli-split:upstream || git branch -f upstream FETCH_HEAD
          git push origin upstream --force-with-lease     # origin = scottzx/happy-cli
      - name: Open PR upstream -> main (in THIS fork only)
        env: { GH_TOKEN: '${{ secrets.GITHUB_TOKEN }}' }  # 默认 token 仅限本仓，无法写 slopus/happy
        run: |
          git checkout main
          git merge --no-edit upstream || true     # 冲突则留给人解
          if ! git diff --quiet HEAD origin/main; then
            BR="sync/upstream-$(git rev-parse --short upstream)"
            git checkout -b "$BR" && git push origin "$BR"
            gh pr create --base main --head "$BR" \
              --repo "$GITHUB_REPOSITORY" \
              --title "Sync upstream slopus/happy" \
              --body "Automated subtree split of packages/happy-cli. Resolve conflicts (likely only the package.json alias §5)." || true
          fi
```
你只在 PR 有冲突时介入。注意 PR 的 `--base main --repo $GITHUB_REPOSITORY` 锁死在 `scottzx/happy-cli`，
不会误指向上游。

---

## 7. 不是锁死

将来若出现**第二个消费者**（如 1agents_mini 也要这套 transport），或范围收窄到「只用 transport 5 个目录、几乎不动」，
可再把它抽成 `@1agents/transport` 发 npm——**先 submodule、后发包**，与 wire 同路，不冲突。

> 涉及的 transport 目录（adapter 真正消费的）：`src/api/`、`src/agent/core/`、`src/agent/transport/`、
> `src/agent/acp/`、`src/modules/common/`。
