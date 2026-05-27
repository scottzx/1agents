package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func readDaemonListenAddr() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ":8080"
	}
	data, err := os.ReadFile(filepath.Join(home, ".remote-agents", "daemon.json"))
	if err != nil {
		return ":8080"
	}
	var info struct {
		ListenAddr string `json:"listen_addr"`
	}
	if json.Unmarshal(data, &info) != nil || info.ListenAddr == "" {
		return ":8080"
	}
	return info.ListenAddr
}

func handleTunnelCommand(cmd string, port string) {
	listenAddr := readDaemonListenAddr()
	token, err := readManagementToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to read management token: %v\n", err)
		fmt.Fprintln(os.Stderr, "   Is remote-agents daemon running? A cc-connect config must exist.")
		os.Exit(1)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1%s", listenAddr)
	client := &http.Client{Timeout: 30 * time.Second}
	authHeader := "Bearer " + token

	switch cmd {
	case "start":
		startTunnel(client, baseURL, authHeader, port)

	case "stop":
		if port == "" {
			fmt.Fprintln(os.Stderr, "Usage: remote-agents tunnel stop <port>")
			fmt.Fprintln(os.Stderr, "       remote-agents tunnel stop-all")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Run 'remote-agents tunnel status' to see active tunnels and their ports.")
			os.Exit(1)
		}
		stopTunnel(client, baseURL, authHeader, port)

	case "stop-all":
		stopAllTunnels(client, baseURL, authHeader)

	case "status":
		showTunnelStatus(client, baseURL)

	default:
		fmt.Fprintf(os.Stderr, "Usage: remote-agents tunnel <start|stop|stop-all|status> [port]\n")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  start [port]   Start a tunnel (defaults to daemon port)")
		fmt.Fprintln(os.Stderr, "  stop <port>    Stop the tunnel for the given local port")
		fmt.Fprintln(os.Stderr, "  stop-all       Stop all active tunnels")
		fmt.Fprintln(os.Stderr, "  status         List all active tunnels")
		os.Exit(1)
	}
}

func startTunnel(client *http.Client, baseURL, authHeader, port string) {
	reqURL := baseURL + "/api/tunnel/start"
	if port != "" {
		reqURL += "?port=" + url.QueryEscape(port)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Request error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Connection error: Could not reach daemon at %s\n   %v\n", baseURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to start tunnel (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result struct {
		Port  string `json:"port"`
		URL   string `json:"url"`
		Token string `json:"token"`
		Link  string `json:"link"`
	}
	json.Unmarshal(body, &result)

	fmt.Println("\n📡 Public tunnel STARTED!")
	fmt.Printf("🔌 Local port: %s\n", result.Port)
	fmt.Printf("🔗 Secure link: %s\n", result.Link)
	fmt.Printf("🔑 Session token: %s\n\n", result.Token)
}

func stopTunnel(client *http.Client, baseURL, authHeader, port string) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/tunnel/stop?port="+url.QueryEscape(port), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Request error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Connection error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to stop tunnel (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result struct {
		Status string `json:"status"`
		Port   string `json:"port"`
	}
	json.Unmarshal(body, &result)
	fmt.Printf("✅ Tunnel stopped (port %s).\n", result.Port)
}

func stopAllTunnels(client *http.Client, baseURL, authHeader string) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/tunnel/stop-all", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Request error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Connection error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to stop all tunnels (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result struct {
		Status       string   `json:"status"`
		StoppedPorts []string `json:"stopped_ports"`
	}
	json.Unmarshal(body, &result)

	if len(result.StoppedPorts) == 0 {
		fmt.Println("⏸️  No active tunnels to stop.")
	} else {
		fmt.Printf("✅ All tunnels stopped (%d closed).\n", len(result.StoppedPorts))
	}
}

func showTunnelStatus(client *http.Client, baseURL string) {
	resp, err := client.Get(baseURL + "/api/tunnel/status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Connection error: Could not reach daemon at %s\n   %v\n", baseURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Active  bool `json:"active"`
		Tunnels []struct {
			Port  string `json:"port"`
			URL   string `json:"url"`
			Token string `json:"token"`
			Link  string `json:"link"`
		} `json:"tunnels"`
	}
	json.Unmarshal(body, &result)

	if !result.Active || len(result.Tunnels) == 0 {
		fmt.Println("⏸️  No active tunnels.")
		return
	}

	fmt.Printf("\n📡 Active tunnels (%d):\n\n", len(result.Tunnels))
	for _, t := range result.Tunnels {
		fmt.Printf("  🔌 Port %s → %s\n", t.Port, t.Link)
	}
	fmt.Println()
}
