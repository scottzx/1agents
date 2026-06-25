// Package feishusync implements bidirectional sync between the local 1agents
// task board and a Feishu (Lark) Bitable (多维表格).
//
// Design (issue #101, local-first):
//   - The local task store is the source of truth; on a true two-sided conflict
//     the local version wins and the remote change is reported, not applied.
//   - Records are matched by external id (Feishu record_id) carried on the task
//     via the #74 external-system sync fields, NOT by title — so re-running a
//     push is idempotent and never creates duplicate rows.
//
// The Bitable HTTP API is reached through the small BitableClient interface so
// the sync engine can be unit-tested with an in-memory fake, no live network.
// The auth model mirrors cc-connect's feishu integration: an app_id/app_secret
// pair is exchanged for a short-lived tenant_access_token against open.feishu.cn.
package feishusync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Feishu / Lark open-platform API bases. Mirrors the constants in
// modules/cc-connect/cmd/cc-connect/feishu.go (read-only reference).
const (
	OpenFeishuBaseURL = "https://open.feishu.cn"
	OpenLarkBaseURL   = "https://open.larksuite.com"
)

// Record is one Bitable row. Fields holds the column values keyed by column
// name; RecordID is the Feishu-assigned id (empty for a not-yet-created row).
// LastModified is the server-side modified time used for incremental pulls.
type Record struct {
	RecordID     string                 `json:"record_id,omitempty"`
	Fields       map[string]interface{} `json:"fields"`
	LastModified time.Time              `json:"-"`
}

// BitableClient is the minimal Bitable surface the syncer needs. It is an
// interface so tests inject an in-memory fake; the production implementation is
// TenantClient. All methods take an explicit (appToken, tableID) so one client
// can serve multiple project bindings.
type BitableClient interface {
	// ListRecords returns every record in the table.
	ListRecords(ctx context.Context, appToken, tableID string) ([]Record, error)
	// CreateRecord inserts a new row and returns it with its assigned RecordID.
	CreateRecord(ctx context.Context, appToken, tableID string, fields map[string]interface{}) (Record, error)
	// UpdateRecord overwrites the given fields of an existing row.
	UpdateRecord(ctx context.Context, appToken, tableID, recordID string, fields map[string]interface{}) (Record, error)
}

// TenantClient is the live BitableClient backed by the Feishu open API. It
// caches a tenant_access_token and refreshes it on expiry.
type TenantClient struct {
	BaseURL   string // OpenFeishuBaseURL or OpenLarkBaseURL
	AppID     string
	AppSecret string
	HTTP      *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewTenantClient builds a live client. baseURL defaults to OpenFeishuBaseURL
// when empty.
func NewTenantClient(baseURL, appID, appSecret string) *TenantClient {
	if baseURL == "" {
		baseURL = OpenFeishuBaseURL
	}
	return &TenantClient{
		BaseURL:   baseURL,
		AppID:     appID,
		AppSecret: appSecret,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

type tokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"` // seconds
}

// tenantToken returns a valid tenant_access_token, refreshing if needed. Mirrors
// the auth/v3/tenant_access_token/internal exchange used by cc-connect.
func (c *TenantClient) tenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"app_id":     c.AppID,
		"app_secret": c.AppSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	if err := json.Unmarshal(data, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.Code != 0 || tr.TenantAccessToken == "" {
		return "", fmt.Errorf("tenant token error: code=%d msg=%q", tr.Code, tr.Msg)
	}
	c.token = tr.TenantAccessToken
	// Refresh a minute early to avoid edge expiry.
	exp := tr.Expire
	if exp <= 0 {
		exp = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(exp-60) * time.Second)
	return c.token, nil
}

// apiError is the common Feishu response envelope.
type apiError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e apiError) err() error {
	if e.Code == 0 {
		return nil
	}
	return fmt.Errorf("feishu api error: code=%d msg=%q", e.Code, e.Msg)
}

func (c *TenantClient) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s %s: %w (body=%s)", method, path, err, truncate(data))
		}
	}
	return nil
}

func truncate(b []byte) string {
	const max = 256
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// ── Bitable record API ──

type listRecordsResp struct {
	apiError
	Data struct {
		Items []struct {
			RecordID         string                 `json:"record_id"`
			Fields           map[string]interface{} `json:"fields"`
			LastModifiedTime int64                  `json:"last_modified_time"` // ms epoch
		} `json:"items"`
		PageToken string `json:"page_token"`
		HasMore   bool   `json:"has_more"`
	} `json:"data"`
}

func (c *TenantClient) ListRecords(ctx context.Context, appToken, tableID string) ([]Record, error) {
	var out []Record
	pageToken := ""
	for {
		p := fmt.Sprintf("/open-apis/bitable/v1/apps/%s/tables/%s/records?page_size=500",
			url.PathEscape(appToken), url.PathEscape(tableID))
		if pageToken != "" {
			p += "&page_token=" + url.QueryEscape(pageToken)
		}
		var resp listRecordsResp
		if err := c.do(ctx, http.MethodGet, p, nil, &resp); err != nil {
			return nil, err
		}
		if err := resp.apiError.err(); err != nil {
			return nil, err
		}
		for _, it := range resp.Data.Items {
			out = append(out, Record{
				RecordID:     it.RecordID,
				Fields:       it.Fields,
				LastModified: time.UnixMilli(it.LastModifiedTime),
			})
		}
		if !resp.Data.HasMore || resp.Data.PageToken == "" {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return out, nil
}

type recordResp struct {
	apiError
	Data struct {
		Record struct {
			RecordID         string                 `json:"record_id"`
			Fields           map[string]interface{} `json:"fields"`
			LastModifiedTime int64                  `json:"last_modified_time"`
		} `json:"record"`
	} `json:"data"`
}

func (c *TenantClient) CreateRecord(ctx context.Context, appToken, tableID string, fields map[string]interface{}) (Record, error) {
	p := fmt.Sprintf("/open-apis/bitable/v1/apps/%s/tables/%s/records",
		url.PathEscape(appToken), url.PathEscape(tableID))
	var resp recordResp
	if err := c.do(ctx, http.MethodPost, p, map[string]interface{}{"fields": fields}, &resp); err != nil {
		return Record{}, err
	}
	if err := resp.apiError.err(); err != nil {
		return Record{}, err
	}
	return Record{
		RecordID:     resp.Data.Record.RecordID,
		Fields:       resp.Data.Record.Fields,
		LastModified: time.UnixMilli(resp.Data.Record.LastModifiedTime),
	}, nil
}

func (c *TenantClient) UpdateRecord(ctx context.Context, appToken, tableID, recordID string, fields map[string]interface{}) (Record, error) {
	p := fmt.Sprintf("/open-apis/bitable/v1/apps/%s/tables/%s/records/%s",
		url.PathEscape(appToken), url.PathEscape(tableID), url.PathEscape(recordID))
	var resp recordResp
	if err := c.do(ctx, http.MethodPut, p, map[string]interface{}{"fields": fields}, &resp); err != nil {
		return Record{}, err
	}
	if err := resp.apiError.err(); err != nil {
		return Record{}, err
	}
	return Record{
		RecordID:     resp.Data.Record.RecordID,
		Fields:       resp.Data.Record.Fields,
		LastModified: time.UnixMilli(resp.Data.Record.LastModifiedTime),
	}, nil
}

// compile-time assertion: TenantClient satisfies BitableClient.
var _ BitableClient = (*TenantClient)(nil)
