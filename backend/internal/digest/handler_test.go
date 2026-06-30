package digest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

func newTestHandler(t *testing.T) (*Handler, *feishu.Store, *meta.DigestStore) {
	t.Helper()
	dir := t.TempDir()
	mdb, err := meta.Open(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatalf("meta open: %v", err)
	}
	t.Cleanup(func() { mdb.Close() })
	fs, err := feishu.Open(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatalf("feishu open: %v", err)
	}
	t.Cleanup(func() { fs.Close() })
	ds := meta.NewDigestStore(mdb)
	h := NewHandler(fs, ds, meta.NewTaskStore(mdb), meta.NewFeishuChatStore(mdb), feishu.NewSyncer(fs, feishu.NewClient("", "self")))
	if err := h.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return h, fs, ds
}

func do(t *testing.T, fn http.HandlerFunc, method, url string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, url, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	fn(w, r)
	return w
}

func TestHandlerTemplatesAndBindings(t *testing.T) {
	h, _, _ := newTestHandler(t)

	// List presets (seeded).
	w := do(t, h.HandleTemplates, "GET", "/api/digest/templates", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var tpls []meta.DigestTemplate
	json.Unmarshal(w.Body.Bytes(), &tpls)
	if len(tpls) != len(presets) {
		t.Fatalf("expected %d presets, got %d", len(presets), len(tpls))
	}

	// Create a custom template.
	w = do(t, h.HandleTemplates, "POST", "/api/digest/templates", map[string]any{
		"name": "自定义群", "bodyMd": "custom standard",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var created meta.DigestTemplate
	json.Unmarshal(w.Body.Bytes(), &created)

	// Edit its body.
	w = do(t, h.HandleTemplateItem, "PATCH", "/api/digest/templates/"+created.ID, map[string]any{"bodyMd": "v2"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d", w.Code)
	}

	// Bind it to a chat, then resolve.
	w = do(t, h.HandleBindings, "POST", "/api/digest/bindings", map[string]any{
		"sessionId": "oc_x", "templateId": created.ID,
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("bind: %d %s", w.Code, w.Body)
	}
	w = do(t, h.HandleBindings, "GET", "/api/digest/bindings?session=oc_x", nil)
	var bound []meta.DigestTemplate
	json.Unmarshal(w.Body.Bytes(), &bound)
	if len(bound) != 1 || bound[0].BodyMD != "v2" {
		t.Fatalf("resolve bound: %+v", bound)
	}

	// Unbind → back to default.
	w = do(t, h.HandleBindings, "DELETE", "/api/digest/bindings?session=oc_x&template="+created.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("unbind: %d", w.Code)
	}
	w = do(t, h.HandleBindings, "GET", "/api/digest/bindings?session=oc_x", nil)
	json.Unmarshal(w.Body.Bytes(), &bound)
	if len(bound) != 1 || bound[0].Name != "通用社群" {
		t.Fatalf("after unbind expected default: %+v", bound)
	}

	// Delete custom template.
	w = do(t, h.HandleTemplateItem, "DELETE", "/api/digest/templates/"+created.ID, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
}

func TestHandlerAnalyzeAndMessages(t *testing.T) {
	h, fs, _ := newTestHandler(t)
	if _, err := fs.UpsertMessages([]feishu.Message{
		{Channel: feishu.Channel, MessageID: "om_1", SessionID: "oc_x", SenderName: "叶子", MsgType: "text", Content: `{"text":"项目自荐:AInvestor"}`, CreateTime: 1782388560000},
	}); err != nil {
		t.Fatalf("seed msgs: %v", err)
	}

	// Messages inspection.
	w := do(t, h.HandleMessages, "GET", "/api/digest/messages?session=oc_x", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("messages: %d", w.Code)
	}
	var msgs []feishu.Message
	json.Unmarshal(w.Body.Bytes(), &msgs)
	if len(msgs) != 1 || msgs[0].SenderName != "叶子" {
		t.Fatalf("messages: %+v", msgs)
	}

	// Analyze → creates a scheduler-eligible task.
	ws := t.TempDir()
	w = do(t, h.HandleAnalyze, "POST", "/api/digest/analyze", map[string]any{
		"chatId": "oc_x", "chatName": "测试群", "workspace": ws,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("analyze: %d %s", w.Code, w.Body)
	}
	var resp struct {
		TaskID       string `json:"taskId"`
		MessageCount int    `json:"messageCount"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TaskID == "" || resp.MessageCount != 1 {
		t.Fatalf("analyze resp: %+v", resp)
	}

	// Empty chat → 400.
	w = do(t, h.HandleAnalyze, "POST", "/api/digest/analyze", map[string]any{
		"chatId": "oc_empty", "workspace": ws,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("analyze empty: expected 400, got %d", w.Code)
	}
}
