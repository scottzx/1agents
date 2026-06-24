package meta

import "testing"

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
