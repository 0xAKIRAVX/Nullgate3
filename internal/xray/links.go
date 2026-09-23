package xray

import (
        "encoding/base64"
        "encoding/json"
        "strconv"
        "strings"
        "time"
)

// Link is one shareable client config.
type Link struct {
        Key   string `json:"key"`
        Label string `json:"label"`
        URL   string `json:"url"`
}

// LinkOpts carries everything needed to render one client's links.
type LinkOpts struct {
        ClientID   string
        ClientName string

        Addr    string // public address for the 443 protocols (settings.address or request host)
        SNI     string // Reality serverName (settings)
        CfgFmt  string // display-name template
        Global  map[string]bool
        Reality struct{ Pub, SID string }
        TCPHost string
        TCPPort string

        WSPath, XHTTPPath, HUPath, VmessPath, TrojanPath string
        UserProtocols                                    []string // per-user override (empty = follow Global)
        Customs                                          []CustomInbound
}

// pyQuote percent-encodes every byte except unreserved ones (identical to
// Python's urllib.parse.quote(s, safe="") used by the reference panel).
func pyQuote(s string) string {
        const unreserved = "-_.~"
        var b strings.Builder
        for i := 0; i < len(s); i++ {
                c := s[i]
                if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
                        strings.IndexByte(unreserved, c) >= 0 {
                        b.WriteByte(c)
                        continue
                }
                b.WriteByte('%')
                b.WriteByte(upperHex(c >> 4))
                b.WriteByte(upperHex(c & 0x0f))
        }
        return b.String()
}

func upperHex(v byte) byte {
        if v < 10 {
                return '0' + v
        }
        return 'A' + v - 10
}

// fmtName mirrors panel.py fmt_name: template with {name} {label} {proto} {net}
// {sec} {server} {uuid8} {date}; a broken template falls back to name-label.
func fmtName(f string, name, label, proto, net, sec, server, uuid8 string) string {
        if f == "" {
                f = "{name}-{label}"
        }
        rep := strings.NewReplacer(
                "{name}", name, "{label}", label, "{proto}", proto, "{net}", net,
                "{sec}", sec, "{server}", server, "{uuid8}", uuid8, "{date}", time.Now().Format("20060102"))
        out := strings.Join(strings.Fields(rep.Replace(f)), " ")
        out = strings.Trim(out, "-_ ")
        if out == "" {
                out = name + "-" + label
        }
        return out
}

// tcpReady reports whether the Reality TCP Proxy is publicly reachable.
func (o *LinkOpts) tcpReady() bool { return o.TCPHost != "" && o.TCPPort != "" && o.Reality.Pub != "" }

// BuildLinks renders every shareable config for one client (mirror of
// panel.py build_links — same URL shapes so old clients keep importing them).
func BuildLinks(o LinkOpts) []Link {
        addr := o.Addr
        sni := o.SNI
        allowed := o.Global
        if len(o.UserProtocols) > 0 {
                allowed = map[string]bool{}
                for _, k := range o.UserProtocols {
                        if o.Global[k] {
                                allowed[k] = true
                        }
                }
        }
        tls := "security=tls&sni=" + pyQuote(addr) + "&fp=chrome" // SNI/Host must match the connect address
        uuid8 := o.ClientID
        if len(uuid8) > 8 {
                uuid8 = uuid8[:8]
        }
        links := []Link{}

        if allowed["vless-ws"] {
                n := fmtName(o.CfgFmt, o.ClientName, "VLESS-WS", "VLESS", "WS", "TLS", addr, uuid8)
                // ?ed=2048: WebSocket 0-RTT early data — the first request leaves the
                // client inside the TLS handshake, cutting one round-trip on cold connects
                links = append(links, Link{"vless-ws", "VLESS · WebSocket",
                        "vless://" + o.ClientID + "@" + addr + ":443?encryption=none&" + tls +
                                "&alpn=http%2F1.1&type=ws&host=" + pyQuote(addr) + "&path=" + pyQuote(o.WSPath+"?ed=2048") + "#" + pyQuote(n)})
        }
        if allowed["vless-xhttp"] {
                n := fmtName(o.CfgFmt, o.ClientName, "VLESS-XHTTP", "VLESS", "XHTTP", "TLS", addr, uuid8)
                links = append(links, Link{"vless-xhttp", "VLESS · XHTTP",
                        "vless://" + o.ClientID + "@" + addr + ":443?encryption=none&" + tls +
                                "&alpn=h2%2Chttp%2F1.1&type=xhttp&host=" + pyQuote(addr) + "&path=" + pyQuote(o.XHTTPPath) +
                                "&mode=packet-up#" + pyQuote(n)})
        }
        if allowed["vless-hu"] {
                n := fmtName(o.CfgFmt, o.ClientName, "VLESS-HTTPUpgrade", "VLESS", "HTTPUpgrade", "TLS", addr, uuid8)
                links = append(links, Link{"vless-hu", "VLESS · HTTPUpgrade",
                        "vless://" + o.ClientID + "@" + addr + ":443?encryption=none&" + tls +
                                "&alpn=http%2F1.1&type=httpupgrade&host=" + pyQuote(addr) + "&path=" + pyQuote(o.HUPath) + "#" + pyQuote(n)})
        }
        if allowed["vmess-ws"] {
                n := fmtName(o.CfgFmt, o.ClientName, "VMess-WS", "VMESS", "WS", "TLS", addr, uuid8)
                v := map[string]string{
                        "v": "2", "ps": n, "add": addr, "port": "443", "id": o.ClientID, "aid": "0",
                        "scy": "auto", "net": "ws", "type": "none", "host": addr,
                        "path": o.VmessPath + "?ed=2048", "tls": "tls", "sni": addr, "alpn": "http/1.1", "fp": "chrome",
                }
                raw, _ := json.Marshal(v)
                links = append(links, Link{"vmess-ws", "VMess · WebSocket", "vmess://" + base64.StdEncoding.EncodeToString(raw)})
        }
        if allowed["trojan-ws"] {
                n := fmtName(o.CfgFmt, o.ClientName, "Trojan-WS", "TROJAN", "WS", "TLS", addr, uuid8)
                links = append(links, Link{"trojan-ws", "Trojan · WebSocket",
                        "trojan://" + o.ClientID + "@" + addr + ":443?" + tls +
                                "&alpn=http%2F1.1&type=ws&host=" + pyQuote(addr) + "&path=" + pyQuote(o.TrojanPath+"?ed=2048") + "#" + pyQuote(n)})
        }
        if o.tcpReady() && allowed["vless-reality"] {
                n := fmtName(o.CfgFmt, o.ClientName, "VLESS-Reality-TCP", "VLESS", "TCP", "REALITY", o.TCPHost, uuid8)
                // spx=%2F keeps Xray's web fingerprint probing on the standard path
                links = append(links, Link{"vless-reality", "VLESS · Reality (TCP Proxy)",
                        "vless://" + o.ClientID + "@" + o.TCPHost + ":" + o.TCPPort +
                                "?encryption=none&flow=xtls-rprx-vision&security=reality&sni=" + pyQuote(sni) +
                                "&fp=chrome&pbk=" + pyQuote(o.Reality.Pub) + "&sid=" + pyQuote(o.Reality.SID) +
                                "&spx=%2F&type=tcp&headerType=none#" + pyQuote(n)})
        }
        for _, ib := range o.Customs {
                if ib.Security == "reality" && o.Reality.Pub == "" {
                        continue
                }
                links = append(links, customLink(ib, o))
        }
        return links
}

// customLink mirrors panel.py custom_link.
func customLink(ib CustomInbound, o LinkOpts) Link {
        uuid8 := o.ClientID
        if len(uuid8) > 8 {
                uuid8 = uuid8[:8]
        }
        addr := o.Addr
        nm := fmtName(o.CfgFmt, o.ClientName, ib.Name, strings.ToUpper(ib.Protocol),
                strings.ToUpper(ib.Network), strings.ToUpper(ib.Security), addr, uuid8)
        frag := pyQuote(nm)
        path := ib.Path
        if ib.Security == "reality" {
                p := "encryption=none&security=reality&sni=" + pyQuote(ib.SNI) + "&fp=chrome&pbk=" +
                        pyQuote(o.Reality.Pub) + "&sid=" + pyQuote(o.Reality.SID) + "&type=" + ib.Network
                switch ib.Network {
                case "tcp":
                        p += "&flow=xtls-rprx-vision&headerType=none"
                case "xhttp":
                        p += "&path=" + pyQuote(path) + "&mode=auto"
                case "grpc":
                        p += "&serviceName=" + pyQuote(trimSlash(path)) + "&mode=gun"
                }
                return Link{ib.Tag, ib.Name, "vless://" + o.ClientID + "@" + addr + ":" + itoa(ib.Port) + "?" + p + "#" + frag}
        }
        alpn := map[string]string{"ws": "http/1.1", "httpupgrade": "http/1.1", "xhttp": "h2,http/1.1", "grpc": "h2"}[ib.Network]
        if ib.Protocol == "vmess" {
                v := map[string]string{
                        "v": "2", "ps": nm, "add": addr, "port": "443", "id": o.ClientID, "aid": "0",
                        "scy": "auto", "net": ib.Network,
                        "type": map[string]string{"grpc": "gun"}[ib.Network], "host": addr,
                        "path": trimSlash(path), "tls": "tls", "sni": addr, "alpn": alpn, "fp": "chrome",
                }
                if ib.Network != "grpc" {
                        v["path"] = path
                        v["type"] = "none"
                } else {
                        v["serviceName"] = trimSlash(path)
                }
                raw, _ := json.Marshal(v)
                return Link{ib.Tag, ib.Name, "vmess://" + base64.StdEncoding.EncodeToString(raw)}
        }
        p := "security=tls&sni=" + pyQuote(addr) + "&fp=chrome&alpn=" + pyQuote(alpn) + "&type=" + ib.Network + "&host=" + pyQuote(addr)
        if ib.Network == "grpc" {
                p += "&serviceName=" + pyQuote(trimSlash(path)) + "&mode=gun"
        } else {
                p += "&path=" + pyQuote(path)
        }
        if ib.Network == "xhttp" {
                p += "&mode=packet-up"
        }
        scheme := ib.Protocol
        if ib.Protocol == "vless" {
                p = "encryption=none&" + p
        }
        return Link{ib.Tag, ib.Name, scheme + "://" + o.ClientID + "@" + addr + ":443?" + p + "#" + frag}
}

func itoa(n int) string { return strconv.Itoa(n) }

// SubBody renders the base64 subscription payload v2rayNG expects.
func SubBody(links []Link) string {
        urls := make([]string, 0, len(links))
        for _, l := range links {
                urls = append(urls, l.URL)
        }
        return base64.StdEncoding.EncodeToString([]byte(strings.Join(urls, "\n")))
}


