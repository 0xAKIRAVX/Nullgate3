//go:build e2e

// NullGate 3.0 end-to-end test: real PostgreSQL + real Redis (miniredis) +
// real Xray (official binary) + a real client tunnel through the Go reverse
// proxy for all six protocols, plus the public subscription endpoint.
//
//      go test -tags e2e ./e2e/ -v -timeout 15m
package e2e

import (
        "context"
        "crypto/ecdsa"
        "crypto/elliptic"
        "crypto/rand"
        "crypto/sha256"
        "crypto/tls"
        "crypto/x509"
        "encoding/base64"
        "encoding/json"
        "fmt"
        "io"
        "math/big"
        "net"
        "net/http"
        "net/http/cookiejar"
        "os"
        "os/exec"
        "strings"
        "testing"
        "time"

        "github.com/alicebob/miniredis/v2"
        embeddedpostgres "github.com/fergusstrange/embedded-postgres"
        "github.com/redis/go-redis/v9"
        "golang.org/x/net/proxy"

        "nullgate/api/internal/api"
        "nullgate/api/internal/auth"
        "nullgate/api/internal/config"
        "nullgate/api/internal/db"
        "nullgate/api/internal/store"
        "nullgate/api/internal/xray"
)

const (
        plainAddr = "127.0.0.1:18080"
        tlsAddr   = "127.0.0.1:18443"
        pgPort    = 55432
        appPort   = 19000
        apiPort   = 19085
)

// socks ports for the six client protocol tunnels
var socks = map[string]int{
        "vless-ws":      10811,
        "vless-xhttp":   10812,
        "vless-hu":      10813,
        "vmess-ws":      10814,
        "trojan-ws":     10815,
        "vless-reality": 10816,
}

func TestNullGateE2E(t *testing.T) {
        base := t.TempDir()

        // allow reusing a pre-seeded Xray binary (GitHub downloads can be slow)
        xrayBin := base + "/xray/xray"
        if pre := os.Getenv("E2E_XRAY_BIN"); pre != "" {
                xrayBin = pre
        }

        // ── environment ──────────────────────────────────────────────────────
        t.Setenv("PORT", "18080")
        t.Setenv("COOKIE_INSECURE", "1")
        os.MkdirAll("/tmp/ng-e2e-work", 0o755)
	t.Setenv("NG_WORKDIR", "/tmp/ng-e2e-work")
        t.Setenv("XRAY_BIN", xrayBin)
        t.Setenv("XRAY_VERSION", "v26.3.27")
        t.Setenv("TCP_APP_PORT", fmt.Sprint(appPort))
        t.Setenv("XRAY_API_PORT", fmt.Sprint(apiPort))
        t.Setenv("COLLECT_INTERVAL", "2")
        t.Setenv("TCP_HOST", "127.0.0.1")
        t.Setenv("TCP_PORT", fmt.Sprint(appPort))
        t.Setenv("CORS_ORIGIN", "*")
	t.Setenv("XRAY_LOGLEVEL", "info")

        mr := miniredis.RunT(t)
        t.Setenv("REDIS_URL", "redis://"+mr.Addr())

        pg := embeddedpostgres.NewDatabase(
                embeddedpostgres.DefaultConfig().
                        Port(uint32(pgPort)).
                        RuntimePath(base + "/pg").
                        Username("ng").Password("ng").Database("nullgate"))
        if err := pg.Start(); err != nil {
                t.Fatalf("embedded postgres: %v", err)
        }
        defer pg.Stop()
        t.Setenv("DATABASE_URL", fmt.Sprintf("postgres://ng:ng@127.0.0.1:%d/nullgate?sslmode=disable", pgPort))

        cfg := config.Load()
        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()

        // ── core stack ───────────────────────────────────────────────────────
        pool, err := db.Connect(ctx, cfg.DatabaseURL)
        if err != nil {
                t.Fatalf("postgres connect: %v", err)
        }
        defer pool.Close()
        if err := db.Migrate(ctx, pool); err != nil {
                t.Fatalf("migrate: %v", err)
        }
        if err := store.EnsurePanelSettings(ctx, pool, cfg.RealitySNI, cfg.SessionHours); err != nil {
                t.Fatalf("seed settings: %v", err)
        }
        if err := store.EnsureReality(ctx, pool, func() (map[string]any, error) {
                k, err := xray.GenerateReality()
                if err != nil {
                        return nil, err
                }
                t.Logf("reality keypair: pub=%s sid=%s", k.Pub, k.SID)
                return map[string]any{"priv": k.Priv, "pub": k.Pub, "sid": k.SID}, nil
        }); err != nil {
                t.Fatalf("seed reality: %v", err)
        }

        sup := xray.NewSupervisor(cfg, pool)
        if _, err := os.Stat(xrayBin); err != nil {
                t.Log("downloading real Xray binary…")
                if err := sup.EnsureBinary(ctx); err != nil {
                        t.Fatalf("ensure xray binary: %v", err)
                }
        }
        if err := sup.Restart(ctx, true); err != nil {
                t.Fatalf("xray first start: %v", err)
        }
        go sup.Watchdog(ctx)

        hub := api.NewHub()
        live := &api.LiveCounters{}
        api.StartCollector(ctx, cfg, pool, sup, hub, live)

        rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
        am := auth.NewManager(rdb, cfg.SessionHours)
        srv := api.NewServer(cfg, pool, am, sup, hub)
        srv.SyncCustomRoutes()

        handler := srv.Handler()
        plainLn, err := net.Listen("tcp", plainAddr)
        if err != nil {
                t.Fatalf("listen %s: %v", plainAddr, err)
        }
        go func() { _ = (&http.Server{Handler: handler}).Serve(plainLn) }()

        cert := selfSigned(t)
        tlsLn, err := net.Listen("tcp", tlsAddr)
        if err != nil {
                t.Fatalf("listen %s: %v", tlsAddr, err)
        }
        tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2", "http/1.1"}}
        go func() { _ = (&http.Server{Handler: handler}).Serve(tls.NewListener(tlsLn, tlsCfg)) }()

        // ── admin API session ────────────────────────────────────────────────
        jar, _ := cookiejar.New(nil)
        hc := &http.Client{
                Jar: jar, Timeout: 30 * time.Second,
                Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
        }
        apiURL := "http://" + plainAddr

        mustStatus(t, hc, "POST", apiURL+"/api/setup",
                `{"username":"admin","password":"admin123"}`, 200)
        t.Log("✓ setup: admin created")

        body := mustStatus(t, hc, "POST", apiURL+"/api/clients",
                `{"name":"AKIRAVX","quota_gb":10,"expire_days":30}`, 200)
        var created struct {
                ID       string `json:"id"`
                SubToken string `json:"sub_token"`
        }
        if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
                t.Fatalf("create client: %v / %s", err, body)
        }
        t.Logf("✓ client AKIRAVX created id=%s", created.ID)

        // rebuild config so the new user enters the Xray inbounds
        mustStatus(t, hc, "POST", apiURL+"/api/restart", `{}`, 200)
        waitXrayUp(t, sup)
        t.Log("✓ xray restarted with the client inside the config")

        // ── links & state ────────────────────────────────────────────────────
        body = mustStatus(t, hc, "GET", apiURL+"/api/clients/"+created.ID+"/links", "", 200)
        var lr struct {
                Links []struct {
                        Key string `json:"key"`
                        URL string `json:"url"`
                } `json:"links"`
        }
        _ = json.Unmarshal(body, &lr)
        if len(lr.Links) != 6 {
                t.Fatalf("expected 6 links, got %d: %s", len(lr.Links), body)
        }
        t.Log("✓ /api/clients/{id}/links → 6 links (all protocols)")

        body = mustStatus(t, hc, "GET", apiURL+"/api/state", "", 200)
        var st struct {
                Xray struct {
                        Running bool `json:"running"`
                } `json:"xray"`
                Reality struct {
                        Pub string `json:"pub"`
                        SID string `json:"sid"`
                        SNI string `json:"sni"`
                } `json:"reality"`
        }
        _ = json.Unmarshal(body, &st)
        if !st.Xray.Running {
                t.Fatalf("state says xray not running: %s", body)
        }
        if st.Reality.Pub == "" || st.Reality.SID == "" || st.Reality.SNI == "" {
                t.Fatalf("state missing reality keys: %s", body)
        }
        t.Logf("✓ /api/state → xray running + reality keys present (sni=%s)", st.Reality.SNI)

        // ── real client tunnel through the Go proxy ────────────────────────
        // Xray v25+ removed allowInsecure — pin the self-signed test cert via
        // its SHA-256 fingerprint instead.
        pin := fmt.Sprintf("%x", sha256.Sum256(cert.Certificate[0]))
        clientCfg := buildClientConfig(created.ID, st.Reality.Pub, st.Reality.SID, st.Reality.SNI, pin)
        raw, _ := json.MarshalIndent(clientCfg, "", " ")
        if err := writeBin(base+"/client.json", raw); err != nil {
                t.Fatal(err)
        }
        if out, err := exec.Command(cfg.XrayBin, "run", "-test", "-c", base+"/client.json").CombinedOutput(); err != nil {
                t.Fatalf("client config invalid: %v\n%s", err, out)
        }
        proc, err := launchClient(cfg.XrayBin, base+"/client.json")
        if err != nil {
                t.Fatalf("launch client xray: %v", err)
        }
        clientProc := proc
        defer func() { _ = clientProc.Process.Kill() }()

        for _, name := range []string{"vless-ws", "vless-xhttp", "vless-hu", "vmess-ws", "trojan-ws", "vless-reality"} {
                if err := waitTCP(socks[name], 30*time.Second); err != nil {
                        t.Fatalf("client socks %s (:%d) never came up: %v\nclient xray output:\n%s",
                                name, socks[name], err, strings.Join(clientProc.tail.snapshot(), "\n"))
                }
        }
        t.Log("✓ client Xray up: 6 SOCKS inbounds listening")

        for _, name := range []string{"vless-ws", "vless-xhttp", "vless-hu", "vmess-ws", "trojan-ws", "vless-reality"} {
                ok, code, err := fetchThroughSocks(socks[name], "http://cp.cloudflare.com/generate_204")
                if err != nil || !ok {
                        t.Errorf("✗ %s through tunnel: status=%d err=%v\nclient xray output:\n%s\nserver xray log:\n%s",
                                name, code, err,
                                strings.Join(clientProc.tail.snapshot(), "\n"),
                                strings.Join(sup.TailLog(400), "\n"))
                        continue
                }
                t.Logf("✓ %s tunnel → HTTP %d", name, code)
        }

        // ── traffic stats must reach Postgres ────────────────────────────────
        time.Sleep(6 * time.Second)
        body = mustStatus(t, hc, "GET", apiURL+"/api/state", "", 200)
        _ = json.Unmarshal(body, &st)
        var traffic struct {
                Traffic struct {
                        Up   int64 `json:"up"`
                        Down int64 `json:"down"`
                } `json:"traffic"`
        }
        _ = json.Unmarshal(body, &traffic)
        if traffic.Traffic.Up+traffic.Traffic.Down <= 0 {
                t.Fatalf("traffic counters are empty after real transfers: %s", body)
        }
        t.Logf("✓ collector persisted traffic: up=%d down=%d bytes",
                traffic.Traffic.Up, traffic.Traffic.Down)

        // ── public subscription endpoint ─────────────────────────────────────
        req, _ := http.NewRequest("GET", apiURL+"/sub/"+created.SubToken, nil)
        resp, err := hc.Do(req)
        if err != nil {
                t.Fatalf("subscription: %v", err)
        }
        subBody, _ := io.ReadAll(resp.Body)
        resp.Body.Close()
        if resp.StatusCode != 200 {
                t.Fatalf("subscription status %d: %s", resp.StatusCode, subBody)
        }
        if ui := resp.Header.Get("Subscription-Userinfo"); !strings.Contains(ui, "upload=") || !strings.Contains(ui, "total=") {
                t.Fatalf("Subscription-Userinfo incomplete: %q", ui)
        }
        decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(subBody)))
        if err != nil {
                t.Fatalf("subscription body is not base64: %v", err)
        }
        lines := nonEmpty(strings.Split(string(decoded), "\n"))
        if len(lines) != 6 {
                t.Fatalf("subscription expected 6 links, got %d: %v", len(lines), lines)
        }
        t.Logf("✓ /sub/{token} → 200, 6 links, userinfo=%q", resp.Header.Get("Subscription-Userinfo"))

        // ── custom inbound: create → routed by the Go proxy → delete ────────
        body = mustStatus(t, hc, "POST", apiURL+"/api/inbounds",
                `{"name":"C1","protocol":"vless","network":"ws","security":"tls","path":"/cx1"}`, 200)
        var ibResp struct {
                Inbound struct {
                        Tag string `json:"tag"`
                } `json:"inbound"`
        }
        _ = json.Unmarshal(body, &ibResp)
        if ibResp.Inbound.Tag == "" {
                t.Fatalf("custom inbound create failed: %s", body)
        }
        time.Sleep(2 * time.Second)
        cResp, err := hc.Get("https://" + tlsAddr + "/cx1")
        if err != nil {
                t.Fatalf("custom path proxy: %v", err)
        }
        cResp.Body.Close()
        if cResp.StatusCode == 502 {
                t.Fatalf("custom path /cx1 hit the proxy but the backend looked dead (%d)", cResp.StatusCode)
        }
        t.Logf("✓ custom inbound /cx1 routed to Xray (HTTP %d from the ws inbound)", cResp.StatusCode)

        mustStatus(t, hc, "DELETE", apiURL+"/api/inbounds/"+ibResp.Inbound.Tag, "", 200)
        t.Log("✓ custom inbound deleted")

        // logs + server-config sanity
        mustStatus(t, hc, "GET", apiURL+"/api/logs", "", 200)
        mustStatus(t, hc, "GET", apiURL+"/api/server-config", "", 200)
        t.Log("✓ /api/logs + /api/server-config")

        // session must survive restarts because it lives in Redis (mirror check)
        mustStatus(t, hc, "GET", apiURL+"/api/me", "", 200)
        t.Log("✓ session from Redis")

        cancel()
        sup.Stop()
        t.Log("ALL E2E STEPS PASSED")
}

// ───────────────────────── helpers ─────────────────────────

func mustStatus(t *testing.T, c *http.Client, method, url, body string, want int) []byte {
        t.Helper()
        var rd io.Reader
        if body != "" {
                rd = strings.NewReader(body)
        }
        req, err := http.NewRequest(method, url, rd)
        if err != nil {
                t.Fatalf("%s %s: %v", method, url, err)
        }
        if body != "" {
                req.Header.Set("Content-Type", "application/json")
        }
        resp, err := c.Do(req)
        if err != nil {
                t.Fatalf("%s %s: %v", method, url, err)
        }
        defer resp.Body.Close()
        raw, _ := io.ReadAll(resp.Body)
        if resp.StatusCode != want {
                t.Fatalf("%s %s: want %d got %d: %s", method, url, want, resp.StatusCode, raw)
        }
        return raw
}

func waitXrayUp(t *testing.T, sup *xray.Supervisor) {
        t.Helper()
        deadline := time.Now().Add(60 * time.Second)
        for time.Now().Before(deadline) {
                if sup.Running() && waitTCP(appPort, 2*time.Second) == nil {
                        return
                }
                time.Sleep(500 * time.Millisecond)
        }
        t.Fatalf("xray never became ready; log tail: %v", sup.TailLog(20))
}

func waitTCP(port int, d time.Duration) error {
        deadline := time.Now().Add(d)
        for time.Now().Before(deadline) {
                c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
                if err == nil {
                        c.Close()
                        return nil
                }
                time.Sleep(200 * time.Millisecond)
        }
        return fmt.Errorf("port %d not open in %v", port, d)
}

func fetchThroughSocks(port int, url string) (bool, int, error) {
        d, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, proxy.Direct)
        if err != nil {
                return false, 0, err
        }
        tr := &http.Transport{}
        if cd, ok := d.(proxy.ContextDialer); ok {
                tr.DialContext = cd.DialContext
        } else {
                tr.Dial = d.Dial
        }
        cl := &http.Client{Transport: tr, Timeout: 45 * time.Second}
        resp, err := cl.Get(url)
        if err != nil {
                return false, 0, err
        }
        defer resp.Body.Close()
        io.Copy(io.Discard, resp.Body)
        return resp.StatusCode == 204, resp.StatusCode, nil
}

func buildClientConfig(cid, pbk, sid, sni, pin string) map[string]any {
        socksIn := func(tag string, port int) map[string]any {
                return map[string]any{
                        "tag": tag, "listen": "127.0.0.1", "port": port, "protocol": "socks",
                        "settings": map[string]any{"auth": "noauth", "udp": false, "userLevel": 0},
                        "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls"}, "routeOnly": false},
                }
        }
        tlsS := map[string]any{"serverName": "127.0.0.1", "pinnedPeerCertSha256": pin, "fingerprint": "chrome"}
        out := []map[string]any{
                {"tag": "out-ws", "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": "127.0.0.1", "port": 18443,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": "", "level": 0}}}}},
                        "streamSettings": map[string]any{"network": "ws", "security": "tls",
                                "tlsSettings": tlsS,
                                "wsSettings":  map[string]any{"path": "/ws?ed=2048", "headers": map[string]string{"Host": "127.0.0.1"}}}},
                {"tag": "out-xhttp", "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": "127.0.0.1", "port": 18443,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": "", "level": 0}}}}},
                        "streamSettings": map[string]any{"network": "xhttp", "security": "tls",
                                "tlsSettings":   map[string]any{"serverName": "127.0.0.1", "pinnedPeerCertSha256": pin, "fingerprint": "chrome", "alpn": []string{"h2", "http/1.1"}},
                                "xhttpSettings": map[string]any{"path": "/xhttp", "mode": "packet-up", "host": "127.0.0.1"}}},
                {"tag": "out-hu", "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": "127.0.0.1", "port": 18443,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": "", "level": 0}}}}},
                        "streamSettings": map[string]any{"network": "httpupgrade", "security": "tls",
                                "tlsSettings":         map[string]any{"serverName": "127.0.0.1", "pinnedPeerCertSha256": pin, "fingerprint": "chrome"},
                                "httpupgradeSettings": map[string]any{"path": "/hu", "host": "127.0.0.1"}}},
                {"tag": "out-vmess", "protocol": "vmess",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": "127.0.0.1", "port": 18443,
                                "users": []map[string]any{{"id": cid, "alterId": 0, "security": "auto", "level": 0}}}}},
                        "streamSettings": map[string]any{"network": "ws", "security": "tls",
                                "tlsSettings": map[string]any{"serverName": "127.0.0.1", "pinnedPeerCertSha256": pin, "fingerprint": "chrome"},
                                "wsSettings":  map[string]any{"path": "/vmess?ed=2048", "headers": map[string]string{"Host": "127.0.0.1"}}}},
                {"tag": "out-trojan", "protocol": "trojan",
                        "settings": map[string]any{"servers": []map[string]any{{"address": "127.0.0.1", "port": 18443, "password": cid, "level": 0}}},
                        "streamSettings": map[string]any{"network": "ws", "security": "tls",
                                "tlsSettings": map[string]any{"serverName": "127.0.0.1", "pinnedPeerCertSha256": pin, "fingerprint": "chrome"},
                                "wsSettings":  map[string]any{"path": "/trojan?ed=2048", "headers": map[string]string{"Host": "127.0.0.1"}}}},
                {"tag": "out-reality", "protocol": "vless",
                        "settings": map[string]any{"vnext": []map[string]any{{"address": "127.0.0.1", "port": appPort,
                                "users": []map[string]any{{"id": cid, "encryption": "none", "flow": "xtls-rprx-vision", "level": 0}}}}},
                        "streamSettings": map[string]any{"network": "tcp", "security": "reality",
                                "realitySettings": map[string]any{"show": false, "fingerprint": "chrome",
                                        "serverName": sni, "publicKey": pbk, "shortId": sid, "spiderX": "/"},
                                "sockopt": map[string]any{"tcpNoDelay": true}}},
        }
        rules := []map[string]any{}
        for tag, out := range map[string]string{
                "s-ws": "out-ws", "s-xhttp": "out-xhttp", "s-hu": "out-hu",
                "s-vmess": "out-vmess", "s-trojan": "out-trojan", "s-reality": "out-reality",
        } {
                rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{tag}, "outboundTag": out})
        }
        inbounds := []map[string]any{}
        for _, p := range []struct {
                tag  string
                port int
        }{{"s-ws", socks["vless-ws"]}, {"s-xhttp", socks["vless-xhttp"]}, {"s-hu", socks["vless-hu"]},
                {"s-vmess", socks["vmess-ws"]}, {"s-trojan", socks["trojan-ws"]}, {"s-reality", socks["vless-reality"]}} {
                inbounds = append(inbounds, socksIn(p.tag, p.port))
        }
        return map[string]any{
                "log":      map[string]any{"loglevel": "info"},
                "inbounds": inbounds,
                "outbounds": append(out,
                        map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{"domainStrategy": "UseIPv4"}},
                        map[string]any{"tag": "block", "protocol": "blackhole"}),
                "routing": map[string]any{"domainStrategy": "AsIs", "rules": rules},
        }
}

func launchClient(bin, conf string) (*execProc, error) {
        return startProc(bin, "run", "-c", conf)
}

func nonEmpty(xs []string) []string {
        out := []string{}
        for _, x := range xs {
                if strings.TrimSpace(x) != "" {
                        out = append(out, strings.TrimSpace(x))
                }
        }
        return out
}

func writeBin(path string, data []byte) error {
        return osWriteFile(path, data)
}

func selfSigned(t *testing.T) tls.Certificate {
        t.Helper()
        key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
        if err != nil {
                t.Fatal(err)
        }
        tmpl := x509.Certificate{
                SerialNumber:          big.NewInt(1),
                NotBefore:             time.Now().Add(-time.Hour),
                NotAfter:              time.Now().Add(24 * time.Hour),
                IsCA:                  true,
                BasicConstraintsValid: true,
                IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
                DNSNames:              []string{"localhost"},
                KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
                ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        }
        der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
        if err != nil {
                t.Fatal(err)
        }
        return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
