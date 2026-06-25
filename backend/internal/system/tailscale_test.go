package system

import (
	"context"
	"testing"
)

const sampleTailscaleStatus = `{
  "Self": {
    "HostName": "my-mac",
    "OS": "macOS",
    "TailscaleIPs": ["100.64.0.1"],
    "Online": true
  },
  "Peer": {
    "nodekey:aaa": {
      "HostName": "linux-server",
      "OS": "linux",
      "TailscaleIPs": ["100.64.0.2"],
      "Online": true
    },
    "nodekey:bbb": {
      "HostName": "offline-box",
      "OS": "linux",
      "TailscaleIPs": ["100.64.0.3"],
      "Online": false
    },
    "nodekey:ccc": {
      "HostName": "no-ip",
      "OS": "windows",
      "TailscaleIPs": [],
      "Online": true
    }
  }
}`

func TestParseTailscalePeers(t *testing.T) {
	peers := parseTailscalePeers([]byte(sampleTailscaleStatus))
	if len(peers) != 1 {
		t.Fatalf("want 1 online peer with an IP, got %d: %+v", len(peers), peers)
	}
	if peers[0].HostName != "linux-server" || peers[0].TailscaleIPs[0] != "100.64.0.2" {
		t.Errorf("unexpected peer: %+v", peers[0])
	}
}

func TestParseTailscalePeersGarbage(t *testing.T) {
	if got := parseTailscalePeers([]byte("not json")); got != nil {
		t.Errorf("garbage input should yield nil, got %+v", got)
	}
}

func TestDiscoverTailscaleDevicesProbeFilters(t *testing.T) {
	origStatus := runTailscaleStatus
	origProbe := probeHealth
	t.Cleanup(func() {
		runTailscaleStatus = origStatus
		probeHealth = origProbe
	})

	runTailscaleStatus = func(context.Context) ([]byte, error) {
		return []byte(`{
  "Peer": {
    "nodekey:1": {"HostName":"alive","OS":"linux","TailscaleIPs":["100.64.0.10"],"Online":true},
    "nodekey:2": {"HostName":"dead","OS":"linux","TailscaleIPs":["100.64.0.11"],"Online":true}
  }
}`), nil
	}
	// Only the "alive" node answers its health probe.
	probeHealth = func(_ context.Context, ip string) bool {
		return ip == "100.64.0.10"
	}

	found := discoverTailscaleDevices(context.Background())
	if len(found) != 1 {
		t.Fatalf("want 1 discovered device, got %d: %+v", len(found), found)
	}
	d := found[0]
	if d.ID != "alive" || d.TailscaleIP != "100.64.0.10" || d.Address != "100.64.0.10:38080" {
		t.Errorf("unexpected discovered device: %+v", d)
	}
}

func TestDiscoverTailscaleDevicesNoCLI(t *testing.T) {
	orig := runTailscaleStatus
	t.Cleanup(func() { runTailscaleStatus = orig })
	runTailscaleStatus = func(context.Context) ([]byte, error) {
		return nil, context.Canceled // simulate missing CLI / failure
	}
	if got := discoverTailscaleDevices(context.Background()); got != nil {
		t.Errorf("missing tailscale should yield nil, got %+v", got)
	}
}
