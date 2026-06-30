// Package social is the 社交数据回流 闭环 (#140, 里程碑 3.3 闭环正反馈).
//
// 项目发布到市面后（自媒体/社交平台）会产生真实流量、评论、粉丝数据。这是正
// 反馈信号，应回流进 Inbox 收口层 (#60) 反哺项目调整，形成闭环：
//
//	已发布内容 ──► MetricsSource.Fetch ──► Metrics 快照 ──► InboxItem ──► Inbox
//	  Publication        (订阅/拉取)        views/comments      (回流口)     #60
//	                                        /followers
//
//   - MetricsSource 是订阅/拉取已发布内容社交指标的接缝（接口）。真实社交 API
//     凭据由实现方注入；本包自带 StaticSource 占位实现，保证离线可单测（对齐
//     #190 research 包「接口 + 占位范式」的做法）。
//   - 指标快照经 Bridge 转成 meta.InboxItem，通过 #60 的 InboxSource 接缝
//     (IngestFrom) 回流进 Inbox。本包不改 Inbox 内部，也不直接写库。
//
// 正反馈三指标 views/comments/followers 对齐 gamified-dashboard「用真实流量取代
// 虚拟币」的决策。
package social

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// Metrics is one social-feedback snapshot for a single published piece of
// content on a single platform. The three头部指标 (views/comments/followers)
// are正反馈 signals; FetchedAt records when the snapshot was taken so回流的
// InboxItem 带时间锚点。
type Metrics struct {
	// PublicationID identifies the published content (post / video / article).
	PublicationID string
	// Title is the human-readable content title, carried into the InboxItem.
	Title string
	// URL is the public link to the content, if any.
	URL string
	// Views / Comments / Followers are the正反馈 counts.
	Views     int
	Comments  int
	Followers int
	// FetchedAt is the snapshot time; zero → filled with now on ingest.
	FetchedAt time.Time
}

// MetricsSource is the injectable seam for subscribing to / pulling social
// metrics of已发布 content. A real adapter (微博/抖音/小红书 API 等) implements
// Fetch using its own凭据; production wires that, tests inject StaticSource.
// Kept as an interface so the回流 bridge never depends on any concrete平台 SDK.
type MetricsSource interface {
	// Platform names the social platform ("weibo", "douyin", …); recorded as a
	// tag on the回流 InboxItem.
	Platform() string
	// Fetch returns the latest metrics snapshots for the content this source
	// tracks. Implementations may订阅 once and返回 cached snapshots, or pull
	// live每次 — the bridge does not care.
	Fetch(ctx context.Context) ([]Metrics, error)
}

// StaticSource is a fixed in-memory占位 source — used when no real social API
// credential is configured, and by tests. It returns its Snapshots verbatim,
// so the回流 path is fully exercisable offline.
type StaticSource struct {
	PlatformName string
	Snapshots    []Metrics
}

func (s *StaticSource) Platform() string {
	if s.PlatformName != "" {
		return s.PlatformName
	}
	return "static"
}

// Fetch returns the configured snapshots unchanged.
func (s *StaticSource) Fetch(context.Context) ([]Metrics, error) { return s.Snapshots, nil }

// toInboxItem renders a metrics snapshot into a meta.InboxItem. Source is left
// as meta.InboxSourceMisc (social feedback has no dedicated Inbox channel
// constant; it collapses to the杂项 bucket via normalizeSource) and the
// platform is recorded as a tag so the回流 trail is identifiable.
func toInboxItem(platform string, m Metrics) meta.InboxItem {
	fetched := m.FetchedAt
	if fetched.IsZero() {
		fetched = time.Now().UTC()
	}
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = m.PublicationID
	}
	summary := fmt.Sprintf("views %d · comments %d · followers %d",
		m.Views, m.Comments, m.Followers)
	tags := []string{"social-feedback"}
	if platform != "" {
		tags = append(tags, platform)
	}
	return meta.InboxItem{
		Source:    meta.InboxSourceMisc,
		Title:     title,
		Content:   summary,
		URL:       m.URL,
		Summary:   summary,
		Tags:      tags,
		CreatedAt: fetched,
	}
}

// Bridge adapts a MetricsSource into a meta.InboxSource (#60 接缝), so social
// feedback回流 reuses InboxStore.IngestFrom without the store importing this
// package. Each Drain pulls fresh snapshots from the underlying source and
// converts them into InboxItems.
type Bridge struct {
	src MetricsSource
}

// NewBridge wraps a MetricsSource so it can feed InboxStore.IngestFrom.
func NewBridge(src MetricsSource) *Bridge { return &Bridge{src: src} }

// Name identifies the回流 channel for the Inbox; it normalizes to misc.
func (b *Bridge) Name() string { return "social:" + b.src.Platform() }

// Drain fetches the latest snapshots and renders them as InboxItems. It uses a
// background context; callers needing cancellation can fetch themselves and
// build items via the exported helper instead.
func (b *Bridge) Drain() ([]meta.InboxItem, error) {
	snaps, err := b.src.Fetch(context.Background())
	if err != nil {
		return nil, err
	}
	items := make([]meta.InboxItem, 0, len(snaps))
	for _, m := range snaps {
		items = append(items, toInboxItem(b.src.Platform(), m))
	}
	return items, nil
}

// Reflow pulls every pending metrics snapshot from src and回流s it into the
// Inbox via the #60 IngestFrom seam. Returns the number of InboxItems captured.
// This is the single entry point a scheduler / publish hook calls after content
// goes live.
func Reflow(store *meta.InboxStore, src MetricsSource) (int, error) {
	return store.IngestFrom(NewBridge(src))
}
