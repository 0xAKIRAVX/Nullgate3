package xray

import (
	"strings"
	"testing"
)

// TestRealityCustomEndpointMapping verifies:
//   - a custom Reality inbound whose port == TCP2AppPort gets the TCP2 proxy host:port
//   - unmapped custom Reality inbounds produce NO link on Railway (dead links)
//   - VPS mode (no TCP proxies configured) keeps direct Addr:port links
func TestRealityCustomEndpointMapping(t *testing.T) {
	customs := []CustomInbound{
		{Tag: "g", Name: "grpc", Protocol: "vless", Network: "grpc", Security: "reality",
			Port: 9001, Path: "/svc", SNI: "www.samsung.com"},
		{Tag: "n", Name: "nomap", Protocol: "vless", Network: "tcp", Security: "reality",
			Port: 9002, SNI: "www.samsung.com"},
	}

	// Railway mode: main TCP proxy + TCP2 proxy mapped to 9001
	railway := LinkOpts{
		ClientID: "11111111-2222-3333-4444-555555555555", ClientName: "u",
		Addr: "panel.example.com", SNI: "www.samsung.com",
		Reality: struct{ Pub, SID string }{Pub: "pubkey", SID: "abcd"},
		TCPHost: "tcp.example.com", TCPPort: "39146",
		TCP2Host: "tcp2.example.com", TCP2Port: "45678", TCP2AppPort: 9001,
		Customs: customs,
	}
	links := BuildLinks(railway)
	mapped := false
	for _, l := range links {
		if strings.Contains(l.URL, "tcp2.example.com:45678") {
			mapped = true
		}
		if strings.Contains(l.URL, "panel.example.com:900") || strings.Contains(l.URL, ":9002") ||
			strings.Contains(l.URL, "tcp.example.com:45678") {
			t.Fatalf("wrong endpoint leaked into custom reality link: %s", l.URL)
		}
	}
	if !mapped {
		t.Fatal("TCP2-mapped reality-grpc link was not generated")
	}

	// Railway mode without TCP2 → both custom reality links must be skipped
	noTCP2 := railway
	noTCP2.TCP2Host, noTCP2.TCP2Port = "", ""
	if n := len(BuildLinks(noTCP2)); n != 0 {
		t.Fatalf("unmapped reality customs should produce no links, got %d", n)
	}

	// VPS mode: no proxies at all → direct Addr:port links for both
	vps := railway
	vps.TCPHost, vps.TCPPort = "", ""
	vps.TCP2Host, vps.TCP2Port = "", ""
	links = BuildLinks(vps)
	if len(links) != 2 {
		t.Fatalf("VPS mode should link both custom inbounds directly, got %d", len(links))
	}
	for _, l := range links {
		if !strings.Contains(l.URL, "panel.example.com:900") {
			t.Fatalf("VPS link should use Addr:Port, got %s", l.URL)
		}
	}
}
