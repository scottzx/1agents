// Package digest turns a batch of synced chat messages into an analysis prompt
// for an ACP agent, driven by hot-editable value-extraction templates stored in
// meta.db. It seeds a few global-default presets (通用社群/投资群/产品创业群/
// 招聘找人); a chat can bind any combination, and editing a template body takes
// effect on the next analysis with no rebuild.
package digest

import "github.com/scottzx/1Agents/backend/internal/meta"

// preset is a built-in template definition with a stable id so re-seeding is
// idempotent and never clobbers user edits (Seed only inserts when absent).
type preset struct {
	id        string
	name      string
	isDefault bool
	body      string
}

// presets are the built-in value standards. 通用社群 is the global fallback.
var presets = []preset{
	{
		id:        "tpl-builtin-general",
		name:      "通用社群",
		isDefault: true,
		body: `从社群聊天记录中提取有价值的信息,忽略寒暄/表情/接龙等噪音。关注:
- **项目/产品自荐**:谁在介绍什么项目,方向、亮点、联系方式、所在城市。
- **招募与找人**:谁在招人(找什么角色)、谁在找项目/找合作。
- **资源与链接**:分享的产品链接、文档、活动、直播回放。
- **运营公告与决策**:管理员发布的活动安排、报名截止、名单等。
- **待办/行动项**:需要跟进的事项与负责人。

输出用 Markdown,按上述类别分节;每条尽量保留 发言人 与 时间。无内容的类别可省略。`,
	},
	{
		id:   "tpl-builtin-investment",
		name: "投资群",
		body: `从投资/创投社群中提取对投资判断有价值的信号:
- **项目融资动态**:在融轮次、金额、估值、领投方。
- **赛道与趋势观点**:对某赛道/技术方向的判断、数据、拐点信号。
- **团队与背景**:创始团队履历、关键人事变动。
- **标的线索**:值得关注的项目及其差异化、护城河、风险点。
- **资源对接**:FA、机构、LP、被投企业之间的撮合需求。

每个标的尽量给出:项目名 / 方向 / 阶段 / 联系方式 / 一句话亮点 / 风险或存疑点。区分事实与推断。`,
	},
	{
		id:   "tpl-builtin-product",
		name: "产品创业群",
		body: `从产品/创业社群中提取对做产品有价值的内容:
- **需求与痛点**:用户/同行反复提到的真实痛点。
- **竞品与对标**:被提及的竞品、其优劣评价。
- **增长与变现**:获客渠道、转化、定价、商业化打法的经验。
- **产品自荐**:项目名 / 解决什么问题 / 目标用户 / 链接 / 联系方式 / 所在城市。
- **可复用经验**:技术选型、踩坑、工具推荐。

输出按类别分节,产品自荐整理成一张表(项目名 | 方向 | 联系方式 | 城市 | 阶段)。`,
	},
	{
		id:   "tpl-builtin-recruiting",
		name: "招聘找人",
		body: `从社群中提取招募/求职/组队相关信息,整理成两张表:
- **在招(找人)**:招募方 | 角色/技能 | 项目方向 | 地点 | 联系方式。
- **求职/找项目(待加入)**:个人 | 擅长/背景 | 期望方向 | 地点 | 联系方式。

只收录明确表达招募或求职意向的消息;附上发言人与时间,便于回溯。`,
	},
}

// DefaultTemplates returns the presets as DigestTemplate records (Builtin=true).
func DefaultTemplates() []meta.DigestTemplate {
	out := make([]meta.DigestTemplate, 0, len(presets))
	for _, p := range presets {
		out = append(out, meta.DigestTemplate{
			ID:        p.id,
			Name:      p.name,
			Scope:     "global",
			BodyMD:    p.body,
			Builtin:   true,
			IsDefault: p.isDefault,
		})
	}
	return out
}

// Seed inserts any built-in preset that is not already present. Seed-once (not
// overwrite) so a user's edits to a preset body survive restarts. Call on
// startup after opening meta.db.
func Seed(store *meta.DigestStore) error {
	for _, t := range DefaultTemplates() {
		_, ok, err := store.GetTemplate(t.ID)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		if err := store.UpsertTemplate(t); err != nil {
			return err
		}
	}
	return nil
}
