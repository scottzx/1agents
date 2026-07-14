# Bug 模板

> 建 `type=bug` 条目时使用。整段填好后作为 `--description` 传入。
> 结构固定便于以后检索 / 对照 / 复用 acceptance。

---

## 空骨架（复制后填空）

```
## 现象
- 用户/系统能看到什么错的现象（视觉、行为、报错、报错码）。
- 截图 / 日志 / 关键栈帧粘这里。

## 重现
- 最小输入 / 操作步骤（粘贴代码 / 数据 / URL / 操作序列）。
- 期望：…
- 实际：…

## 根因（可选，但强烈建议）
- 直接原因（一两句话）。
- 影响范围：涉及的文件 / 接口 / 调用路径（**多文件多路径要列全**，避免执行 agent 只改一处遗漏）。

## 验收标准
- [ ] 现象中的错误 case 修复后满足：…
- [ ] 正常用法不退化：…
- [ ] 边界 / 异常 case 不退化：…

## 修复方向（可选，给执行 agent 参考，不是死规定）
- 思路 / 建议路径。执行 agent 可提更好的方案；这里只是锚定范围。
```

---

## 填好的实例：Markdown 渲染误识别 YAML frontmatter

`type=bug`，`priority=high`，`issueState=open`：

```
## 现象

任务卡片 / README 等 Markdown 文档中**任意两段连续的 `---`** 都会被解析器当成 YAML frontmatter，导致：

- 中间的 Markdown 内容（最常见是 `# Header`）被吞进 "frontmatter" 块，渲染成 YAML 高亮的注释，header 视觉消失。
- body 被截断。

YAML frontmatter 按规范只允许出现在文档**开头**。当前实现没有校验这一点。

## 重现 1（最直接）

输入：
---
# Title
---

body content here
```
预期：识别为「分隔线 + H1 + 分隔线 + 段落」，body 应是 `# Title\n\nbody content here`。
实际（`frontend/src/utils/markdown.ts` 的 `frontmatterExtension`，已实测）：输出 `<div class="md-yaml-block md-yaml-frontmatter">…# Title…</div><p>body content here</p>`，`# Title` 被当 frontmatter 吃掉。

## 重现 2（结构性）

输入：
---
# Title
---

body content here
```
`frontend/src/utils/frontmatter.ts` 的 `splitFrontmatter` 行为：
- `fm = "# Title"`（错，应为空）
- `body = "body content here"`（错，丢了 `# Title` 和它与正文之间的空行）

## 根因

`---` 在 Markdown 里身兼两职：YAML frontmatter 闭合符 **和** thematic break（分割线）。当前 3 处实现都只看 "开头是不是 `---`" 和 "下一个 `---` 在哪"，**从不校验两者之间是不是 YAML 结构**（`key: value`、列表项、块标量）。所以任何两个 `---` 行都会被错配成 frontmatter 对。

## 影响范围（3 处实现同样有 bug）

1. `frontend/src/utils/frontmatter.ts:17-36` `splitFrontmatter` — 控制 acceptance 提取和 body 拆分（TaskDetail 编辑器、RequirementPool 卡片预览都依赖它）。
2. `frontend/src/utils/markdown.ts:172-190` `frontmatterExtension`（marked tokenizer，正则 `^(?:---\r?\n)([\s\S]*?)(?:\r?\n---)(?=\r?\n|$)/`）— 控制 HTML 渲染产物（被 `markdown.worker.ts`、`TaskDetail.tsx`、`renderMarkdown()` 调用）。
3. `backend/internal/meta/frontmatter.go:88-106` `SplitFrontmatter` — Go 镜像，注释明确写了 "mirrors this exactly"，必须一起改以保持前端/后端解析一致（避免渲染/落库分叉）。

## 验收标准

修复后必须同时满足：

1. 上面「重现 1」「重现 2」两个 case 解析为 `fm = ""`，body 完整保留为 `# Title\n\nbody content here`，HTML 渲染出 `<h1>Title</h1><p>body content here</p>`（带正常 thematic break）。
2. 正常的 frontmatter 用例不退化：acceptance 列仍能被正确抽出。
   - 输入 `---nacceptance: foon---nnbody` → `acceptance = ["foo"]`，`body = "body"`。
3. frontmatter 内含 `|`/`>` 块标量、列表项、空行 的复杂用例不退化。
4. 3 处实现（TS 两处 + Go 一处）解析结果一致；新增或更新对应的单测覆盖以上 case。
5. README / 长文档里常见的 "开头用 `---` 当装饰分隔线" 的写法不再被破坏（在前端预览、详情面板、worker 渲染三条路径下都正确）。

## 修复方向（仅供参考，不是死规定）

判定 closing fence 时，除了 "以 `---` 开头" 之外，再校验两者之间的内容**形似 YAML**（只含 `key: value` 顶层映射、缩进的列表项、`|`/`>` 块标量标记、空行；不含 `#` 起始行、`- ` 起首项以外的缩进段落等明显的 Markdown 形状）。不满足则把候选 `---` 当 thematic break，继续向后扫描下一个 `---`。
```

> 调用：
> ```
> project-items create --type bug --priority high \
>   --title "Markdown 渲染误把任意两段 --- 识别为 YAML frontmatter（吞掉 header / 把分割线当 frontmatter）" \
>   --description "<上面整段>"
> ```