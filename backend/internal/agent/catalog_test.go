package agent

import "testing"

func TestAgentCatalogIncludesGrokBuild(t *testing.T) {
	for _, descriptor := range AgentCatalog {
		if descriptor.Type != AgentTypeGrokBuild {
			continue
		}
		if descriptor.Binary != "grok" {
			t.Fatalf("Grok binary = %q, want grok", descriptor.Binary)
		}
		if !descriptor.Integrated || !descriptor.AcpCapable || descriptor.CcTransport != TransportACP {
			t.Fatalf("Grok descriptor is not ACP chat-ready: %+v", descriptor)
		}
		if descriptor.AdapterPackage != "" {
			t.Fatalf("Grok must require its installed CLI, got adapter %q", descriptor.AdapterPackage)
		}
		return
	}
	t.Fatal("Grok Build missing from AgentCatalog")
}
