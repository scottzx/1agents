// Command seed-roadmap writes this PM tool's own development roadmap (P0–P7,
// with parent containers, subtasks and cross-phase dependencies) into the
// live ~/.1agents/meta.db, under the existing "1agents" project.
//
// It is a one-shot dogfood seed used to validate that the task/dependency
// data model can faithfully express a real project plan, and to give the new
// Kanban/Overview frontend real data to render. Auto-execution is left blank:
// every not-yet-done task is scheduled far in the future (2099) so the
// scheduler's own trigger-time gate skips it — no agent is ever dispatched.
//
// Run once from the backend dir:  go run ./cmd/seed-roadmap
// It is safe to re-run (whole-config replace under the project).
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

const projectID = "1agents"

// far is the sentinel "do not auto-run" trigger time. The scheduler skips any
// task whose trigger time is after now, so this keeps seeded tasks inert.
var far = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

// base anchors created_at so the tree keeps its intended order (ORDER BY
// created_at, id). Each task bumps the cursor by a minute.
var base = time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)
var cursor int

func at() time.Time {
	t := base.Add(time.Duration(cursor) * time.Minute)
	cursor++
	return t
}

// task builds one task. Pending tasks are pinned to the far-future trigger so
// the live scheduler never dispatches them; completed tasks carry CompletedAt.
func task(id, title, milestone, parent string, status meta.TaskStatus, prio meta.Priority, deps []string, labels []string, desc string) meta.Task {
	created := at()
	t := meta.Task{
		ID:          id,
		Title:       title,
		Description: desc,
		IssueState:  meta.IssueOpen,
		Status:      status,
		Priority:    prio,
		Milestone:   milestone,
		ParentID:    parent,
		DependsOn:   deps,
		Labels:      labels,
		CreatedBy:   "user",
		MaxRetries:  1,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	if t.DependsOn == nil {
		t.DependsOn = []string{}
	}
	switch status {
	case meta.TaskStatusCompleted:
		t.ScheduleType = meta.ScheduleTypeImmediate
		c := created.Add(30 * time.Minute)
		t.CompletedAt = &c
		t.StartedAt = &created
	default:
		// Park everything not done in the far future so it stays inert.
		t.ScheduleType = meta.ScheduleTypeScheduled
		f := far
		t.ScheduledAt = &f
	}
	return t
}

func main() {
	db, err := meta.OpenDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open meta.db: %v\n", err)
		os.Exit(1)
	}
	proj, ok, err := db.GetProject(projectID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup project %q: %v\n", projectID, err)
		os.Exit(1)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "project %q not found in meta.db\n", projectID)
		os.Exit(1)
	}
	// Use the project's stored workspace_path verbatim so Save resolves to the
	// existing row (exact-string match) instead of creating a duplicate.
	wsPath := proj.WorkspacePath

	const (
		mP0 = "P0 规划与决策"
		mP1 = "P1 看板+总览"
		mP2 = "P2 依赖接力"
		mP3 = "P3 验收闭环"
		mP4 = "P4 IM 卡片"
		mP5 = "P5 需求池+里程碑分组"
		mP6 = "P6 AI 项目经理"
		mP7 = "P7 时间轴+成本+日报"
	)

	C := meta.TaskStatusCompleted
	P := meta.TaskStatusPending

	tasks := []meta.Task{
		// ── P0 规划与决策（已完成：本对话产出）──
		task("p0", "P0 规划与决策", mP0, "", C, meta.PriorityMedium, nil, []string{"roadmap", "planning"},
			"把 1agents 升格为轻量化项目管理工具的功能取舍、数据模型与 Phase 1 设计。本对话产出。"),
		task("p0-tradeoff", "功能取舍与优先级", mP0, "p0", C, meta.PriorityMedium, nil, []string{"planning"},
			"判断标准：只保留“因为执行者是 AI 才成立”的功能；砍掉人类团队版 Jira 的重功能（甘特关键路径/延期预测/知识库）。"),
		task("p0-model", "数据模型决策", mP0, "p0", C, meta.PriorityMedium, nil, []string{"planning"},
			"需求池=GitHub 思路统一 tasks 表加 type 字段；里程碑=字符串字段+分组视图；Sprint 休眠留空不删。"),
		task("p0-design", "Phase 1 详细设计", mP0, "p0", C, meta.PriorityMedium, nil, []string{"planning"},
			"landing 内 列表/看板/总览 切换器；seed 路线图入 meta.db；自动执行靠远未来 scheduledAt 留空。"),

		// ── P1 看板+总览（本批构建）──
		task("p1", "P1 看板视图 + 总览页", mP1, "", P, meta.PriorityHigh, []string{"p0"}, []string{"roadmap", "frontend"},
			"在 tasks landing 内加视图切换器，新增看板与总览两个可视化视图，渲染本路线图数据。"),
		task("p1-switcher", "landing 视图切换器", mP1, "p1", P, meta.PriorityHigh, nil, []string{"frontend"},
			"TaskList/index.tsx 加 useSignal<'table'|'board'|'overview'>，顶部分段控件，三视图共享同一份 tasks。"),
		task("p1-kanban", "KanbanBoard 组件", mP1, "p1", P, meta.PriorityHigh, []string{"p1-switcher"}, []string{"frontend"},
			"status→列分组（待办/进行中/阻塞/已完成/失败取消），卡片复用现有徽章，拖拽复用 LeftSidebar 原生 DnD。"),
		task("p1-overview", "Overview 总览页", mP1, "p1", P, meta.PriorityHigh, []string{"p1-switcher"}, []string{"frontend"},
			"纯 SVG 完成率环 + 统计卡 + 按 milestone 分组进度条 + 临近 deadline，Bento 布局。"),

		// ── 后续阶段（容器任务，依赖前置阶段）──
		task("p2", "P2 依赖即时接力 + blocked 态", mP2, "", P, meta.PriorityHigh, []string{"p1"}, []string{"roadmap", "backend"},
			"任务完成时即时推进被依赖任务，并补全 blocked 状态管理。scheduler 已基本实现，补“即时+阻塞态”。"),
		task("p3", "P3 Dev→Review 验收闭环", mP3, "", P, meta.PriorityMedium, []string{"p2"}, []string{"roadmap", "backend"},
			"让 AcceptanceCriteria 真正有约束力：开发 agent 完成后由 review agent 按验收标准打回/通过。"),
		task("p4", "P4 cc-connect 交互卡片", mP4, "", P, meta.PriorityMedium, []string{"p1"}, []string{"roadmap", "backend"},
			"任务状态变化时向飞书/Slack 推交互卡片，用户一键批准/打回反馈回工作区。"),
		task("p5", "P5 需求池 + 里程碑分组视图", mP5, "", P, meta.PriorityMedium, []string{"p1"}, []string{"roadmap", "fullstack"},
			"tasks 表加 type 字段（issue/bug/需求/task）做需求池；里程碑分组门户。漏斗：需求→里程碑→任务。"),
		task("p6", "P6 AI 项目经理", mP6, "", P, meta.PriorityHigh, []string{"p1", "p5"}, []string{"roadmap", "agent"},
			"对话式 AI PM：系统提示词 + 任务 CRUD(by ID) 工具（锁定 project_id）+ 应用内聊天入口；吸收 AI 拆解。本批 seed 即其写入路径的手动原型。"),
		task("p7", "P7 时间轴 + Token 成本 + 日报", mP7, "", P, meta.PriorityLow, []string{"p6"}, []string{"roadmap", "later"},
			"时间轴可视化（不做关键路径）、按任务/agent 的 Token 成本展示与图表、git+任务状态自动日报。后置。"),
	}

	// A few open-ended requirement/bug cards to populate the 需求池 (type != task).
	reqCard := func(id, title string, typ meta.TaskType, prio meta.Priority, desc string) meta.Task {
		t := task(id, title, "", "", P, prio, nil, []string{"需求池"}, desc)
		t.Type = typ
		return t
	}
	tasks = append(tasks,
		reqCard("req-template", "需求：支持任务模板，一键生成常用任务组", meta.TaskTypeRequirement, meta.PriorityMedium,
			"把常见的任务组合存成模板，新项目一键铺开整套带依赖的任务。"),
		reqCard("req-mobile-gesture", "需求：移动端看板支持手势拖拽", meta.TaskTypeRequirement, meta.PriorityLow,
			"移动端看板目前只能点击，希望支持长按拖拽改状态。"),
		reqCard("bug-safari-dnd", "缺陷：看板拖拽在 Safari 偶发失效", meta.TaskTypeBug, meta.PriorityHigh,
			"Safari 下偶发 dragend 不触发，卡片卡在半拖状态需刷新。"),
	)

	store := meta.NewTaskStore(db)
	if err := store.Save(wsPath, &meta.TasksConfig{Tasks: tasks}); err != nil {
		fmt.Fprintf(os.Stderr, "save tasks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("seeded %d roadmap tasks into project %q (%s)\n", len(tasks), projectID, wsPath)
}
