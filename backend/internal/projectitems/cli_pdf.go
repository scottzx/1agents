package projectitems

// PDF export verb for the project-items CLI.
//
// Renders every item on the current project's board (需求/缺陷/任务/讨论) into a
// single PDF report. Fetches items via the same daemon HTTP path as `list`, so
// scoping (workspace resolution via --project or cwd) is identical.

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/signintech/gopdf"
)

// pdfItem is the fuller task JSON shape the report renders. The list-only
// task{} used by `list` drops fields we need here (Assignee, IssueState nuances,
// etc.), so we redecode with our own struct.
type pdfItem struct {
	ID                 string `json:"id"`
	Number             int    `json:"number"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	Type               string `json:"type"`
	Priority           string `json:"priority"`
	Milestone          string `json:"milestone"`
	Assignee           string `json:"assignee"`
	IssueState         string `json:"issueState"`
	AcceptanceCriteria string `json:"acceptanceCriteria"`
}

func cliPDF(args []string) int {
	fs := flag.NewFlagSet("pdf", flag.ContinueOnError)
	project := fs.String("project", "", "project id|name|path (default: cwd)")
	output := fs.String("out", "", "output PDF path (default: 项目看板-<timestamp>.pdf in cwd)")
	fontPath := fs.String("font", "", "override CJK-capable TTF font path (else auto-detect / ONEAGENTS_PDF_FONT)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	c, wsID, code := cliClient(*project)
	if code >= 0 {
		return code
	}

	items, err := fetchPDFItems(c)
	if err != nil {
		return cliFail("fetch items: %v", err)
	}

	projectName := lookupProjectName(c, wsID)

	font, err := resolvePDFFont(*fontPath)
	if err != nil {
		return cliFail("%v", err)
	}

	outPath := *output
	if strings.TrimSpace(outPath) == "" {
		stamp := time.Now().Format("20060102-150405")
		name := strings.TrimSpace(projectName)
		if name == "" {
			name = "项目看板"
		}
		outPath = fmt.Sprintf("%s-%s.pdf", name, stamp)
	}

	if err := renderKanbanPDF(outPath, projectName, items, font); err != nil {
		return cliFail("render pdf: %v", err)
	}
	fmt.Printf("wrote %s (%d items, font=%s)\n", outPath, len(items), font)
	return 0
}

// fetchPDFItems calls the same /api/agent/project-items?workspace_id=... endpoint
// as ListTasks, but decodes into pdfItem so the report has all display fields.
func fetchPDFItems(c *Client) ([]pdfItem, error) {
	q := url.Values{"workspace_id": {c.workspaceID}}
	status, body, err := c.api.do("GET", "/api/agent/project-items", q, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list failed (%d): %s", status, strings.TrimSpace(string(body)))
	}
	var items []pdfItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// lookupProjectName resolves a human-readable workspace name for the header;
// on any error it returns "" and the caller falls back to a generic title.
func lookupProjectName(c *Client, wsID string) string {
	status, body, err := c.api.do("GET", "/api/workspace/list", nil, nil)
	if err != nil || status != 200 {
		return ""
	}
	var list []wsRecord
	if err := json.Unmarshal(body, &list); err != nil {
		return ""
	}
	for _, w := range list {
		if w.ID == wsID {
			return w.Name
		}
	}
	return ""
}

// resolvePDFFont picks a CJK-capable TTF: explicit flag, env override, then a
// handful of well-known TTF locations per OS. TTC (font collections) are
// deliberately excluded — gopdf's TrueType parser cannot open them.
func resolvePDFFont(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("--font %q: %v", override, err)
		}
		return override, nil
	}
	if env := strings.TrimSpace(os.Getenv("ONEAGENTS_PDF_FONT")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}
	for _, p := range pdfFontCandidates() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf(
		"no CJK-capable TTF font found. Pass --font <path.ttf>, set ONEAGENTS_PDF_FONT, or drop a TTF at ~/.1agents/fonts/font.ttf")
}

func pdfFontCandidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".1agents", "fonts", "font.ttf"),
			filepath.Join(home, ".1agents", "fonts", "cjk.ttf"),
		)
	}
	switch runtime.GOOS {
	case "darwin":
		out = append(out,
			"/Library/Fonts/Arial Unicode.ttf",
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		)
	case "linux":
		out = append(out,
			"/usr/share/fonts/truetype/arphic/uming.ttf",
			"/usr/share/fonts/truetype/arphic/ukai.ttf",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttf",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttf",
			"/usr/share/fonts/opentype/source-han-sans/SourceHanSansSC-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf",
		)
	case "windows":
		out = append(out,
			`C:\Windows\Fonts\simhei.ttf`,
			`C:\Windows\Fonts\simkai.ttf`,
			`C:\Windows\Fonts\simfang.ttf`,
		)
	}
	return out
}

// ── rendering ────────────────────────────────────────────────────────────────

const (
	pdfPageWidth   = 595.28 // A4 pt
	pdfPageHeight  = 841.89
	pdfMarginLeft  = 40.0
	pdfMarginRight = 40.0
	pdfMarginTop   = 40.0
	pdfMarginBot   = 40.0
	pdfFontFamily  = "cjk"
)

// typeOrder groups items by 需求 → 缺陷 → 任务 → 讨论 → other (issue-model board
// ordering used by the daemon's project item panel).
var typeOrder = []struct {
	Key   string
	Label string
}{
	{"requirement", "需求"},
	{"bug", "缺陷"},
	{"task", "任务"},
	{"discussion", "讨论"},
}

func renderKanbanPDF(outPath, projectName string, items []pdfItem, fontPath string) error {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: pdfPageWidth, H: pdfPageHeight}})
	if err := pdf.AddTTFFont(pdfFontFamily, fontPath); err != nil {
		return fmt.Errorf("load font %s: %w", fontPath, err)
	}
	pdf.AddPage()

	r := &pdfRenderer{pdf: &pdf, y: pdfMarginTop, lineW: pdfPageWidth - pdfMarginLeft - pdfMarginRight}

	// Header
	r.setFont(20)
	title := "项目看板报告"
	if strings.TrimSpace(projectName) != "" {
		title = "项目看板报告 · " + projectName
	}
	r.writeLine(title)
	r.setFont(10)
	r.writeLine(fmt.Sprintf("生成时间：%s     条目总数：%d",
		time.Now().Format("2006-01-02 15:04"), len(items)))
	r.gap(6)
	r.rule()

	// Group + emit each type section.
	byType := groupByType(items)
	knownTypes := map[string]bool{}
	for _, sec := range typeOrder {
		knownTypes[sec.Key] = true
		list := byType[sec.Key]
		r.writeSection(sec.Label, list)
	}
	// Anything else (e.g. legacy/unknown type) still ships in a catch-all
	// section so the report never silently drops board items.
	var otherKeys []string
	for k := range byType {
		if !knownTypes[k] {
			otherKeys = append(otherKeys, k)
		}
	}
	sort.Strings(otherKeys)
	for _, k := range otherKeys {
		label := "其他"
		if k != "" {
			label = "其他（" + k + "）"
		}
		r.writeSection(label, byType[k])
	}

	return pdf.WritePdf(outPath)
}

func groupByType(items []pdfItem) map[string][]pdfItem {
	out := map[string][]pdfItem{}
	for _, it := range items {
		t := it.Type
		if t == "" {
			t = "task"
		}
		out[t] = append(out[t], it)
	}
	for k := range out {
		list := out[k]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Number == 0 || list[j].Number == 0 {
				return list[i].Title < list[j].Title
			}
			return list[i].Number < list[j].Number
		})
		out[k] = list
	}
	return out
}

// pdfRenderer is a tiny top-down text cursor over a gopdf page. It handles
// pagination and Chinese-friendly char-level wrapping.
type pdfRenderer struct {
	pdf         *gopdf.GoPdf
	y           float64
	lineW       float64
	currentSize float64
}

func (r *pdfRenderer) setFont(size int) {
	_ = r.pdf.SetFont(pdfFontFamily, "", size)
	r.currentSize = float64(size)
}

func (r *pdfRenderer) gap(dy float64) {
	r.y += dy
}

// writeLine renders one line, wrapping char-by-char when it overflows the
// printable width. Auto-paginates when the cursor runs off the page.
func (r *pdfRenderer) writeLine(s string) {
	if s == "" {
		r.y += r.currentSize * 1.35
		return
	}
	segments := r.wrap(s, r.lineW)
	for _, seg := range segments {
		r.ensurePage()
		r.pdf.SetX(pdfMarginLeft)
		r.pdf.SetY(r.y)
		_ = r.pdf.Cell(nil, seg)
		r.y += r.currentSize * 1.35
	}
}

// writeIndented renders a wrapped block indented by dx points from the left
// margin (used for item body lines under the "#N Title" head).
func (r *pdfRenderer) writeIndented(s string, dx float64) {
	if s == "" {
		return
	}
	segments := r.wrap(s, r.lineW-dx)
	for _, seg := range segments {
		r.ensurePage()
		r.pdf.SetX(pdfMarginLeft + dx)
		r.pdf.SetY(r.y)
		_ = r.pdf.Cell(nil, seg)
		r.y += r.currentSize * 1.35
	}
}

func (r *pdfRenderer) wrap(s string, maxWidth float64) []string {
	// Normalize newlines and split on them so paragraphs preserve breaks.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		out = append(out, r.wrapOne(para, maxWidth)...)
	}
	return out
}

func (r *pdfRenderer) wrapOne(s string, maxWidth float64) []string {
	if s == "" {
		return []string{""}
	}
	// Char-by-char wrap: works for CJK where there are no word boundaries.
	// MeasureTextWidth requires the font subset to include every rune, which
	// AddTTFFont+SetFont does lazily on Cell — we call it before Cell, so gopdf
	// won't have measured yet. Manually building char widths is unreliable, so
	// we fall back to a heuristic: assume avg char width ≈ 0.55 × font size for
	// Latin, 1.0 × for CJK. Overflow tolerance is fine here; the goal is
	// legible pagination, not typographic perfection.
	var out []string
	var cur strings.Builder
	var curW float64
	for _, ch := range s {
		w := r.estCharWidth(ch)
		if curW+w > maxWidth && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(ch)
		curW += w
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (r *pdfRenderer) estCharWidth(ch rune) float64 {
	// Rough width estimate: CJK / full-width / emoji ≈ 1.0 em, ASCII ≈ 0.55 em.
	if ch < 0x80 {
		return r.currentSize * 0.55
	}
	// CJK Unified Ideographs, Hiragana/Katakana, Hangul, full-width forms,
	// most punctuation used in Chinese.
	return r.currentSize * 1.0
}

func (r *pdfRenderer) ensurePage() {
	if r.y > pdfPageHeight-pdfMarginBot {
		r.pdf.AddPage()
		r.y = pdfMarginTop
	}
}

func (r *pdfRenderer) rule() {
	r.ensurePage()
	r.pdf.SetLineWidth(0.5)
	r.pdf.Line(pdfMarginLeft, r.y, pdfPageWidth-pdfMarginRight, r.y)
	r.y += 6
}

func (r *pdfRenderer) writeSection(label string, items []pdfItem) {
	r.gap(8)
	r.setFont(14)
	r.writeLine(fmt.Sprintf("%s（%d）", label, len(items)))
	r.rule()
	if len(items) == 0 {
		r.setFont(10)
		r.writeIndented("（暂无）", 12)
		return
	}
	for _, it := range items {
		r.writeItem(it)
	}
}

func (r *pdfRenderer) writeItem(it pdfItem) {
	r.gap(4)
	// Head line: "#N  Title"
	r.setFont(12)
	head := fmt.Sprintf("#%d  %s", it.Number, strings.TrimSpace(it.Title))
	if it.Number == 0 {
		head = strings.TrimSpace(it.Title)
	}
	r.writeLine(head)

	// Meta line: status | priority | milestone | assignee | issueState
	r.setFont(10)
	metaBits := []string{}
	if s := strings.TrimSpace(it.Status); s != "" {
		metaBits = append(metaBits, "状态: "+s)
	}
	if s := strings.TrimSpace(it.IssueState); s != "" {
		metaBits = append(metaBits, "开闭: "+s)
	}
	if s := strings.TrimSpace(it.Priority); s != "" {
		metaBits = append(metaBits, "优先级: "+s)
	}
	if s := strings.TrimSpace(it.Milestone); s != "" {
		metaBits = append(metaBits, "里程碑: "+s)
	}
	if s := strings.TrimSpace(it.Assignee); s != "" {
		metaBits = append(metaBits, "负责人: "+s)
	}
	if len(metaBits) > 0 {
		r.writeIndented(strings.Join(metaBits, "    "), 12)
	}

	if s := strings.TrimSpace(it.AcceptanceCriteria); s != "" {
		r.writeIndented("验收标准：", 12)
		r.writeIndented(s, 24)
	}
	if s := strings.TrimSpace(it.Description); s != "" {
		r.writeIndented("描述：", 12)
		r.writeIndented(s, 24)
	}
	r.gap(4)
}
