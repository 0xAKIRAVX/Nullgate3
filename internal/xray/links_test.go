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

// TestBuiltinRealityNetSwitch verifies the REALITY_NET=grpc switch:
//   - default (tcp) keeps the vision flow + spx probe path on type=tcp
//   - grpc emits type=grpc&serviceName&mode=gun with NO flow and no spx
func TestBuiltinRealityNetSwitch(t *testing.T) {
        base := LinkOpts{
                ClientID: "11111111-2222-3333-4444-555555555555", ClientName: "u",
                Addr: "panel.example.com", SNI: "www.samsung.com",
                Global: map[string]bool{"vless-reality": true},
                Reality: struct{ Pub, SID string }{Pub: "pubkey", SID: "abcd"},
                TCPHost: "tcp.example.com", TCPPort: "39146",
        }

        // default: tcp + vision
        var tcpLink string
        for _, l := range BuildLinks(base) {
                if strings.Contains(l.URL, "tcp.example.com:39146") {
                        tcpLink = l.URL
                }
        }
        if tcpLink == "" {
                t.Fatal("builtin reality link missing in tcp mode")
        }
        if !strings.Contains(tcpLink, "type=tcp") || !strings.Contains(tcpLink, "flow=xtls-rprx-vision") ||
                !strings.Contains(tcpLink, "spx=%2F") {
                t.Fatalf("tcp mode link wrong: %s", tcpLink)
        }

        // grpc: gun + serviceName, no flow/spx
        grpc := base
        grpc.RealityNet, grpc.RealitySvc = "grpc", "nullgate"
        var grpcLink string
        for _, l := range BuildLinks(grpc) {
                if strings.Contains(l.URL, "tcp.example.com:39146") {
                        grpcLink = l.URL
                }
        }
        if grpcLink == "" {
                t.Fatal("builtin reality link missing in grpc mode")
        }
        if !strings.Contains(grpcLink, "type=grpc") || !strings.Contains(grpcLink, "serviceName=nullgate") ||
                !strings.Contains(grpcLink, "mode=gun") || strings.Contains(grpcLink, "flow=") ||
                strings.Contains(grpcLink, "spx=") {
                t.Fatalf("grpc mode link wrong: %s", grpcLink)
        }
}
