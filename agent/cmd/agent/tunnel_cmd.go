package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type ccConnectManagement struct {
	Management struct {
		Token string `toml:"token"`
	} `toml:"management"`
}

func readManagementToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	configPath := filepath.Join(home, ".cc-connect", "config.toml")
	var cfg ccConnectManagement
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return "", fmt.Errorf("cannot read cc-connect config at %s: %w", configPath, err)
	}
	if cfg.Management.Token == "" {
		return "", fmt.Errorf("management token not found in %s", configPath)
	}
	return cfg.Management.Token, nil
}

func handleTunnelCommand(cmd string, listenAddr string) {
	token, err := readManagementToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to read management token: %v\n", err)
		fmt.Fprintln(os.Stderr, "   Is remote-agent daemon running? A cc-connect config must exist.")
		os.Exit(1)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1%s", listenAddr)
	client := &http.Client{Timeout: 30 * time.Second}

	switch cmd {
	case "start":
		req, err := http.NewRequest(http.MethodPost, baseURL+"/api/tunnel/start", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Request error: %v\n", err)
			os.Exit(1)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Connection error: Could not reach daemon at %s\n   %v\n", listenAddr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "⚠️  Failed to start tunnel (HTTP %d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}

		var result struct {
			URL   string `json:"url"`
			Token string `json:"token"`
			Link  string `json:"link"`
		}
		json.Unmarshal(body, &result)

		fmt.Println("\n📡 Public tunnel STARTED!")
		fmt.Printf("🔗 Secure  Link: %s\n", result.Link)
		fmt.Printf("🔑 Session Token: %s\n\n", result.Token)

	case "stop":
		req, err := http.NewRequest(http.MethodPost, baseURL+"/api/tunnel/stop", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Request error: %v\n", err)
			os.Exit(1)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Connection error: Could not reach daemon at %s\n   %v\n", listenAddr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "⚠️  Failed to stop tunnel (HTTP %d): %s\n", resp.StatusCode, string(body))
			os.Exit(1)
		}
		fmt.Println("✅ Tunnel stopped.")

	case "status":
		resp, err := client.Get(baseURL + "/api/tunnel/status")
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Connection error: Could not reach daemon at %s\n   %v\n", listenAddr, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Active bool   `json:"active"`
			URL    string `json:"url"`
			Token  string `json:"token"`
			Link   string `json:"link"`
		}
		json.Unmarshal(body, &result)

		if result.Active {
			fmt.Println("\n📡 Tunnel: ACTIVE")
			fmt.Printf("🔗 URL:  %s\n", result.URL)
			fmt.Printf("🔑 Link: %s\n\n", result.Link)
		} else {
			fmt.Println("⏸️  Tunnel: INACTIVE")
		}

	default:
		fmt.Fprintf(os.Stderr, "Usage: remote-agent tunnel <start|stop|status>\n")
		os.Exit(1)
	}
}
