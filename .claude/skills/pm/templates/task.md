# Task 模板

> 建 `type=task` 条目时使用（**会被派给执行 agent**，必须填 `--acceptance`）。
> 拆自某条 requirement / bug，**先有源条目再建 task**。

---

## 空骨架（复制后填空）

```
实现 #<源 requirement/bug 编号>：<一句话说明要做什么>。

## 目标
- 一句话讲清这个 task 的产出。

## 实现要点
- 文件 / 接口 / 数据结构 / 命令行 flag 提示（**只列锚点，不写实现细节**——细节交给执行 agent）。
- 与其他模块的契约（输入 / 输出 / 副作用）。

## 关联
- 依赖：`#<前置 task id>`（用 `--json '{"dependsOn":["..."]}'` 挂；先建被依赖项拿到 id）。
- 归口：源 requirement / bug 用 `#编号` 引用即可（自动建 relates 关系）。

## 验收标准（**必填**，否则 task 进不了队列）
- [ ] …（可观察的产物 / 行为 / 测试命令）
- [ ] …（边界 case / 错误处理）
- [ ] …（向后兼容 / 不退化）
```

---

## 调用

```
project-items create --type task \
  --title "..." \
  --description "<整段>" \
  --acceptance "<整段，独立于 description>" \
  --milestone "v0.1" \
  --json '{"dependsOn":["<前置id>"]}'
```

## 拆 task 的尺寸

- 一个 task = 一次提交 / 一个 PR / 一次完整验证。
- 超过 ~200 行实现代码的，先问自己"是不是该拆成两个"。
- 跨多个独立模块的，必须拆。