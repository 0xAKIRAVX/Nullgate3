package xray

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"nullgate/api/internal/config"
	"nullgate/api/internal/store"
)

// ClientRef is one active client as seen by the Xray config builder.
type ClientRef struct {
	ID        string   // UUID — used as both the Xray credential and the stats email
	Name      string   `json:"name"`
	Protocols []string `json:"protocols"` // per-user override; empty = follow global toggles
}

// CustomInbound is an admin-defined inbound stored in the inbounds table.
type CustomInbound struct {
	Tag      string `json:"tag"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"` // vless | vmess | trojan
	Network  string `json:"network"`  // ws | xhttp | httpupgrade | grpc | tcp
	Security string `json:"security"` // tls | reality
	Port     int    `json:"port"`     // reality only
	LPort    int    `json:"lport"`    // tls only (local port behind the reverse proxy)
	Path     string `json:"path"`     // ws/xhttp/httpupgrade: "/p", grpc: "/svc"
	SNI      string `json:"sni"`      // reality only
}

// Builder renders the Xray server config from Postgres state.
type Builder struct {
	Cfg  *config.Config
	Pool *pgxpool.Pool
}

// LoadClients returns every quota-valid, non-expired client (the only ones
// allowed inside the generated config).
func (b *Builder) LoadClients(ctx context.Context) ([]ClientRef, error) {
	rows, err := b.Pool.Query(ctx, `
                SELECT c.id::text, c.name, c.protocols::text FROM clients c
                WHERE (c.expire_at IS NULL OR c.expire_at > now())
                  AND (c.quota = 0 OR c.quota > COALESCE((SELECT u.up+u.down FROM client_usage u WHERE u.client_id=c.id),0))
                ORDER BY c.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClientRef{}
	for rows.Next() {
		var c ClientRef
		var raw []byte
		if err := rows.Scan(&c.ID, &c.Name, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &c.Protocols)
		out = append(out, c)
	}
	return out, rows.Err()
}

// LoadCustoms returns the admin-defined inbounds.
func (b *Builder) LoadCustoms(ctx context.Context) ([]CustomInbound, error) {
	rows, err := b.Pool.Query(ctx, `
                SELECT tag, name, protocol, network, security, COALESCE(port,0), COALESCE(lport,0),
                       path, svc, sni FROM inbounds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomInbound{}
	for rows.Next() {
		var ib CustomInbound
		var svc string
		if err := rows.Scan(&ib.Tag, &ib.Name, &ib.Protocol, &ib.Network, &ib.Security,
			&ib.Port, &ib.LPort, &ib.Path, &svc, &ib.SNI); err != nil {
			return nil, err
		}
		if ib.Network == "grpc" {
			ib.Path = "/" + svc
		}
		out = append(out, ib)
	}
	return out, rows.Err()
}

// userProtocols mirrors panel.py user_protocols: a non-empty per-user override
// wins, otherwise the global toggles decide.
func userProtocols(c ClientRef, global map[string]bool) map[string]bool {
	if len(c.Protocols) > 0 {
		set := map[string]bool{}
		for _, k := range c.Protocols {
			if global[k] {
				set[k] = true
			}
		}
		return set
	}
	return global
}

var privateNets = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.168.0.0/16", "::1/128", "fc00::/7", "fe80::/10",
}

// Built-in inbound tags → local ports (identical to panel.py so existing
// client links keep working after an upgrade).
const (
	PortWS     = 10001
	PortXHTTP  = 10002
	PortVmess  = 10004
	PortTrojan = 10005
	PortHU     = 10006
)

// BuiltResult is the rendered config plus the bookkeeping the supervisor needs.
type BuiltResult struct {
	Config map[string]any
	Active []string // client ids included in this config
}

// Build renders the full Xray server config.
func (b *Builder) Build(ctx context.Context) (*BuiltResult, error) {
	panel, err := store.LoadPanelSettings(ctx, b.Pool, b.Cfg.RealitySNI, b.Cfg.SessionHours)
	if err != nil {
		return nil, err
	}
	global := map[string]bool{}
	if m, ok := panel["protocols"].(map[string]any); ok {
		for k, v := range m {
			if bv, ok := v.(bool); ok {
				global[k] = bv
			}
		}
	}
	if len(global) == 0 {
		for _, k := range []string{"vless-reality", "vless-ws", "vless-xhttp", "vless-hu", "vmess-ws", "trojan-ws"} {
			global[k] = true
		}
	}
	sni, _ := panel["reality_sni"].(string)
	if sni == "" {
		sni = b.Cfg.RealitySNI
	}

	var reality struct{ Priv, SID string }
	if raw, err := store.GetJSON(ctx, b.Pool, "reality"); err == nil {
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			reality.Priv, _ = m["priv"].(string)
			reality.SID, _ = m["sid"].(string)
		}
	}

	clients, err := b.LoadClients(ctx)
	if err != nil {
		return nil, err
	}
	customs, err := b.LoadCustoms(ctx)
	if err != nil {
		return nil, err
	}

	active := make([]string, 0, len(clients))
	for _, c := range clients {
		active = append(active, c.ID)
	}

	// routeOnly: the sniffed domain is used for routing only; destination is not
	// rewritten (less DNS work).
	sniff := map[string]any{"enabled": true, "destOverride": []string{"http", "tls"}, "routeOnly": true}

	// inbound sockopt: TCP_NODELAY kills Nagle latency on every protocol;
	// fast-open saves one RTT for TFO-capable clients (harmless otherwise)
	inSockopt := func() map[string]any {
		return map[string]any{"tcpNoDelay": true, "tcpFastOpen": true, "tcpKeepAliveIdle": 300}
	}

	local := func(tag string, port int, proto string, settings, stream map[string]any) map[string]any {
		return map[string]any{"tag": tag, "listen": "127.0.0.1", "port": port,
			"protocol": proto, "settings": settings, "streamSettings": stream,
			"sniffing": sniff, "sockopt": inSockopt()}
	}
	wsStream := func(p string) map[string]any {
		return map[string]any{"network": "ws", "wsSettings": map[string]any{
			"path": p, "maxEarlyData": 2048, "earlyDataHeaderName": "Sec-WebSocket-Protocol"}}
	}
	// per-protocol credential lists: global toggles + per-user overrides both apply
	idsFor := func(key string, trojans ...bool) []map[string]any {
		trojan := len(trojans) > 0 && trojans[0]
		out := []map[string]any{}
		for _, c := range clients {
			if !userProtocols(c, global)[key] {
				continue
			}
			if trojan {
				out = append(out, map[string]any{"password": c.ID, "email": c.ID})
			} else {
				out = append(out, map[string]any{"id": c.ID, "email": c.ID})
			}
		}
		return out
	}

	inbounds := []map[string]any{}
	if global["vless-xhttp"] {
		// XHTTP behind an HTTP proxy: the server accepts every mode, the client uses packet-up
		inbounds = append(inbounds, local("vless-xhttp", PortXHTTP, "vless",
			map[string]any{"clients": idsFor("vless-xhttp"), "decryption": "none"},
			map[string]any{"network": "xhttp", "xhttpSettings": map[string]any{"path": b.Cfg.XHTTPPath, "mode": "auto"}}))
	}
	if global["vless-hu"] {
		inbounds = append(inbounds, local("vless-hu", PortHU, "vless",
			map[string]any{"clients": idsFor("vless-hu"), "decryption": "none"},
			map[string]any{"network": "httpupgrade", "httpupgradeSettings": map[string]any{"path": b.Cfg.HUPath}}))
	}
	if global["vless-ws"] {
		inbounds = append(inbounds, local("vless-ws", PortWS, "vless",
			map[string]any{"clients": idsFor("vless-ws"), "decryption": "none"}, wsStream(b.Cfg.WSPath)))
	}
	if global["vmess-ws"] {
		inbounds = append(inbounds, local("vmess-ws", PortVmess, "vmess",
			map[string]any{"clients": idsFor("vmess-ws")}, wsStream(b.Cfg.VmessPath)))
	}
	if global["trojan-ws"] {
		inbounds = append(inbounds, local("trojan-ws", PortTrojan, "trojan",
			map[string]any{"clients": idsFor("trojan-ws", true)}, wsStream(b.Cfg.TrojanPath)))
	}
	if reality.Priv != "" && global["vless-reality"] {
		// Reality TCP accounts MUST carry flow=xtls-rprx-vision: the client
		// links request vision and Xray (v24+) rejects accounts without it
		// ("account ... is not able to use the flow xtls-rprx-vision")
		realityClients := idsFor("vless-reality")
		for _, u := range realityClients {
			u["flow"] = "xtls-rprx-vision"
		}
		inbounds = append(inbounds, map[string]any{
			"tag": "vless-reality", "listen": "0.0.0.0", "port": b.Cfg.AppPort, "protocol": "vless",
			"settings": map[string]any{"clients": realityClients, "decryption": "none"},
			"streamSettings": map[string]any{
				"network": "tcp", "security": "reality",
				"realitySettings": map[string]any{
					"show": false, "dest": sni + ":443", "xver": 0, "serverNames": []string{sni},
					"privateKey": reality.Priv, "shortIds": []string{reality.SID},
				},
			},
			"sniffing": sniff,
			"sockopt":  inSockopt(),
		})
	}
	for _, ib := range customs {
		inb, err := b.customInbound(ctx, ib, clients, global, reality.Priv, reality.SID, sni, sniff)
		if err != nil {
			continue // skip broken customs instead of killing the whole config
		}
		inbounds = append(inbounds, inb)
	}
	inbounds = append(inbounds, map[string]any{"tag": "api", "listen": "127.0.0.1",
		"port": b.Cfg.APIPort, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"}})

	return &BuiltResult{
		Active: active,
		Config: map[string]any{
			"log":   map[string]any{"loglevel": envStr("XRAY_LOGLEVEL", "warning")},
			"stats": map[string]any{},
			"api":   map[string]any{"tag": "api", "services": []string{"StatsService"}},
			// tuned policy: longer idle window keeps tunnels warm (stable ping,
			// fewer re-handshakes) and a large per-connection buffer maximises
			// throughput on high-RTT paths (buffer caps speed ≈ size/RTT:
			// 4MB @ 200ms ≈ 160Mbps per connection; allocated lazily)
			"policy": map[string]any{"levels": map[string]any{"0": map[string]any{
				"statsUserUplink": true, "statsUserDownlink": true,
				"handshake": 4, "connIdle": envInt("XRAY_IDLE", 600), "bufferSize": envInt("XRAY_BUFFER", 4096)}}},
			// cached DoH lookups + IPv4 first = faster connection setup
			"dns": map[string]any{
				"servers":       []string{"https+local://1.1.1.1/dns-query", "localhost"},
				"queryStrategy": "UseIPv4",
			},
			"inbounds": inbounds,
			// tcpNoDelay + keepalive + fast-open on the direct outbound: lower TTFB,
			// long-lived connections survive idle NAT windows
			"outbounds": []map[string]any{
				{"tag": "direct", "protocol": "freedom", "settings": map[string]any{
					"domainStrategy": "UseIPv4",
					"sockopt":        map[string]any{"tcpNoDelay": true, "tcpKeepAliveIdle": 300, "tcpFastOpen": true}}},
				{"tag": "block", "protocol": "blackhole"},
			},
			// users must not reach the host's private network or the container itself
			"routing": map[string]any{"domainStrategy": "IPIfNonMatch", "rules": []map[string]any{
				{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"},
				{"type": "field", "domain": []string{"domain:railway.internal", "domain:localhost"}, "outboundTag": "block"},
				{"type": "field", "ip": privateNets, "outboundTag": "block"},
			}},
		},
	}, nil
}

// customInbound renders one admin-defined inbound (mirror of panel.py custom_inbound).
func (b *Builder) customInbound(ctx context.Context, ib CustomInbound, clients []ClientRef,
	global map[string]bool, priv, sid, defaultSNI string, sniff map[string]any) (map[string]any, error) {

	proto, net, sec := ib.Protocol, ib.Network, ib.Security
	clientsFor := func() []map[string]any {
		out := []map[string]any{}
		for _, c := range clients {
			if len(c.Protocols) > 0 {
				ok := false
				for _, k := range c.Protocols {
					if k == ib.Tag {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			if proto == "trojan" {
				out = append(out, map[string]any{"password": c.ID, "email": c.ID})
			} else {
				out = append(out, map[string]any{"id": c.ID, "email": c.ID})
			}
		}
		return out
	}

	settings := map[string]any{"clients": clientsFor()}
	if proto == "vless" {
		settings["decryption"] = "none"
	}
	stream := map[string]any{"network": net}
	switch net {
	case "ws":
		stream["wsSettings"] = map[string]any{"path": ib.Path, "maxEarlyData": 2048,
			"earlyDataHeaderName": "Sec-WebSocket-Protocol"}
	case "xhttp":
		stream["xhttpSettings"] = map[string]any{"path": ib.Path, "mode": "auto"}
	case "httpupgrade":
		stream["httpupgradeSettings"] = map[string]any{"path": ib.Path}
	case "grpc":
		stream["grpcSettings"] = map[string]any{"serviceName": trimSlash(ib.Path)}
	}
	var listen string
	var port int
	if sec == "reality" {
		if priv == "" {
			return nil, errRealityMissing
		}
		if net == "tcp" {
			stream["tcpSettings"] = map[string]any{}
		}
		stream["security"] = "reality"
		stream["realitySettings"] = map[string]any{
			"show": false, "dest": ib.SNI + ":443", "xver": 0, "serverNames": []string{ib.SNI},
			"privateKey": priv, "shortIds": []string{sid}}
		listen, port = "0.0.0.0", ib.Port
		if net == "tcp" {
			for _, c := range settings["clients"].([]map[string]any) {
				c["flow"] = "xtls-rprx-vision"
			}
		}
	} else { // TLS terminated by the platform edge, the API server routes the path here
		listen, port = "127.0.0.1", ib.LPort
	}
	return map[string]any{"tag": ib.Tag, "listen": listen, "port": port, "protocol": proto,
		"settings": settings, "streamSettings": stream, "sniffing": sniff,
		"sockopt": map[string]any{"tcpNoDelay": true, "tcpFastOpen": true, "tcpKeepAliveIdle": 300}}, nil
}

func trimSlash(p string) string {
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	return p
}
