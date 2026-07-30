package harnesskit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/scottzx/1Agents/backend/internal/supervisor"
)

type clientRuntime struct {
	endpoint string
	token    string
}

func (r clientRuntime) Status() supervisor.HarnessKitStatus {
	return supervisor.HarnessKitStatus{State: "ready", Ready: true}
}

func (r clientRuntime) Endpoint() (string, string, bool) {
	return r.endpoint, r.token, true
}

func TestClientEnsureProjectTreatsConflictAsSuccessThenScans(t *testing.T) {
	var mu sync.Mutex
	var commands []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer private-token" {
			t.Fatalf("Authorization = %q", got)
		}
		mu.Lock()
		commands = append(commands, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/add_project":
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"kind": "Conflict", "message": "Project already added"})
		case "/api/scan_and_sync":
			_, _ = w.Write([]byte("4"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(clientRuntime{endpoint: server.URL, token: "private-token"})
	if err := client.EnsureProject(context.Background(), "/tmp/project"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 2 || commands[0] != "/api/add_project" || commands[1] != "/api/scan_and_sync" {
		t.Fatalf("commands = %#v", commands)
	}
}

func TestClientInstallToAgentUsesProjectScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExtensionID string `json:"extension_id"`
			TargetAgent string `json:"target_agent"`
			TargetScope struct {
				Type string `json:"type"`
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"target_scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ExtensionID != "global-skill" || body.TargetAgent != "claude" ||
			body.TargetScope.Type != "project" || body.TargetScope.Name != "Demo" ||
			body.TargetScope.Path != "/tmp/demo" {
			t.Fatalf("unexpected body: %+v", body)
		}
		_ = json.NewEncoder(w).Encode("project-skill")
	}))
	defer server.Close()

	client := NewClient(clientRuntime{endpoint: server.URL, token: "private-token"})
	id, err := client.InstallToAgent(context.Background(), "global-skill", "claude", "Demo", "/tmp/demo")
	if err != nil {
		t.Fatalf("InstallToAgent: %v", err)
	}
	if id != "project-skill" {
		t.Fatalf("id = %q", id)
	}
}
