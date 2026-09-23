package api

import (
        "context"
        "crypto/tls"
        "net"
        "net/http"
        "net/http/httputil"
        "net/url"
        "strings"
        "sync"

        "golang.org/x/net/http2"
)

// ───────────────────────── reverse proxy (nginx replacement) ─────────────
// The platform edge terminates TLS and forwards /ws /xhttp /hu /vmess /trojan
// (plus custom inbound paths) to the local Xray inbounds — the same job nginx
// did in the Python panel, now in-process.

// h2cTransport lets the proxy reach gRPC inbounds over cleartext HTTP/2.
var h2cTransport http.RoundTripper = &http2.Transport{
        AllowHTTP: true,
        DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
                var d net.Dialer
                return d.DialContext(ctx, network, addr)
        },
}

func newProxy(target string, network string) *httputil.ReverseProxy {
        u, err := url.Parse("http://" + target)
        if err != nil {
                return nil
        }
        rp := &httputil.ReverseProxy{
                Rewrite: func(pr *httputil.ProxyRequest) {
                        pr.SetURL(u)
                        pr.Out.Host = pr.In.Host
                },
                // unbuffered streaming: websocket frames and xhttp payloads go through
                // immediately (nginx: proxy_buffering off / proxy_request_buffering off)
                FlushInterval: -1,
        }
        if network == "grpc" {
                rp.Transport = h2cTransport
        }
        return rp
}

// registerBuiltins wires the five built-in 443 paths to their fixed Xray ports.
// Each path is registered BOTH exact and as a subtree: recent XHTTP clients
// suffix the configured path with "/{session-uuid}" frames (nginx's
// "location ^~ /p" was a prefix match — the mux needs the "/" pattern for it).
func (s *Server) registerBuiltins(mux *http.ServeMux) {
        for _, p := range []struct {
                path, port, network string
        }{
                {s.Cfg.WSPath, "127.0.0.1:10001", "ws"},
                {s.Cfg.XHTTPPath, "127.0.0.1:10002", "xhttp"},
                {s.Cfg.VmessPath, "127.0.0.1:10004", "ws"},
                {s.Cfg.TrojanPath, "127.0.0.1:10005", "ws"},
                {s.Cfg.HUPath, "127.0.0.1:10006", "httpupgrade"},
        } {
                if rp := newProxy(p.port, p.network); rp != nil {
                        mux.Handle(p.path, rp)
                        if !strings.HasSuffix(p.path, "/") {
                                mux.Handle(p.path+"/", rp)
                        }
                }
        }
}

// ───────────────────────── custom inbound routing ─────────────────────────
// Custom inbounds are created/deleted at runtime, so their paths cannot live
// in the static mux; the outer handler resolves them by longest prefix.

type customRoute struct {
        path string // "/p"
        rp   *httputil.ReverseProxy
}

func (s *Server) initCustomRoutes() {
        s.customMu = &sync.Mutex{}
        s.customs = map[string]customRoute{}
}

// SyncCustomRoutes rebuilds the custom path registry from the DB.
func (s *Server) SyncCustomRoutes() {
        if s.customMu == nil || s.Pool == nil {
                return
        }
        rows, err := s.Pool.Query(context.Background(), `
                SELECT tag, path, network, COALESCE(lport,0) FROM inbounds WHERE security='tls'`)
        if err != nil {
                return
        }
        defer rows.Close()
        next := map[string]customRoute{}
        for rows.Next() {
                var tag, path, network string
                var lport int
                if err := rows.Scan(&tag, &path, &network, &lport); err != nil {
                        continue
                }
                if path == "" || lport == 0 {
                        continue
                }
                if rp := newProxy("127.0.0.1:"+itoa(lport), network); rp != nil {
                        next[path] = customRoute{path: path, rp: rp}
                }
        }
        s.customMu.Lock()
        s.customs = next
        s.customMu.Unlock()
}

func itoa(n int) string {
        if n == 0 {
                return "0"
        }
        neg := n < 0
        if neg {
                n = -n
        }
        var b [20]byte
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

// lookupCustom returns the proxy for the longest matching custom path.
func (s *Server) lookupCustom(p string) *httputil.ReverseProxy {
        if s.customMu == nil {
                return nil
        }
        s.customMu.Lock()
        defer s.customMu.Unlock()
        var best *httputil.ReverseProxy
        bestLen := 0
        for path, route := range s.customs {
                if strings.HasPrefix(p, path) && len(path) > bestLen {
                        best, bestLen = route.rp, len(path)
                }
        }
        return best
}

func (s *Server) serveMuxWithCustoms(mux *http.ServeMux) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                if rp := s.lookupCustom(r.URL.Path); rp != nil {
                        rp.ServeHTTP(w, r)
                        return
                }
                mux.ServeHTTP(w, r)
        })
}
