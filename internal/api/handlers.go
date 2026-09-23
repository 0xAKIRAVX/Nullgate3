package api

import (
        "context"
        "crypto/subtle"
        "encoding/base64"
        "encoding/json"
        "net/http"
        "regexp"
        "strings"
        "time"

        "github.com/google/uuid"

        "nullgate/api/internal/auth"
        "nullgate/api/internal/store"
        "nullgate/api/internal/xray"
)

// ───────────────────────── link / config building ─────────────────────────

// linkContext loads panel settings, reality keys and customs from Postgres and
// renders xray.LinkOpts for one client.
func (s *Server) linkContext(ctx context.Context, client clientRow, host string) (xray.LinkOpts, error) {
        panel, err := store.LoadPanelSettings(ctx, s.Pool, s.Cfg.RealitySNI, s.Cfg.SessionHours)
        if err != nil {
                return xray.LinkOpts{}, err
        }
        addr, _ := panel["address"].(string)
        if strings.TrimSpace(addr) == "" {
                addr = hostOnly(host)
        }
        sni, _ := panel["reality_sni"].(string)
        if sni == "" {
                sni = s.Cfg.RealitySNI
        }
        cfgFmt, _ := panel["cfg_fmt"].(string)
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
        customs, err := s.Sup.B.LoadCustoms(ctx)
        if err != nil {
                return xray.LinkOpts{}, err
        }
        o := xray.LinkOpts{
                ClientID:      client.ID,
                ClientName:    client.Name,
                Addr:          addr,
                SNI:           sni,
                CfgFmt:        cfgFmt,
                Global:        global,
                TCPHost:       s.Cfg.TCPHost,
                TCPPort:       s.Cfg.TCPPublicPort,
                WSPath:        s.Cfg.WSPath,
                XHTTPPath:     s.Cfg.XHTTPPath,
                HUPath:        s.Cfg.HUPath,
                VmessPath:     s.Cfg.VmessPath,
                TrojanPath:    s.Cfg.TrojanPath,
                UserProtocols: client.Protocols,
                Customs:       customs,
        }
        if raw, err := store.GetJSON(ctx, s.Pool, "reality"); err == nil {
                var m map[string]any
                if json.Unmarshal(raw, &m) == nil {
                        o.Reality.Pub, _ = m["pub"].(string)
                        o.Reality.SID, _ = m["sid"].(string)
                }
        }
        return o, nil
}

func hostOnly(hostport string) string {
        if i := strings.LastIndex(hostport, ":"); i > 0 && !strings.Contains(hostport[i:], "]") {
                return hostport[:i]
        }
        return hostport
}

// GET /api/clients/{id}/links — every shareable config for one user.
func (s *Server) clientLinks(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        if _, err := uuid.Parse(id); err != nil {
                writeErr(w, 400, "invalid client id")
                return
        }
        var name string
        var protos []byte
        err := s.Pool.QueryRow(r.Context(),
                `SELECT name, protocols::text FROM clients WHERE id=$1::uuid`, id).Scan(&name, &protos)
        if err != nil {
                writeErr(w, 404, "client not found")
                return
        }
        c := clientRow{ID: id, Name: name}
        _ = json.Unmarshal(protos, &c.Protocols)
        o, err := s.linkContext(r.Context(), c, r.Host)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, map[string]any{"links": xray.BuildLinks(o)})
}

// GET /api/clients/{id}/config — full Xray client JSON download.
func (s *Server) clientConfig(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        if _, err := uuid.Parse(id); err != nil {
                writeErr(w, 400, "invalid client id")
                return
        }
        var name string
        var protos []byte
        err := s.Pool.QueryRow(r.Context(),
                `SELECT name, protocols::text FROM clients WHERE id=$1::uuid`, id).Scan(&name, &protos)
        if err != nil {
                writeErr(w, 404, "client not found")
                return
        }
        c := clientRow{ID: id, Name: name}
        _ = json.Unmarshal(protos, &c.Protocols)
        o, err := s.linkContext(r.Context(), c, r.Host)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        jo := xray.ClientJSONOpts{
                ClientID: c.ID, Addr: o.Addr, SNI: o.SNI, Global: o.Global, TCPHost: o.TCPHost,
                TCPPort: o.TCPPort, WSPath: o.WSPath, XHTTPPath: o.XHTTPPath, HUPath: o.HUPath,
                VmessPath: o.VmessPath, TrojanPath: o.TrojanPath, UserProtocols: c.Protocols,
        }
        jo.Reality.Pub, jo.Reality.SID = o.Reality.Pub, o.Reality.SID
        cfg := xray.ClientJSON(jo)
        if _, hasErr := cfg["error"]; hasErr {
                writeErr(w, 400, cfg["error"].(string))
                return
        }
        safe := regexp.MustCompile(`[^A-Za-z0-9_.\-]`).ReplaceAllString(c.Name, "")
        if len(safe) > 24 || safe == "" {
                safe = "user"
        }
        body, _ := json.MarshalIndent(cfg, "", "  ")
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.Header().Set("Content-Disposition", "attachment; filename=nullgate-"+safe+".json")
        _, _ = w.Write(body)
}

// GET /api/server-config — the live Xray server config (diagnostics).
func (s *Server) serverConfig(w http.ResponseWriter, r *http.Request) {
        res, err := s.Sup.B.Build(r.Context())
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        body, _ := json.MarshalIndent(res.Config, "", "  ")
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.Header().Set("Content-Disposition", "attachment; filename=xray-server.json")
        _, _ = w.Write(body)
}

// ───────────────────────── public subscription ─────────────────────────

// GET /sub/{token} — the endpoint phone clients poll (v2rayNG reads the
// quota/expire headers; zero values mean unlimited and are simply omitted).
func (s *Server) sub(w http.ResponseWriter, r *http.Request) {
        tok := strings.Trim(strings.TrimPrefix(r.URL.Path, "/sub/"), "/")
        if tok == "" {
                http.Error(w, "not found", 404)
                return
        }
        rows, err := s.Pool.Query(r.Context(), `
                SELECT c.id::text, c.name, c.quota, COALESCE(c.expire_at,'1970-01-01'::timestamptz),
                       c.protocols::text, c.sub_token, COALESCE(u.up,0), COALESCE(u.down,0)
                FROM clients c LEFT JOIN client_usage u ON u.client_id = c.id`)
        if err != nil {
                http.Error(w, "not found", 404)
                return
        }
        defer rows.Close()
        var match *clientRow
        for rows.Next() {
                var c clientRow
                var expire time.Time
                var protoRaw, token []byte
                if err := rows.Scan(&c.ID, &c.Name, &c.Quota, &expire, &protoRaw, &token, &c.Up, &c.Down); err != nil {
                        continue
                }
                _ = json.Unmarshal(protoRaw, &c.Protocols)
                if subtle.ConstantTimeCompare([]byte(string(token)), []byte(tok)) == 1 {
                        if expire.Year() > 1971 {
                                iso := expire.UTC().Format(time.RFC3339)
                                c.ExpireAt = &iso
                        }
                        match = &c
                        break
                }
        }
        if match == nil {
                http.Error(w, "not found", 404)
                return
        }
        o, err := s.linkContext(r.Context(), *match, r.Host)
        if err != nil {
                http.Error(w, "server error", 500)
                return
        }
        links := xray.BuildLinks(o)
        body := xray.SubBody(links)
        title := base64.StdEncoding.EncodeToString([]byte(match.Name))
        info := []string{"upload=" + itoa64(match.Up), "download=" + itoa64(match.Down)}
        if match.Quota > 0 {
                info = append(info, "total="+itoa64(match.Quota))
        }
        if match.ExpireAt != nil {
                info = append(info, "expire="+*match.ExpireAt)
        }
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.Header().Set("Profile-Title", "base64:"+title)
        w.Header().Set("Profile-Update-Interval", "12")
        w.Header().Set("Subscription-Userinfo", strings.Join(info, "; "))
        _, _ = w.Write([]byte(body))
}

func itoa64(n int64) string {
        if n == 0 {
                return "0"
        }
        neg := n < 0
        if neg {
                n = -n
        }
        var b [24]byte
        i := len(b)
        for n > 0 {
                i--
                b[i] = byte('0' + n%10)
                n /= 10
        }
        if neg {
                i--
                b[i] = '-'
        }
        return string(b[i:])
}

// ───────────────────────── ops: logs / restart / ws ─────────────────────────

// GET /api/logs — last 150 lines of Xray's stderr.
func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, 200, map[string]any{"lines": s.Sup.TailLog(150)})
}

// POST /api/restart — force a config rebuild + process restart.
func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
        if err := s.Sup.Restart(r.Context(), true); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, map[string]any{"ok": true, "xray": s.Sup.Info()})
}

// GET /api/ws — live traffic stream for the admin UI.
func (s *Server) liveWS(w http.ResponseWriter, r *http.Request) {
        s.Hub.Serve(w, r)
}

// ───────────────────────── custom inbounds ─────────────────────────

var (
        hostRe    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
        svcRe     = regexp.MustCompile(`^[A-Za-z0-9_\-]{3,40}$`)
        pathOKRe  = regexp.MustCompile(`^[A-Za-z0-9/_\-.]+$`)
)

// normPath mirrors panel.py norm_path.
func normPath(v, def string) string {
        p := strings.TrimSpace(v)
        if p == "" {
                p = def
        }
        if !strings.HasPrefix(p, "/") {
                p = "/" + p
        }
        p = strings.TrimRight(p, "/")
        if p == "" {
                p = def
        }
        if !pathOKRe.MatchString(p) {
                return def
        }
        return p
}

// POST /api/inbounds — create a custom inbound (mirror of panel.py validate_inbound).
func (s *Server) createInbound(w http.ResponseWriter, r *http.Request) {
        var body struct {
                Name     string `json:"name"`
                Protocol string `json:"protocol"`
                Network  string `json:"network"`
                Security string `json:"security"`
                Port     int    `json:"port"`
                Path     string `json:"path"`
                SNI      string `json:"sni"`
        }
        if !readJSON(w, r, &body) {
                return
        }
        name := strings.TrimSpace(body.Name)
        if len(name) > 30 {
                name = name[:30]
        }
        proto := strings.ToLower(strings.TrimSpace(body.Protocol))
        net := strings.ToLower(strings.TrimSpace(body.Network))
        sec := strings.ToLower(strings.TrimSpace(body.Security))
        if name == "" {
                writeErr(w, 400, "نام اینباند را وارد کنید")
                return
        }
        var count int
        _ = s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM inbounds`).Scan(&count)
        if count >= 20 {
                writeErr(w, 400, "حداکثر ۲۰ اینباند سفارشی مجاز است")
                return
        }
        if !in(proto, "vless", "vmess", "trojan") || !in(net, "ws", "xhttp", "httpupgrade", "grpc", "tcp") ||
                !in(sec, "tls", "reality") {
                writeErr(w, 400, "انتخاب نامعتبر است")
                return
        }

        ib := xray.CustomInbound{Tag: "ib-" + randTag(), Name: name, Protocol: proto, Network: net, Security: sec}
        ctx := r.Context()
        if sec == "reality" {
                if proto != "vless" || !in(net, "tcp", "xhttp", "grpc") {
                        writeErr(w, 400, "Reality فقط با VLESS و ترنسپورت TCP یا XHTTP یا gRPC کار می‌کند")
                        return
                }
                var priv string
                if raw, err := store.GetJSON(ctx, s.Pool, "reality"); err != nil {
                        writeErr(w, 400, "کلید Reality آماده نیست")
                        return
                } else {
                        var m map[string]any
                        _ = json.Unmarshal(raw, &m)
                        priv, _ = m["priv"].(string)
                }
                if priv == "" {
                        writeErr(w, 400, "کلید Reality آماده نیست")
                        return
                }
                if body.Port < 1024 || body.Port > 65535 || s.portTaken(ctx, body.Port) {
                        writeErr(w, 400, "این پورت نامعتبر است یا قبلاً استفاده شده")
                        return
                }
                sni := strings.ToLower(strings.TrimSpace(body.SNI))
                if sni == "" {
                        if panel, err := store.LoadPanelSettings(ctx, s.Pool, s.Cfg.RealitySNI, s.Cfg.SessionHours); err == nil {
                                sni, _ = panel["reality_sni"].(string)
                        }
                }
                if !hostRe.MatchString(sni) {
                        writeErr(w, 400, "SNI نامعتبر است")
                        return
                }
                ib.Port, ib.SNI = body.Port, sni
                if net != "tcp" {
                        svc := strings.Trim(strings.TrimSpace(body.Path), "/")
                        if net == "grpc" {
                                if !svcRe.MatchString(svc) {
                                        writeErr(w, 400, "Service name فقط حروف انگلیسی/عدد/خط تیره (۳ تا ۴۰ نویسه)")
                                        return
                                }
                                ib.Path = "/" + svc
                        } else {
                                p := normPath(body.Path, "/"+randTag())
                                if len(p) < 3 {
                                        writeErr(w, 400, "مسیر نامعتبر است (حداقل ۲ نویسه؛ فقط حروف انگلیسی، عدد، - _ . /)")
                                        return
                                }
                                ib.Path = p
                        }
                }
        } else {
                if net == "tcp" {
                        writeErr(w, 400, "TCP خام فقط با Reality ممکن است (TLS را دامنه انجام می‌دهد)")
                        return
                }
                if proto == "vmess" && net == "xhttp" {
                        writeErr(w, 400, "VMess با XHTTP پشتیبانی نمی‌شود؛ VLESS یا Trojan را انتخاب کنید")
                        return
                }
                ib.LPort = s.freeLocalPort(ctx)
                if net != "tcp" {
                        if net == "grpc" {
                                svc := strings.Trim(strings.TrimSpace(body.Path), "/")
                                if svc == "" {
                                        svc = randTag()
                                }
                                if !svcRe.MatchString(svc) {
                                        writeErr(w, 400, "Service name فقط حروف انگلیسی/عدد/خط تیره (۳ تا ۴۰ نویسه)")
                                        return
                                }
                                ib.Path = "/" + svc
                        } else {
                                p := normPath(body.Path, "")
                                if p == "" {
                                        p = "/" + randTag()
                                }
                                if len(p) < 3 {
                                        writeErr(w, 400, "مسیر نامعتبر است (حداقل ۲ نویسه؛ فقط حروف انگلیسی، عدد، - _ . /)")
                                        return
                                }
                                low := strings.ToLower(p)
                                if strings.HasPrefix(low, "/panel") || strings.HasPrefix(low, "/sub") || strings.HasPrefix(low, "/api") {
                                        writeErr(w, 400, "این مسیر رزرو شده است")
                                        return
                                }
                                used := map[string]bool{
                                        strings.ToLower(s.Cfg.WSPath): true, strings.ToLower(s.Cfg.XHTTPPath): true,
                                        strings.ToLower(s.Cfg.HUPath): true, strings.ToLower(s.Cfg.VmessPath): true,
                                        strings.ToLower(s.Cfg.TrojanPath): true,
                                }
                                _ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM inbounds WHERE security='tls' AND lower(path)=$1`, low).Scan(&count)
                                if used[low] || count > 0 {
                                        writeErr(w, 400, "این مسیر قبلاً استفاده شده است")
                                        return
                                }
                                ib.Path = p
                        }
                }
        }

        svc := ""
        if ib.Network == "grpc" {
                svc = strings.Trim(ib.Path, "/")
        }
        var port, lport any
        if ib.Port > 0 {
                port = ib.Port
        }
        if ib.LPort > 0 {
                lport = ib.LPort
        }
        if _, err := s.Pool.Exec(ctx, `
                INSERT INTO inbounds(tag, name, protocol, network, security, port, lport, path, svc, sni)
                VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
                ib.Tag, ib.Name, ib.Protocol, ib.Network, ib.Security, port, lport, ib.Path, svc, ib.SNI); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        s.SyncCustomRoutes()
        _ = s.Sup.Restart(ctx, false)
        writeJSON(w, 200, map[string]any{"ok": true, "inbound": ib})
}

// DELETE /api/inbounds/{tag} — remove a custom inbound.
func (s *Server) deleteInbound(w http.ResponseWriter, r *http.Request) {
        tag := r.PathValue("tag")
        tagResp, err := s.Pool.Exec(r.Context(), `DELETE FROM inbounds WHERE tag=$1`, tag)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if tagResp.RowsAffected() == 0 {
                writeErr(w, 404, "inbound not found")
                return
        }
        s.SyncCustomRoutes()
        _ = s.Sup.Restart(r.Context(), false)
        writeJSON(w, 200, map[string]any{"ok": true})
}

func in(v string, allowed ...string) bool {
        for _, a := range allowed {
                if v == a {
                        return true
                }
        }
        return false
}

func randTag() string { return auth.RandomHex(4) }

// portTaken: reality customs must not collide with built-ins or each other.
func (s *Server) portTaken(ctx context.Context, port int) bool {
        builtin := map[int]bool{
                s.Cfg.AppPort: true, s.Cfg.APIPort: true,
                10001: true, 10002: true, 10003: true, 10004: true, 10005: true, 10006: true,
        }
        if n, err := atoiSafe(s.Cfg.HTTPPort); err {
                builtin[n] = true
        }
        var count int
        if err := s.Pool.QueryRow(ctx,
                `SELECT count(*) FROM inbounds WHERE port=$1 OR lport=$2`, port, port).Scan(&count); err != nil {
                return true
        }
        return builtin[port] || count > 0
}

func (s *Server) freeLocalPort(ctx context.Context) int {
        for p := 10100; p <= 10999; p++ {
                var count int
                if err := s.Pool.QueryRow(ctx,
                        `SELECT count(*) FROM inbounds WHERE lport=$1 OR port=$1`, p).Scan(&count); err == nil && count == 0 {
                        return p
                }
        }
        return 0
}

func atoiSafe(s string) (int, bool) {
        if s == "" {
                return 0, false
        }
        n := 0
        for i := 0; i < len(s); i++ {
                if s[i] < '0' || s[i] > '9' {
                        return 0, false
                }
                n = n*10 + int(s[i]-'0')
        }
        return n, true
}
