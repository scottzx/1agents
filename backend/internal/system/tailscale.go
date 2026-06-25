package system

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// Tailscale auto-discovery (issue #110).
//
// `tailscale status --json` lists every peer in the tailnet along with its
// online state and tailnet IPs. We enumerate the online peers, probe each one's
// {ip}:38080/api/health, and surface the nodes that answer as candidate 1agents
// devices. This is best-effort: if the tailscale CLI is absent (common in CI /
// dev), discovery returns nothing rather than erroring.

const (
	// oneAgentsPort is the default port a peer 1agents instance listens on.
	oneAgentsPort = "38080"
	// healthPath is probed to confirm a peer is actually running 1agents.
	healthPath = "/api/health"
	// probeTimeout bounds each per-peer health probe (#110 specifies 2s).
	probeTimeout = 2 * time.Second
)

// tailscaleStatus is the subset of `tailscale status --json` we consume.
type tailscaleStatus struct {
	Self *tailscalePeer            `json:"Self"`
	Peer map[string]*tailscalePeer `json:"Peer"`
}

type tailscalePeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
}

// runTailscaleStatus is overridable in tests. It returns the raw JSON of
// `tailscale status --json`, or an error if the CLI is missing/fails.
var runTailscaleStatus = func(ctx context.Context) ([]byte, error) {
	bin, err := exec.LookPath("tailscale")
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, bin, "status", "--json").Output()
}

// probeHealth reports whether http://{ip}:38080/api/health answers 2xx within
// probeTimeout. Overridable in tests.
var probeHealth = func(ctx context.Context, ip string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	url := "http://" + ip + ":" + oneAgentsPort + healthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// tailscaleSelfIP returns this host's primary tailnet IP, or "" if Tailscale is
// not available. Used to populate the self device record.
func tailscaleSelfIP() string {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	raw, err := runTailscaleStatus(ctx)
	if err != nil {
		return ""
	}
	var status tailscaleStatus
	if json.Unmarshal(raw, &status) != nil || status.Self == nil {
		return ""
	}
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0]
	}
	return ""
}

// parseTailscalePeers extracts online peers (excluding self) with at least one
// tailnet IP. Pure function — unit-testable without the CLI.
func parseTailscalePeers(raw []byte) []tailscalePeer {
	var status tailscaleStatus
	if json.Unmarshal(raw, &status) != nil {
		return nil
	}
	var peers []tailscalePeer
	for _, p := range status.Peer {
		if p == nil || !p.Online || len(p.TailscaleIPs) == 0 {
			continue
		}
		peers = append(peers, *p)
	}
	return peers
}

// discoverTailscaleDevices runs tailscale status, probes each online peer's
// health endpoint concurrently (2s timeout each), and returns the peers that
// respond as candidate Device records. Returns nil when Tailscale is
// unavailable — this is the placeholder-safe path for environments without the
// CLI (CI, containers).
func discoverTailscaleDevices(ctx context.Context) []Device {
	raw, err := runTailscaleStatus(ctx)
	if err != nil {
		return nil
	}
	peers := parseTailscalePeers(raw)
	if len(peers) == 0 {
		return nil
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found []Device
	)
	for _, p := range peers {
		p := p
		ip := p.TailscaleIPs[0]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !probeHealth(ctx, ip) {
				return
			}
			dev := Device{
				ID:          p.HostName,
				Name:        p.HostName,
				OS:          p.OS,
				Address:     ip + ":" + oneAgentsPort,
				TailscaleIP: ip,
			}
			if dev.ID == "" {
				dev.ID = ip
				dev.Name = ip
			}
			mu.Lock()
			found = append(found, dev)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return found
}
