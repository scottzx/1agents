// Package research is the Inbox 自动调研管线 (#190, RFC §3.1 L2 / §7 C4).
//
// It wires three already-built pieces into one automatic loop, adding only the
// orchestration:
//
//		pull 源 (RSS/榜单)  ──► L1 ingest ──► 命中深挖规则? ──► L2 深度调研 ──► Discussion 卡草稿
//		   Source.Fetch         kwiki.Ingest    DeepDiveRule       Browser           ResearchResult
//
//	  - L0/L1：拉到的条目原样落 kwiki raw/，自动分类(domain)+摘要+标签 (复用
//	    kwiki.Ingest，本包不重写知识压缩)。
//	  - L2：只对命中 DeepDiveRule 的条目触发二次调研。调研能力来自 Browser
//	    接口——这是 gstack /browse 的「重映射」：本仓不 vendor 其 Chromium 代码，
//	    而是抽象成一个浏览/抓取接口，实时网络抓取由实现方注入。包内自带一个
//	    StubBrowser 占位实现，保证离线可测 (RFC §5 code-backed 技能不 vendor)。
//	  - L3：深挖产物渲染成一张 Discussion 卡草稿 (ResearchResult.Card)，由调用方
//	    落进 Discussion/建议卡表 (#189/#47 拥有该表，本包不直接写库)。
//
// 配额 (RFC §9.2 Open Question)：DeepDiveBudget 限制单次 Run 的 L2 调用次数，
// 避免命中规则的条目过多导致调研成本失控。
//
// 本包不新造调度器：把 Run 挂到既有 scheduler 的定时回调上即可。
package research

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/kwiki"
)

// Item is one pulled piece of external context before it enters the wiki. It is
// the pull-源 → 管线 的传输单元；落 kwiki 时映射成 kwiki.InboxItem。
type Item struct {
	ID    string // stable id from the source (used as the wiki slug seed)
	Title string
	Text  string // raw captured content
	URL   string // origin link, if any (carried into deep research)
	// Domain routes the wiki page (work/market/personal…). Defaults to "misc".
	Domain string
	Tags   []string
}

// Source is a pull 源 (RSS / 市场榜单 / IM 群聊…). Fetch returns the current batch
// of items; the pipeline re-ingests by slug, so returning the same item twice is
// harmless (kwiki overwrites in place).
type Source interface {
	// Name identifies the source in logs and provenance ("rss:foo", "top50").
	Name() string
	// Fetch pulls the latest items. It may hit the network; callers run it on a
	// schedule, not a hot path.
	Fetch(ctx context.Context) ([]Item, error)
}

// Browser is the gstack /browse 能力在本仓的重映射接口：给定一个调研主题/URL，
// 返回抓取到的文本证据。真实实现可接 computer-use / headless 浏览器；本包提供
// StubBrowser 占位实现以保证离线可测 (RFC §5：不 vendor gstack Chromium 代码)。
type Browser interface {
	// Research fetches evidence for a topic. url 可为空 (纯主题检索)。返回的文本
	// 是 L2 深挖的原始素材，由角色/管线提炼成 why-分析。
	Research(ctx context.Context, topic, url string) (string, error)
}

// DeepDiveRule 判定一个 L1 条目是否值得触发 L2 深挖 (RFC §9.2)。返回 true 才花
// 配额做二次调研。默认规则见 KeywordRule。
type DeepDiveRule interface {
	ShouldDeepDive(item Item) bool
}

// DeepDiveRuleFunc adapts a plain func to DeepDiveRule.
type DeepDiveRuleFunc func(item Item) bool

func (f DeepDiveRuleFunc) ShouldDeepDive(item Item) bool { return f(item) }

// KeywordRule 命中任一关键词 (大小写不敏感，匹配标题+正文+标签) 即触发深挖。
// 对应 RFC「榜单出现未见过的产品形态」一类的内置触发规则。
func KeywordRule(keywords ...string) DeepDiveRule {
	lowered := make([]string, len(keywords))
	for i, k := range keywords {
		lowered[i] = strings.ToLower(k)
	}
	return DeepDiveRuleFunc(func(item Item) bool {
		hay := strings.ToLower(strings.Join(append([]string{item.Title, item.Text}, item.Tags...), "\n"))
		for _, k := range lowered {
			if k != "" && strings.Contains(hay, k) {
				return true
			}
		}
		return false
	})
}

// Card 是 L3 Discussion 卡草稿：深挖产物的人类可读结论，供调用方落进
// Discussion/建议卡表 (#189/#47)。本包不直接写库，只产出内容。
type Card struct {
	Title  string   // 卡标题 (= 触发深挖的条目标题)
	Body   string   // why-分析 Markdown 正文 (角色提示 + 证据摘要)
	Tags   []string // 继承自条目 (domain 等)
	Source string   // 触发深挖的源名 + 角色名，做 provenance
}

// ResearchResult 是 Run 的产物清单。
type ResearchResult struct {
	// Ingested 是本次落进 kwiki 的页面 slug (L0/L1 产物，含未深挖的)。
	Ingested []string
	// Cards 是命中深挖规则并完成 L2 调研后产出的 Discussion 卡草稿 (L3 产物)。
	Cards []Card
	// DeepDiveSkipped 是命中规则但因配额耗尽未深挖的条目标题，用于可观测/告警。
	DeepDiveSkipped []string
}

// Pipeline 编排一次完整的自动调研 (一个源 → 落 kwiki → 选择性深挖 → 出卡)。
// 字段全部由调用方注入，便于测试替身 (StubBrowser / StubSource)。
type Pipeline struct {
	Store   *kwiki.Store // 知识基底 (#191)，L0/L1 落点
	Source  Source       // pull 源
	Browser Browser      // L2 调研能力 (gstack /browse 重映射)
	Rule    DeepDiveRule // 深挖触发规则；nil = 从不深挖

	// Role 是执行 L2 调研的专家角色名 (如 "市场分析师")，仅作 provenance 与提示
	// 注入；角色模板的解析/可用性由 internal/agent 的 role loader 负责，本包不
	// 依赖它以保持解耦。空表示不挂角色。
	Role string
	// RolePrompt 是该角色的 system prompt 片段，注入到深挖卡正文头部，让结论带
	// 上角色视角 (市场分析师等)。空则省略。
	RolePrompt string

	// DeepDiveBudget 限制单次 Run 的 L2 调用次数 (RFC §9.2 限流)。<=0 表示禁用
	// 深挖 (纯抓取+ingest)。
	DeepDiveBudget int
}

// Run 执行一次管线：拉源 → 逐条 ingest → 命中规则且配额未尽则深挖出卡。
// 任一条目的 ingest 失败立即返回错误 (落库失败是硬错误)；深挖失败不致命——
// 记一条带错误说明的卡，继续处理后续条目 (调研是 best-effort)。
func (p *Pipeline) Run(ctx context.Context) (ResearchResult, error) {
	var res ResearchResult
	if p.Store == nil {
		return res, fmt.Errorf("research: nil kwiki store")
	}
	if p.Source == nil {
		return res, fmt.Errorf("research: nil source")
	}

	items, err := p.Source.Fetch(ctx)
	if err != nil {
		return res, fmt.Errorf("research: fetch %s: %w", p.Source.Name(), err)
	}

	budget := p.DeepDiveBudget
	for _, item := range items {
		page, err := p.Store.Ingest(toInboxItem(item, p.Source.Name()))
		if err != nil {
			return res, fmt.Errorf("research: ingest %q: %w", item.Title, err)
		}
		res.Ingested = append(res.Ingested, page.Slug)

		if p.Browser == nil || p.Rule == nil || !p.Rule.ShouldDeepDive(item) {
			continue
		}
		if budget <= 0 {
			res.DeepDiveSkipped = append(res.DeepDiveSkipped, item.Title)
			continue
		}
		budget--
		res.Cards = append(res.Cards, p.deepDive(ctx, item))
	}
	return res, nil
}

// deepDive runs the L2 research call and renders the result as a Discussion card
// draft. A browser error is non-fatal: it still produces a card noting the
// failure so the boss sees the attempt.
func (p *Pipeline) deepDive(ctx context.Context, item Item) Card {
	evidence, err := p.Browser.Research(ctx, item.Title, item.URL)
	src := p.Source.Name()
	if p.Role != "" {
		src += " / " + p.Role
	}
	card := Card{Title: item.Title, Tags: item.Tags, Source: src}
	if err != nil {
		card.Body = fmt.Sprintf("深度调研未完成：%v\n\n_触发条目_：%s", err, item.Title)
		return card
	}
	card.Body = renderCardBody(p.Role, p.RolePrompt, item, evidence)
	return card
}

// renderCardBody assembles the why-分析 card body: optional role lens, the
// triggering item, and the gathered evidence. 提炼成结论的智能由真实角色会话完成；
// 本包做的是把素材结构化成一张可讨论的卡。
func renderCardBody(role, rolePrompt string, item Item, evidence string) string {
	var b strings.Builder
	if role != "" {
		fmt.Fprintf(&b, "_调研角色_：%s\n\n", role)
	}
	if rolePrompt != "" {
		fmt.Fprintf(&b, "> %s\n\n", strings.ReplaceAll(strings.TrimSpace(rolePrompt), "\n", "\n> "))
	}
	b.WriteString("## 触发条目\n\n")
	fmt.Fprintf(&b, "**%s**\n\n", item.Title)
	if item.URL != "" {
		fmt.Fprintf(&b, "来源：%s\n\n", item.URL)
	}
	if s := strings.TrimSpace(item.Text); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	b.WriteString("## 调研证据\n\n")
	b.WriteString(strings.TrimSpace(evidence))
	b.WriteString("\n")
	return b.String()
}

// toInboxItem maps a pulled Item to the kwiki ingest value object, stamping the
// source so the wiki page records its origin.
func toInboxItem(item Item, source string) kwiki.InboxItem {
	return kwiki.InboxItem{
		ID:         item.ID,
		Title:      item.Title,
		Text:       item.Text,
		Source:     source,
		Domain:     item.Domain,
		Tags:       item.Tags,
		CapturedAt: time.Now(),
	}
}
