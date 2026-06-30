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

// SenderRef is one distinct message sender seen in a sync batch: the open_id and
// the org (tenant_key) carried by the message's sender object. The caller uses
// these to incrementally discover active speakers as degree-2 contacts, without
// a chat.members roster call.
type SenderRef struct {
	OpenID    string
	TenantKey string
}

// SyncResult reports one SyncChat run.
type SyncResult struct {
	ChatID    string
	Fetched   int   // messages returned by this pull (incl. boundary re-fetch)
	Inserted  int   // newly stored (deduped) rows
	Watermark int64 // new high-water mark, epoch ms
	// Senders are the distinct (open_id + tenant_key) message senders in this
	// batch, for incremental degree-2 ingestion by the caller. A sender with a
	// non-empty tenant_key wins over an empty one when the same open_id repeats.
	Senders []SenderRef
}

// SyncChat performs one incremental pull for a chat:
//  1. start from the stored watermark, or now-lookback for a first sync;
//  2. fetch messages ascending; enrich sender names from the caller-supplied
//     names map (open_id→name, sourced from the stored roster cache — NO
//     chat.members API call happens here);
//  3. upsert (dedup by message_id); advance the watermark to the max create_time.
//
// The returned SyncResult.Senders carries the distinct senders seen this batch
// so the caller can incrementally ingest active speakers (beyond the roster cap).
//
// start_time is seconds while create_time is milliseconds — the conversion is
// the one easy-to-get-wrong spot, so it is centralized here.
func (s *Syncer) SyncChat(ctx context.Context, chatID string, names map[string]string) (SyncResult, error) {
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

	// Enrich sender names from the caller-supplied roster cache (no API call).
	for i := range msgs {
		if n := names[msgs[i].SenderID]; n != "" {
			msgs[i].SenderName = n
		}
	}

	inserted, err := s.store.UpsertMessages(msgs)
	if err != nil {
		return SyncResult{}, err
	}

	newWM := wm
	senders := map[string]string{} // open_id → tenant_key (non-empty wins)
	for _, m := range msgs {
		if m.CreateTime > newWM {
			newWM = m.CreateTime
		}
		if m.SenderID == "" {
			continue
		}
		if tk, seen := senders[m.SenderID]; !seen || (tk == "" && m.SenderTenantKey != "") {
			senders[m.SenderID] = m.SenderTenantKey
		}
	}
	if newWM > wm {
		if err := s.store.SetWatermark(Channel, acc, chatID, newWM, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return SyncResult{}, err
		}
	}

	refs := make([]SenderRef, 0, len(senders))
	for openID, tk := range senders {
		refs = append(refs, SenderRef{OpenID: openID, TenantKey: tk})
	}

	return SyncResult{ChatID: chatID, Fetched: len(msgs), Inserted: inserted, Watermark: newWM, Senders: refs}, nil
}
