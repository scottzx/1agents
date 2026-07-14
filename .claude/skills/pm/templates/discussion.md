# Discussion 模板

> 建 `type=discussion` 条目时使用（落讨论区，**不会被调度**，纯记录用）。
> 用在「方向还在想、还没明确交付物」的时候。清晰化后**升级成 requirement 或 bug**。

---

## 空骨架（复制后填空）

```
## 议题
- 一句话讲清在讨论什么。

## 已有信息
- 已观察到的事实 / 数据 / 引用（链接 + 关键摘录）。
- 不要凭空补「我觉得会怎样」。

## 待讨论的角度
- 角度 A：…（附支持 / 反对方）
- 角度 B：…（附支持 / 反对方）

## 开放问题
- 必须回答才能往下走的具体问题（不要泛泛「怎么做好」）。

## 决策方向（可选，仅当讨论已收敛时填）
- 倾向：…
- 代价 / 取舍：…
```

---

## 调用

```
project-items discussion \
  --title "..." \
  --description "<整段>"
```

## 与 requirement / bug 的边界

- 没明确交付物 → discussion。
- 明确交付物 + 没错误 → requirement。
- 明确要修的错 → bug。

discussion 收尾时如果明确成 requirement / bug，用 `--type` update 升级（不影响 issueState）。