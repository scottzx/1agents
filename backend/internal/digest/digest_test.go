package digest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func TestSeedIdempotentAndPreservesEdits(t *testing.T) {
	db, err := meta.Open(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := meta.NewDigestStore(db)

	if err := Seed(store); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, _ := store.ListTemplates()
	if len(all) != len(presets) {
		t.Fatalf("expected %d presets, got %d", len(presets), len(all))
	}
	// Default fallback is 通用社群 only.
	def, _ := store.TemplatesForSession("oc_any")
	if len(def) != 1 || def[0].Name != "通用社群" {
		t.Fatalf("default fallback: %+v", def)
	}

	// Edit a preset body, then re-seed: edit must survive.
	if err := store.UpdateTemplateBody("tpl-builtin-investment", "EDITED"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := Seed(store); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	v, _, _ := store.GetTemplate("tpl-builtin-investment")
	if v.BodyMD != "EDITED" {
		t.Fatalf("re-seed clobbered edit: %q", v.BodyMD)
	}
	if all2, _ := store.ListTemplates(); len(all2) != len(presets) {
		t.Fatalf("re-seed duplicated templates: %d", len(all2))
	}
}

func TestRenderBatchAndPrompt(t *testing.T) {
	msgs := []feishu.Message{
		{MessageID: "om_1", SenderName: "叶子", MsgType: "text", Content: `{"text":"今晚 8点 分享\n大家准时"}`, CreateTime: 1782388560000},
		{MessageID: "om_2", SenderID: "ou_xxxxxxabc123", MsgType: "post", Content: `{"title":"我的项目","content":[[{"tag":"text","text":"做 AI 投研"},{"tag":"a","href":"https://x.cn"}]]}`, CreateTime: 1782388620000},
		{MessageID: "om_3", SenderName: "小马", MsgType: "image", Content: `{"image_key":"k"}`, CreateTime: 1782388680000},
	}
	out := RenderBatch(msgs)
	if !strings.Contains(out, "叶子（text）: 今晚 8点 分享 大家准时") { // newline collapsed
		t.Fatalf("text render: %q", out)
	}
	if !strings.Contains(out, "我的项目 做 AI 投研 https://x.cn") {
		t.Fatalf("post render: %q", out)
	}
	if !strings.Contains(out, "…abc123（post）: 我的项目") { // fallback sender label (no name)
		t.Fatalf("sender fallback: %q", out)
	}
	if !strings.Contains(out, "小马（image）: [图片]") {
		t.Fatalf("image render: %q", out)
	}

	tpls := []meta.DigestTemplate{{Name: "投资群", BodyMD: "investment standard"}}
	prompt := BuildAnalysisPrompt("AGIBuilder", tpls, msgs)
	for _, want := range []string{"价值标准", "模板：投资群", "investment standard", "群「AGIBuilder」", "共 3 条"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
