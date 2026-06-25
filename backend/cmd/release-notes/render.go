package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// PageData is the fully-resolved input to the HTML template.
type PageData struct {
	Version    string // display version / tag for this release, e.g. "v20260625-1"
	PrevRef    string // lower bound of the range (tag or commit), may be empty
	CommitTo   string // upper bound ref, e.g. "HEAD"
	Repo       string // "owner/name", used to build PR links; may be empty
	Generated  string // generation timestamp (UTC)
	Groups     []Group
	TotalCount int
}

var funcs = template.FuncMap{
	"badgeClass": badgeClass,
	"hasScope":   func(s string) bool { return strings.TrimSpace(s) != "" },
	// prURL builds the GitHub PR link, or "" when repo/PR is unknown.
	"prURL": func(repo string, pr int) string {
		if repo == "" || pr == 0 {
			return ""
		}
		return fmt.Sprintf("https://github.com/%s/pull/%d", repo, pr)
	},
}

// badgeClass maps a commit type to one of the styled badge classes.
func badgeClass(t string) string {
	switch t {
	case "feat":
		return "feat"
	case "fix":
		return "fix"
	case "perf":
		return "perf"
	default:
		return "other"
	}
}

// pageTemplate is a single self-contained HTML document with all styles inlined
// so it can be opened directly from disk with no external assets. Colors mirror
// the project's semantic token families (accent / success / etc.) but are
// hard-coded here because the page lives outside the SCSS design system.
var pageTemplate = template.Must(template.New("page").Funcs(funcs).Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>1agents {{.Version}} 新功能介绍</title>
<style>
  :root {
    --bg-page: #f6f7f9; --bg-card: #ffffff; --border: #e5e7eb;
    --text-main: #111827; --text-secondary: #4b5563; --text-muted: #9ca3af;
    --accent-fg: #2563eb;
    --feat: #2563eb; --feat-bg: #eff6ff;
    --fix: #d97706; --fix-bg: #fffbeb;
    --perf: #7c3aed; --perf-bg: #f5f3ff;
    --other: #059669; --other-bg: #ecfdf5;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg-page: #0d1117; --bg-card: #161b22; --border: #30363d;
      --text-main: #e6edf3; --text-secondary: #9da7b3; --text-muted: #6e7681;
      --accent-fg: #58a6ff;
      --feat: #58a6ff; --feat-bg: #11203a;
      --fix: #e3b341; --fix-bg: #2b2410;
      --perf: #bc8cff; --perf-bg: #221634;
      --other: #3fb950; --other-bg: #0f2417;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 0 16px 64px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
    background: var(--bg-page); color: var(--text-main); line-height: 1.6;
  }
  .wrap { max-width: 860px; margin: 0 auto; }
  header { padding: 56px 0 32px; text-align: center; }
  header .eyebrow { color: var(--accent-fg); font-weight: 600; letter-spacing: .08em; text-transform: uppercase; font-size: 13px; }
  header h1 { font-size: 40px; margin: 12px 0 8px; }
  header .meta { color: var(--text-muted); font-size: 14px; }
  header .meta code { background: var(--bg-card); border: 1px solid var(--border); border-radius: 6px; padding: 1px 6px; }
  section { margin: 32px 0; }
  .section-head { display: flex; align-items: baseline; gap: 10px; margin: 0 0 16px; }
  .section-head h2 { font-size: 22px; margin: 0; }
  .section-head .count { color: var(--text-muted); font-size: 14px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 14px; }
  .card {
    background: var(--bg-card); border: 1px solid var(--border); border-radius: 14px;
    padding: 18px 20px; transition: transform .15s ease, box-shadow .15s ease;
  }
  .card:hover { transform: translateY(-2px); box-shadow: 0 8px 24px rgba(0,0,0,.08); }
  .card .tags { display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap; }
  .badge { font-size: 12px; font-weight: 600; padding: 2px 9px; border-radius: 999px; }
  .badge.feat { color: var(--feat); background: var(--feat-bg); }
  .badge.fix { color: var(--fix); background: var(--fix-bg); }
  .badge.perf { color: var(--perf); background: var(--perf-bg); }
  .badge.other { color: var(--other); background: var(--other-bg); }
  .scope { font-size: 12px; color: var(--text-secondary); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .card .summary { font-size: 15px; color: var(--text-main); margin: 0; }
  .card .pr { margin-top: 10px; font-size: 13px; }
  .card .pr a { color: var(--accent-fg); text-decoration: none; }
  .card .pr a:hover { text-decoration: underline; }
  .card .pr span { color: var(--text-muted); }
  footer { text-align: center; color: var(--text-muted); font-size: 13px; margin-top: 48px; }
  .empty { text-align: center; color: var(--text-muted); padding: 48px 0; }
</style>
</head>
<body>
<div class="wrap">
  <header>
    <div class="eyebrow">RELEASE NOTES</div>
    <h1>{{.Version}}</h1>
    <div class="meta">
      本次发布共 {{.TotalCount}} 项更新
      {{- if .PrevRef}} · 区间 <code>{{.PrevRef}}..{{.CommitTo}}</code>{{end}}
      · 生成于 {{.Generated}}
    </div>
  </header>

  {{if .Groups}}
  {{$repo := .Repo}}
  {{range .Groups}}
  <section>
    <div class="section-head">
      <h2>{{.Title}}</h2>
      <span class="count">{{len .Features}} 项</span>
    </div>
    <div class="grid">
      {{range .Features}}
      <div class="card">
        <div class="tags">
          <span class="badge {{badgeClass .Type}}">{{.Type}}</span>
          {{if hasScope .Scope}}<span class="scope">{{.Scope}}</span>{{end}}
        </div>
        <p class="summary">{{.Summary}}</p>
        {{if .PR}}
        <div class="pr">
          {{$u := prURL $repo .PR}}
          {{if $u}}<a href="{{$u}}" target="_blank" rel="noopener">#{{.PR}}</a>{{else}}<span>#{{.PR}}</span>{{end}}
        </div>
        {{end}}
      </div>
      {{end}}
    </div>
  </section>
  {{end}}
  {{else}}
  <div class="empty">该区间内没有可识别的功能更新。</div>
  {{end}}

  <footer>由 1agents release-notes 生成{{if .Repo}} · {{.Repo}}{{end}}</footer>
</div>
</body>
</html>
`))

// Render produces the complete HTML document for the given page data.
func Render(d PageData) (string, error) {
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}
