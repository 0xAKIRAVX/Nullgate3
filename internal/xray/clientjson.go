package xray

// ClientJSONOpts carries everything needed to render a full client config.
type ClientJSONOpts struct {
        ClientID string
        Addr     string // public address for the 443 protocols
        SNI      string
        Global   map[string]bool
        Reality  struct{ Pub, SID string }
        TCPHost  string
        TCPPort  string

        RealityNet string // builtin Reality transport: tcp (default) | grpc
        RealitySvc string // gRPC serviceName when RealityNet=grpc

        WSPath, XHTTPPath, HUPath, VmessPath, TrojanPath string
        UserProtocols                                    []string

        // Customs + TCP2 wiring: the full config must carry the SAME custom
        // inbounds as the share links / subscription — it used to render only
        // the builtin six, silently giving JSON users fewer outbounds.
        Customs    []CustomInbound
        TCP2Host   string
        TCP2Port   string
        TCP2AppPort int
}

// clientAllowed resolves the per-user protocol set.
func clientAllowed(o ClientJSONOpts) map[string]bool {
        if len(o.UserProtocols) > 0 {
                set := map[string]bool{}
                for _, k := range o.UserProtocols {
                        if o.Global[k] {
                                set[k] = true
                        }
                }
                return set
        }
        return o.Global
}

// ClientJSON builds the full, ready-to-import Xray client config for one user —
// tuned for Iranian users: ads/malware blocked, Iranian domains & IPs direct,
// DNS over HTTPS, mux off (XHTTP/Vision handle multiplexing natively).
func ClientJSON(o ClientJSONOpts) map[string]any {
        allowed := clientAllowed(o)
        addr := o.Addr
        cid := o.ClientID
        outbounds := []map[string]any{}
        order := 0
        nextTag := func(base string) string {
                order++
                if order == 1 {
                        return "proxy"
                }
                return "alt-" + base
        }

        sockopt := sockoptFn

        vlessOut := func(tag string, port int, stream map[string]any, flow string) map[string]any {
                return map[string]any{"tag": tag, "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{
                                "address": addr, "port": port,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": flow, "level": 0}},
                        }}},
                        "streamSettings": stream,
                        "mux":            map[string]any{"enabled": false, "concurrency": -1}}
        }

        if o.tcpReady() && allowed["vless-reality"] {
                port := 443
                if v := atoi(o.TCPPort); v > 0 {
                        port = v
                }
                rnet := o.RealityNet
                if rnet != "grpc" {
                        rnet = "tcp"
                }
                stream := map[string]any{
                        "security": "reality",
                        "realitySettings": map[string]any{"show": false, "fingerprint": "chrome",
                                "serverName": o.SNI, "publicKey": o.Reality.Pub, "shortId": o.Reality.SID, "spiderX": "/"},
                        "sockopt": sockopt(),
                }
                flow := ""
                if rnet == "grpc" {
                        stream["network"] = "grpc"
                        stream["grpcSettings"] = map[string]any{"serviceName": o.RealitySvc}
                } else {
                        flow = "xtls-rprx-vision" // vision is TCP-only
                        stream["network"] = "tcp"
                }
                outbounds = append(outbounds, map[string]any{
                        "tag": "proxy", "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{
                                "address": o.TCPHost, "port": port,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": flow, "level": 0}},
                        }}},
                        "streamSettings": stream,
                        "mux":            map[string]any{"enabled": false, "concurrency": -1}})
                order++
        }
        if allowed["vless-ws"] {
                outbounds = append(outbounds, vlessOut(nextTag("vless-ws"), 443, tlsStreamWS("ws", o.WSPath, addr, []string{"http/1.1"}, sockopt()), ""))
        }
        if allowed["vless-hu"] {
                outbounds = append(outbounds, vlessOut(nextTag("vless-hu"), 443, tlsStreamHU(o.HUPath, addr, sockopt()), ""))
        }
        if allowed["vless-xhttp"] {
                outbounds = append(outbounds, vlessOut(nextTag("vless-xhttp"), 443, tlsStreamXH(o.XHTTPPath, addr, sockopt()), ""))
        }
        if allowed["vmess-ws"] {
                outbounds = append(outbounds, map[string]any{"tag": nextTag("vmess-ws"), "protocol": "vmess",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": addr, "port": 443,
                                "users": []map[string]any{{"id": cid, "alterId": 0, "security": "auto", "level": 0}}}}},
                        "streamSettings": tlsStreamWS("ws", o.VmessPath, addr, []string{"http/1.1"}, sockopt()),
                        "mux":            map[string]any{"enabled": false, "concurrency": -1}})
        }
        if allowed["trojan-ws"] {
                outbounds = append(outbounds, map[string]any{"tag": nextTag("trojan-ws"), "protocol": "trojan",
                        "settings":       map[string]any{"servers": []map[string]any{{"address": addr, "port": 443, "password": cid, "level": 0}}},
                        "streamSettings": tlsStreamWS("ws", o.TrojanPath, addr, []string{"http/1.1"}, sockopt()),
                        "mux":            map[string]any{"enabled": false, "concurrency": -1}})
        }
        for _, ib := range o.Customs {
                // same membership rule as BuildLinks: a non-empty per-user list
                // must contain the inbound tag, or the outbound can never connect
                if len(o.UserProtocols) > 0 && !containsString(o.UserProtocols, ib.Tag) {
                        continue
                }
                ob := customOutbound(ib, o, nextTag(ib.Tag))
                if ob == nil {
                        continue // unreachable custom (e.g. Railway without a TCP Proxy)
                }
                outbounds = append(outbounds, ob)
        }
        if len(outbounds) == 0 {
                return map[string]any{"error": "no protocol enabled for this user"}
        }
        outbounds = append(outbounds,
                map[string]any{"tag": "direct", "protocol": "freedom",
                        "settings": map[string]any{"domainStrategy": "UseIPv4",
                                "sockopt": map[string]any{"tcpNoDelay": true, "tcpKeepAliveIdle": 300}}},
                map[string]any{"tag": "block", "protocol": "blackhole"})
        return map[string]any{
                "log": map[string]any{"loglevel": "warning"},
                // DoH first; plain resolvers as fallback. Iranian domains are routed
                // direct below, so they never depend on these foreign resolvers.
                "dns": map[string]any{"queryStrategy": "UseIPv4",
                        "servers": []string{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}},
                "inbounds": []map[string]any{
                        {"tag": "socks", "listen": "127.0.0.1", "port": 10808, "protocol": "socks",
                                "settings": map[string]any{"auth": "noauth", "udp": true, "userLevel": 0},
                                "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": false}},
                        {"tag": "http", "listen": "127.0.0.1", "port": 10809, "protocol": "http",
                                "settings": map[string]any{"userLevel": 0},
                                "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": false}},
                },
                "outbounds": outbounds,
                "routing": map[string]any{"domainStrategy": "IPIfNonMatch", "rules": []map[string]any{
                        // block QUIC: browsers fall back to HTTP/2 over TCP, which is faster
                        // and steadier inside a TCP-based tunnel
                        {"type": "field", "port": "443", "network": "udp", "outboundTag": "block"},
                        {"type": "field", "domain": []string{"geosite:category-ads-all"}, "outboundTag": "block"},
                        // Iranian sites stay direct: faster and does not burn quota
                        {"type": "field", "domain": []string{"geosite:category-ir", "regexp:.*\\.ir$"}, "outboundTag": "direct"},
                        {"type": "field", "ip": []string{"geoip:ir", "geoip:private"}, "outboundTag": "direct"},
                }},
        }
}

// sockoptFn is the shared outbound socket tuning (TCP_NODELAY kills Nagle
// latency, fast-open saves one RTT, keepalive survives idle NAT windows).
func sockoptFn() map[string]any {
        return map[string]any{"tcpNoDelay": true, "tcpFastOpen": true, "tcpKeepAliveIdle": 300}
}

// customOutbound renders one custom inbound as a full outbound — the JSON
// twin of customLink. Returns nil when the inbound has no public endpoint.
func customOutbound(ib CustomInbound, o ClientJSONOpts, tag string) map[string]any {
        cid := o.ClientID
        addr := o.Addr
        mux := map[string]any{"enabled": false, "concurrency": -1}
        if ib.Security == "reality" {
                if o.Reality.Pub == "" {
                        return nil
                }
                host, port, ok := resolveRealityEndpoint(ib, o.TCP2Host, o.TCP2Port, o.TCP2AppPort, o.TCPHost, o.TCPPort, o.Addr)
                if !ok {
                        return nil
                }
                stream := map[string]any{
                        "security": "reality",
                        "realitySettings": map[string]any{"show": false, "fingerprint": "chrome",
                                "serverName": ib.SNI, "publicKey": o.Reality.Pub, "shortId": o.Reality.SID, "spiderX": "/"},
                        "sockopt": sockoptFn(),
                }
                flow := ""
                switch ib.Network {
                case "tcp":
                        flow = "xtls-rprx-vision" // vision is TCP-only
                        stream["network"] = "tcp"
                case "grpc":
                        stream["network"] = "grpc"
                        stream["grpcSettings"] = map[string]any{"serviceName": trimSlash(ib.Path)}
                case "xhttp":
                        stream["network"] = "xhttp"
                        stream["xhttpSettings"] = map[string]any{"path": ib.Path, "host": ib.SNI, "mode": "packet-up"}
                }
                p := 443
                if v := atoi(port); v > 0 {
                        p = v
                }
                return map[string]any{"tag": tag, "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{
                                "address": host, "port": p,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": flow, "level": 0}},
                        }}},
                        "streamSettings": stream, "mux": mux}
        }

        // TLS custom inbound behind the panel domain (port 443), like customLink
        alpn := map[string]string{"ws": "http/1.1", "httpupgrade": "http/1.1", "xhttp": "h2,http/1.1", "grpc": "h2"}[ib.Network]
        var stream map[string]any
        switch ib.Network {
        case "grpc":
                stream = map[string]any{"network": "grpc", "security": "tls",
                        "tlsSettings":    map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{alpn}, "allowInsecure": false},
                        "grpcSettings":   map[string]any{"serviceName": trimSlash(ib.Path)},
                        "sockopt":         sockoptFn()}
        case "httpupgrade":
                stream = map[string]any{"network": "httpupgrade", "security": "tls",
                        "tlsSettings":         map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{alpn}, "allowInsecure": false},
                        "httpupgradeSettings": map[string]any{"path": ib.Path, "host": addr},
                        "sockopt":             sockoptFn()}
        case "xhttp":
                stream = map[string]any{"network": "xhttp", "security": "tls",
                        "tlsSettings":   map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{alpn}, "allowInsecure": false},
                        "xhttpSettings": map[string]any{"path": ib.Path, "host": addr, "mode": "packet-up"},
                        "sockopt":       sockoptFn()}
        default: // ws
                stream = map[string]any{"network": "ws", "security": "tls",
                        "tlsSettings": map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{alpn}, "allowInsecure": false},
                        "wsSettings":   map[string]any{"path": ib.Path, "headers": map[string]string{"Host": addr}},
                        "sockopt":       sockoptFn()}
        }
        switch ib.Protocol {
        case "vmess":
                return map[string]any{"tag": tag, "protocol": "vmess",
                        "settings":       map[string]any{"vnext": []map[string]any{{"address": addr, "port": 443,
                        "users": []map[string]any{{"id": cid, "alterId": 0, "security": "auto", "level": 0}}}}},
                        "streamSettings": stream, "mux": mux}
        case "trojan":
                return map[string]any{"tag": tag, "protocol": "trojan",
                        "settings":       map[string]any{"servers": []map[string]any{{"address": addr, "port": 443, "password": cid, "level": 0}}},
                        "streamSettings": stream, "mux": mux}
        default: // vless
                return map[string]any{"tag": tag, "protocol": "vless",
                        "settings":       map[string]any{"vnext": []map[string]any{{"address": addr, "port": 443,
                        "users": []map[string]any{{"id": cid, "encryption": "none", "flow": "", "level": 0}}}}},
                        "streamSettings": stream, "mux": mux}
        }
}

func tlsStreamWS(net, path, addr string, alpn []string, so map[string]any) map[string]any {
        return map[string]any{"network": net, "security": "tls",
                "tlsSettings": map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": alpn, "allowInsecure": false},
                "sockopt":     so,
                "wsSettings": map[string]any{"path": path + "?ed=2048",
                        "headers": map[string]string{"Host": addr}}}
}

func tlsStreamHU(path, addr string, so map[string]any) map[string]any {
        return map[string]any{"network": "httpupgrade", "security": "tls",
                "tlsSettings":         map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{"http/1.1"}, "allowInsecure": false},
                "sockopt":             so,
                "httpupgradeSettings": map[string]any{"path": path, "host": addr}}
}

func tlsStreamXH(path, addr string, so map[string]any) map[string]any {
        return map[string]any{"network": "xhttp", "security": "tls",
                "tlsSettings":   map[string]any{"serverName": addr, "fingerprint": "chrome", "alpn": []string{"h2", "http/1.1"}, "allowInsecure": false},
                "sockopt":       so,
                "xhttpSettings": map[string]any{"path": path, "mode": "packet-up", "host": addr}}
}

// tcpReady mirrors LinkOpts.tcpReady for the client JSON path.
func (o *ClientJSONOpts) tcpReady() bool {
        return o.TCPHost != "" && o.TCPPort != "" && o.Reality.Pub != ""
}

func atoi(s string) int {
        n := 0
        for i := 0; i < len(s); i++ {
                if s[i] < '0' || s[i] > '9' {
                        return 0
                }
                n = n*10 + int(s[i]-'0')
        }
        return n
}
