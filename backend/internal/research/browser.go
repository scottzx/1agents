package research

import (
	"context"
	"fmt"
)

// StubBrowser is the offline placeholder for the gstack /browse 能力 (RFC §5：
// code-backed 技能不 vendor 其 Chromium 代码，重映射到自有浏览器/computer-use)。
// 它不联网，只回显 topic/url，让管线端到端可测，并为真实实现 (computer-use /
// headless 浏览器) 留出注入点 —— 生产环境用一个真正抓取的 Browser 实现替换它。
type StubBrowser struct {
	// Note 附在回显证据后，标记这是占位结果 (例如 "stub — no live network")。
	Note string
}

func (b StubBrowser) Research(_ context.Context, topic, url string) (string, error) {
	note := b.Note
	if note == "" {
		note = "占位实现：未联网抓取，待接入真实浏览能力 (gstack /browse 重映射)。"
	}
	if url != "" {
		return fmt.Sprintf("调研主题：%s\nURL：%s\n\n%s", topic, url, note), nil
	}
	return fmt.Sprintf("调研主题：%s\n\n%s", topic, note), nil
}

// FuncBrowser adapts a plain func to Browser, for tests and lightweight wiring.
type FuncBrowser func(ctx context.Context, topic, url string) (string, error)

func (f FuncBrowser) Research(ctx context.Context, topic, url string) (string, error) {
	return f(ctx, topic, url)
}
