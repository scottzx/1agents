package meta

import (
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	doc := "---\nacceptance:\n  - a\n  - b\n---\n## 背景\n正文"
	fm, body := SplitFrontmatter(doc)
	if fm != "acceptance:\n  - a\n  - b" {
		t.Fatalf("frontmatter = %q", fm)
	}
	if body != "## 背景\n正文" {
		t.Fatalf("body = %q", body)
	}

	// No frontmatter → all body.
	if fm, body := SplitFrontmatter("just text"); fm != "" || body != "just text" {
		t.Fatalf("no-fm split = %q / %q", fm, body)
	}
}

func TestRenderCardDoc(t *testing.T) {
	task := Task{
		Number:      7,
		Title:       "需求：分页",
		Type:        TaskTypeRequirement,
		Status:      TaskStatusPending,
		IssueState:  IssueOpen,
		Priority:    PriorityHigh,
		UserConfirm: true,
		Description: "---\nacceptance:\n  - 分页正确\n---\n## 背景\n正文",
	}
	doc := RenderCardDoc(task)
	// Round-trips: our own parser reads the generated doc back.
	fm, body := SplitFrontmatter(doc)
	if fm == "" {
		t.Fatalf("no frontmatter in rendered doc:\n%s", doc)
	}
	if strings.TrimSpace(body) != "## 背景\n正文" {
		t.Fatalf("body = %q", body)
	}
	for _, want := range []string{"number: 7", "type: requirement", "userConfirm: true", "分页正确"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("doc missing %q:\n%s", want, doc)
		}
	}
	// The source listed acceptance; re-emitted as a block scalar the list marker
	// survives as literal text. Content is preserved, which is what matters.
	if got := FrontmatterAcceptance(doc); got != "- 分页正确" {
		t.Fatalf("acceptance round-trip = %q", got)
	}
}

func TestFrontmatterAcceptance(t *testing.T) {
	cases := []struct{ name, doc, want string }{
		{"list", "---\nacceptance:\n  - 分页正确\n  - 空态有提示\n---\nbody", "- 分页正确\n- 空态有提示"},
		{"inline", "---\nacceptance: 一句话标准\n---\nbody", "一句话标准"},
		{"block", "---\nacceptance: |\n  第一行\n  第二行\n---\nbody", "第一行\n第二行"},
		{"absent", "---\npriority: high\n---\nbody", ""},
		{"no-frontmatter", "## 背景\n正文", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FrontmatterAcceptance(c.doc); got != c.want {
				t.Fatalf("acceptance = %q, want %q", got, c.want)
			}
		})
	}
}
