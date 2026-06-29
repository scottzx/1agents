package feishu

import (
	"context"
	"time"
)

// defaultLookback is how far back a brand-new (never-synced) chat is pulled:
// the first sync of a freshly joined group grabs the last 7 days.
const defaultLookback = 7 * 24 * time.Hour

// Syncer pulls a chat's messages into the Store incrementally.
type Syncer struct {
	store    *Store
	client   *Client
	lookback time.Duration
}

// NewSyncer wires a Store and Client together.
func NewSyncer(store *Store, client *Client) *Syncer {
	return &Syncer{store: store, client: client, lookback: defaultLookback}
}

// Client exposes the underlying lark-cli client (for chat listing / doctor).
func (s *Syncer) Client() *Client { return s.client }

// SyncResult reports one SyncChat run.
type SyncResult struct {
	ChatID    string
	Fetched   int   // messages returned by this pull (incl. boundary re-fetch)
	Inserted  int   // newly stored (deduped) rows
	Watermark int64 // new high-water mark, epoch ms
}

// SyncChat performs one incremental pull for a chat:
//  1. start from the stored watermark, or now-lookback for a first sync;
//  2. fetch messages ascending; enrich sender names via chat.members;
//  3. upsert (dedup by message_id); advance the watermark to the max create_time.
//
// start_time is seconds while create_time is milliseconds — the conversion is
// the one easy-to-get-wrong spot, so it is centralized here.
func (s *Syncer) SyncChat(ctx context.Context, chatID string) (SyncResult, error) {
	acc := s.client.account
	wm, ok, err := s.store.GetWatermark(Channel, acc, chatID)
	if err != nil {
		return SyncResult{}, err
	}
	var startSec int64
	if ok {
		startSec = wm / 1000 // ms → s; inclusive boundary, dedup handles overlap
	} else {
		startSec = time.Now().Add(-s.lookback).Unix()
	}

	msgs, err := s.client.FetchMessages(ctx, chatID, startSec)
	if err != nil {
		return SyncResult{}, err
	}

	// Best-effort name enrichment: a members lookup failure must not lose the
	// messages themselves, so it is non-fatal.
	if len(msgs) > 0 {
		if names, err := s.client.FetchMembers(ctx, chatID); err == nil {
			for i := range msgs {
				if n := names[msgs[i].SenderID]; n != "" {
					msgs[i].SenderName = n
				}
			}
		}
	}

	inserted, err := s.store.UpsertMessages(msgs)
	if err != nil {
		return SyncResult{}, err
	}

	newWM := wm
	for _, m := range msgs {
		if m.CreateTime > newWM {
			newWM = m.CreateTime
		}
	}
	if newWM > wm {
		if err := s.store.SetWatermark(Channel, acc, chatID, newWM, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return SyncResult{}, err
		}
	}

	return SyncResult{ChatID: chatID, Fetched: len(msgs), Inserted: inserted, Watermark: newWM}, nil
}
