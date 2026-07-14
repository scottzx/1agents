# Requirement 模板

> 建 `type=requirement` 条目时使用（issueState=open，目标清晰、有明确交付物）。
> 与 bug 不同：requirement 是「要做成什么样」，bug 是「哪里错了」。

---

## 空骨架（复制后填空）

```
## 背景
- 为什么要做这件事。痛点 / 机会 / 用户场景。
- 关联：上游需求 #N / 文档链接 / 数据点。

## 目标
- 用 1-3 句话说清楚「做完后世界长什么样」。

## 范围（做什么）
- 必须包含的功能 / 行为 / 接口。

## 不做（明确范围外）
- 这次明确不做的相邻事项。**写下来避免 scope creep**。

## 验收标准（顶层；子任务的 acceptance 在拆任务时再细化）
- [ ] …（可观察、可检验的产物 / 行为）

## 备注
- 设计取舍 / 风险 / 依赖 / 关联需求。
```

---

## 调用

```
project-items create --type requirement \
  --title "..." \
  --description "<整段>"
```

> 注意：`--acceptance` 是**可执行 task** 才必填的。requirement 的顶层 acceptance 写在 description 的「验收标准」段里，子任务的 acceptance 在后续拆任务时逐条细化。