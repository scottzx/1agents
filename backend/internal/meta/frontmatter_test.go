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

func TestSplitFrontmatterDecorativeHeader(t *testing.T) {
	// Regression for #83: the README-header triple `--- / # Title / ---` used
	// to be parsed as YAML frontmatter, swallowing `# Title`. It must now be
	// treated as a thematic break + H1 + thematic break.
	doc := "---\n# Title\n---\n\nbody content here"
	fm, body := SplitFrontmatter(doc)
	if fm != "" {
		t.Fatalf("frontmatter = %q, want empty", fm)
	}
	// Opening `---` dropped as decoration; the inner `---` stays as a thematic break.
	if body != "# Title\n---\n\nbody content here" {
		t.Fatalf("body = %q", body)
	}
}

func TestSplitFrontmatterRealFrontmatterCoexists(t *testing.T) {
	// Legitimate acceptance list must still parse cleanly.
	doc := "---\nacceptance:\n  - 分页正确\n  - 空态有提示\n---\n## 背景\n正文"
	fm, body := SplitFrontmatter(doc)
	if fm != "acceptance:\n  - 分页正确\n  - 空态有提示" {
		t.Fatalf("frontmatter = %q", fm)
	}
	if body != "## 背景\n正文" {
		t.Fatalf("body = %q", body)
	}
}

func TestSplitFrontmatterMalformedNoClose(t *testing.T) {
	// No closing fence at all → preserve the original doc (legacy behavior).
	doc := "---\nacceptance: still body"
	fm, body := SplitFrontmatter(doc)
	if fm != "" {
		t.Fatalf("frontmatter = %q, want empty", fm)
	}
	if body != doc {
		t.Fatalf("body = %q, want original doc", body)
	}
}

func TestSplitFrontmatterNonYamlInsideFences(t *testing.T) {
	// Real prose between the fences must NOT be parsed as frontmatter.
	doc := "---\nreal prose\n---\nreal body"
	fm, body := SplitFrontmatter(doc)
	if fm != "" {
		t.Fatalf("frontmatter = %q, want empty", fm)
	}
	if body != "real prose\n---\nreal body" {
		t.Fatalf("body = %q", body)
	}
}

func TestSplitFrontmatterBOM(t *testing.T) {
	// ·built without a literal BOM in this source file (which Go's
	// compiler rejects). Compose the BOM by hand so we can verify the parser
	// tolerates a leading U+FEFF.
	doc := string([]byte{0xef, 0xbb, 0xbf}) + "---\nacceptance: x\n---\nbody"
	fm, body := SplitFrontmatter(doc)
	if fm != "acceptance: x" {
		t.Fatalf("frontmatter = %q", fm)
	}
	if body != "body" {
		t.Fatalf("body = %q", body)
	}
}

func TestRenderCardDoc(t *testing.T) {
	task := Task{
		Number:      7,
		Title:       "需求：分页",
		Type:        ItemTypeRequirement,
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
		// Regression for #83: the decoy `# Title` must not become frontmatter.
		{"markdown-decoy", "---\n# Title\n---\n\nbody", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FrontmatterAcceptance(c.doc); got != c.want {
				t.Fatalf("acceptance = %q, want %q", got, c.want)
			}
		})
	}
}
