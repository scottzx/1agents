package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoffeeProxyPreservesPublicOriginHeaders(t *testing.T) {
	type receivedRequest struct {
		Host           string `json:"host"`
		ForwardedHost  string `json:"forwardedHost"`
		ForwardedProto string `json:"forwardedProto"`
		Path           string `json:"path"`
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(receivedRequest{
			Host:           r.Host,
			ForwardedHost:  r.Header.Get("X-Forwarded-Host"),
			ForwardedProto: r.Header.Get("X-Forwarded-Proto"),
			Path:           r.URL.Path,
		})
	}))
	t.Cleanup(upstream.Close)

	proxy := NewCoffeeProxy(strings.TrimPrefix(upstream.URL, "http://"))
	request := httptest.NewRequest(http.MethodGet, "http://1agents.test/coffee/", nil)
	request.Host = "pay.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var received receivedRequest
	if err := json.Unmarshal(response.Body.Bytes(), &received); err != nil {
		t.Fatal(err)
	}
	if received.ForwardedHost != "pay.example.test" {
		t.Fatalf("X-Forwarded-Host = %q", received.ForwardedHost)
	}
	if received.ForwardedProto != "https" {
		t.Fatalf("X-Forwarded-Proto = %q", received.ForwardedProto)
	}
	if received.Path != "/coffee/" {
		t.Fatalf("path = %q", received.Path)
	}
}
