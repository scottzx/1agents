package ingest

import (
	"encoding/json"
	"net/http"

	"github.com/scottzx/1Agents/backend/internal/feishu"
)

// CachedChat is one Feishu group as served to the 数据源 UI: parsed from the
// bronze cache (source_records kind=feishu_chat, written by the 群列表 sync
// task) and joined with its tracked state (feishu_tracked_chats = the message
// scope selection). Serving from bronze means opening the chat picker never
// shells out to lark-cli — the list is refreshed only when the user explicitly
// dispatches a 群列表 sync.
type CachedChat struct {
	ChatID      string `json:"chatId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	External    bool   `json:"external"`
	TenantKey   string `json:"tenantKey"`
	Tracked     bool   `json:"tracked"`
}

// chatsResponse carries the cache freshness alongside the rows so the UI can
// show "缓存于 <time>" and offer an explicit refresh.
type chatsResponse struct {
	Chats    []CachedChat `json:"chats"`
	CachedAt int64        `json:"cachedAt"` // epoch ms of the newest bronze row; 0 = never synced
}

// HandleChats serves GET /api/sources/feishu/chats — the cached group list.
func (h *Handler) HandleChats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	recs, err := h.bronze.ListRecords(feishu.Source, "feishu_chat", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tracked := map[string]bool{}
	if h.chats != nil {
		if list, err := h.chats.ListTrackedChats(false); err == nil {
			for _, c := range list {
				tracked[c.ChatID] = true
			}
		}
	}

	resp := chatsResponse{Chats: []CachedChat{}}
	for _, rec := range recs {
		if rec.Deleted {
			continue
		}
		var raw struct {
			ChatID      string `json:"chat_id"`
			Name        string `json:"name"`
			Avatar      string `json:"avatar"`
			Description string `json:"description"`
			External    bool   `json:"external"`
			TenantKey   string `json:"tenant_key"`
		}
		if err := json.Unmarshal([]byte(rec.Payload), &raw); err != nil || raw.ChatID == "" {
			continue // malformed bronze row — skip, never fail the whole list
		}
		resp.Chats = append(resp.Chats, CachedChat{
			ChatID: raw.ChatID, Name: raw.Name, Avatar: raw.Avatar,
			Description: raw.Description, External: raw.External, TenantKey: raw.TenantKey,
			Tracked: tracked[raw.ChatID],
		})
		if rec.FetchedAt > resp.CachedAt {
			resp.CachedAt = rec.FetchedAt
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
