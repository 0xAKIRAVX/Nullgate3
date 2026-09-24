package api

import (
        "context"
        "encoding/json"
        "errors"
        "log"
        "math"
        "net"
        "net/http"
        "strings"
        "sync"
        "time"

        "github.com/google/uuid"
        "github.com/jackc/pgx/v5"
        "github.com/jackc/pgx/v5/pgxpool"

        "nullgate/api/internal/auth"
        "nullgate/api/internal/config"
        "nullgate/api/internal/store"
        "nullgate/api/internal/xray"
)

const Version = "3.0.0-alpha.3"

type Server struct {
        Cfg  *config.Config
        Pool *pgxpool.Pool
        Auth *auth.Manager
        Sup  *xray.Supervisor
        Hub  *Hub

        // custom inbound path registry (rebuilt from the DB on change)
        customMu *sync.Mutex
        customs  map[string]customRoute

        limiter *loginLimiter
}

func NewServer(cfg *config.Config, pool *pgxpool.Pool, am *auth.Manager, sup *xray.Supervisor, hub *Hub) *Server {
        s := &Server{
                Cfg:     cfg,
                Pool:    pool,
                Auth:    am,
                Sup:     sup,
                Hub:     hub,
                limiter: newLoginLimiter(8, 10*time.Minute),
        }
        s.initCustomRoutes()
        return s
}

// ───────────────────────── plumbing ─────────────────────────

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()

        mux.HandleFunc("GET /api/health", s.health)
        mux.HandleFunc("/api/grpc-probe", s.grpcProbe)
        mux.HandleFunc("GET /api/setup/status", s.setupStatus)
        mux.HandleFunc("POST /api/setup", s.setup)
        mux.HandleFunc("POST /api/login", s.login)
        mux.HandleFunc("POST /api/logout", s.authed(s.logout))
        mux.HandleFunc("GET /api/me", s.authed(s.me))
        mux.HandleFunc("GET /api/state", s.authed(s.state))
        mux.HandleFunc("GET /api/settings", s.authed(s.getSettings))
        mux.HandleFunc("PUT /api/settings", s.authed(s.putSettings))
        mux.HandleFunc("GET /api/clients", s.authed(s.listClients))
        mux.HandleFunc("POST /api/clients", s.authed(s.createClient))
        mux.HandleFunc("GET /api/clients/{id}", s.authed(s.getClient))
        mux.HandleFunc("PATCH /api/clients/{id}", s.authed(s.patchClient))
        mux.HandleFunc("DELETE /api/clients/{id}", s.authed(s.deleteClient))
        mux.HandleFunc("GET /api/clients/{id}/links", s.authed(s.clientLinks))
        mux.HandleFunc("GET /api/clients/{id}/config", s.authed(s.clientConfig))
        mux.HandleFunc("GET /api/inbounds", s.authed(s.listInbounds))
        mux.HandleFunc("POST /api/inbounds", s.authed(s.createInbound))
        mux.HandleFunc("DELETE /api/inbounds/{tag}", s.authed(s.deleteInbound))
        mux.HandleFunc("GET /api/server-config", s.authed(s.serverConfig))
        mux.HandleFunc("GET /api/logs", s.authed(s.logs))
        mux.HandleFunc("POST /api/restart", s.authed(s.restart))
        mux.HandleFunc("GET /api/ws", s.authed(s.liveWS))
        mux.HandleFunc("GET /sub/", s.sub)

        s.registerBuiltins(mux)
        return s.cors(s.headers(s.serveMuxWithCustoms(mux)))
}

func (s *Server) cors(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Add("Vary", "Origin")
                if origin := r.Header.Get("Origin"); origin != "" {
                        // Only a strict, explicitly configured origin may call the API
                        // cross-site WITH credentials. The default ("*") keeps the panel
                        // same-origin-only — reflecting arbitrary origins together with
                        // Allow-Credentials would let any website hijack the admin session
                        // cookie (the UI is embedded and served same-origin, so the default
                        // costs nothing).
                        if s.Cfg.CORSOrigin != "" && s.Cfg.CORSOrigin != "*" && origin == s.Cfg.CORSOrigin {
                                w.Header().Set("Access-Control-Allow-Origin", origin)
                                w.Header().Set("Access-Control-Allow-Credentials", "true")
                        }
                }
                if r.Method == http.MethodOptions {
                        w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
                        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
                        w.WriteHeader(http.StatusNoContent)
                        return
                }
                next.ServeHTTP(w, r)
        })
}

func (s *Server) headers(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("X-Content-Type-Options", "nosniff")
                w.Header().Set("Cache-Control", "no-store")
                next.ServeHTTP(w, r)
        })
}

type ctxKey string

const adminKey ctxKey = "admin"

func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                c, err := r.Cookie("ng_session")
                if err == nil {
                        if id, ok := s.Auth.Get(r.Context(), c.Value); ok {
                                next(w, r.WithContext(contextWith(r, adminKey, id)))
                                return
                        }
                }
                writeErr(w, http.StatusUnauthorized, "unauthorized")
        }
}

func writeJSON(w http.ResponseWriter, code int, v any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(code)
        _ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
        writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
        r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
        dec := json.NewDecoder(r.Body)
        if err := dec.Decode(dst); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid json body")
                return false
        }
        return true
}

// ───────────────────────── auth handlers ─────────────────────────

// grpcProbe is a diagnostic endpoint that speaks minimal gRPC framing: it
// answers with content-type application/grpc and grpc-status trailers. If a
// client receives the trailers, every hop in front of the API server (platform
// edge included) correctly carried an HTTP/2 gRPC exchange — isolating gRPC
// failures to the Xray leg rather than ingress.
func (s *Server) grpcProbe(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/grpc")
        w.Header().Set("Trailer", "grpc-status, grpc-message")
        w.Header().Set("X-Server-Proto", r.Proto)
        w.WriteHeader(http.StatusOK)
        if f, ok := w.(http.Flusher); ok {
                f.Flush()
        }
        w.Header().Set("grpc-status", "12") // UNIMPLEMENTED — framing test only, no payload
        w.Header().Set("grpc-message", "probe-ok")
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
        redisState := "off"
        if s.Auth.RedisAlive(r.Context()) {
                redisState = "on"
        }
        dbOK := true
        if s.Pool != nil {
                if err := s.Pool.Ping(r.Context()); err != nil {
                        dbOK = false
                }
        }
        code := http.StatusOK
        if !dbOK {
                // HTTP-based monitors (Railway healthcheck) rely on a failing status.
                code = http.StatusServiceUnavailable
        }
        writeJSON(w, code, map[string]any{
                "ok": dbOK, "version": Version, "db": dbOK, "redis": redisState,
                "proto": r.Proto,
        })
}

func (s *Server) adminCount(r *http.Request) (int64, error) {
        var n int64
        err := s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM admins`).Scan(&n)
        return n, err
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
        n, err := s.adminCount(r)
        if err != nil {
                writeInternal(w, "setup status", err)
                return
        }
        writeJSON(w, 200, map[string]any{"needs_setup": n == 0})
}

// writeInternal logs the real error server-side and answers with a generic
// 500 — pre-auth endpoints must not mirror Postgres driver/DSN internals to
// anonymous callers.
func writeInternal(w http.ResponseWriter, where string, err error) {
        log.Printf("api: %s: %v", where, err)
        writeErr(w, 500, "internal error")
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
        n, err := s.adminCount(r)
        if err != nil {
                writeInternal(w, "setup", err)
                return
        }
        if n > 0 {
                writeErr(w, http.StatusForbidden, "setup already completed")
                return
        }
        var body struct {
                Username string `json:"username"`
                Password string `json:"password"`
        }
        if !readJSON(w, r, &body) {
                return
        }
        body.Username = strings.TrimSpace(strings.ToLower(body.Username))
        if len(body.Username) < 3 || len(body.Password) < 6 {
                writeErr(w, 400, "username must be 3+ chars and password 6+ chars")
                return
        }
        hash, err := auth.HashPassword(body.Password)
        if err != nil {
                writeInternal(w, "setup hash", err)
                return
        }
        var id int64
        err = s.Pool.QueryRow(r.Context(), `
                INSERT INTO admins(username, password_hash)
                SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM admins)
                RETURNING id`,
                body.Username, hash).Scan(&id)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        // TOCTOU guard: a concurrent setup request already created the admin.
                        writeErr(w, http.StatusForbidden, "setup already completed")
                        return
                }
                writeInternal(w, "setup insert", err)
                return
        }
        if err := s.startSession(w, r, id); err != nil {
                return
        }
        writeJSON(w, 200, map[string]any{"ok": true, "username": body.Username})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
        ip := clientIP(r)
        if !s.limiter.allow(ip) {
                writeErr(w, http.StatusTooManyRequests, "too many attempts, try later")
                return
        }
        var body struct {
                Username string `json:"username"`
                Password string `json:"password"`
        }
        if !readJSON(w, r, &body) {
                return
        }
        var (
                id   int64
                hash string
        )
        err := s.Pool.QueryRow(r.Context(),
                `SELECT id, password_hash FROM admins WHERE username=$1`,
                strings.TrimSpace(strings.ToLower(body.Username))).Scan(&id, &hash)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        // burn the same bcrypt cost as the real path so response time does
                        // not reveal whether the username exists
                        _ = auth.CheckPassword(dummyBcryptHash, body.Password)
                        writeErr(w, 401, "wrong username or password")
                        return
                }
                writeInternal(w, "login", err)
                return
        }
        if !auth.CheckPassword(hash, body.Password) {
                writeErr(w, 401, "wrong username or password")
                return
        }
        s.limiter.reset(ip)
        if err := s.startSession(w, r, id); err != nil {
                return
        }
        writeJSON(w, 200, map[string]any{"ok": true})
}

// dummyBcryptHash exists solely to equalize login response timing.
var dummyBcryptHash, _ = auth.HashPassword("nullgate-timing-" + auth.RandomHex(8))

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, adminID int64) error {
        tok, err := s.Auth.Create(r.Context(), adminID)
        if err != nil {
                writeInternal(w, "session create", err)
                return err
        }
        cookie := &http.Cookie{
                Name:     "ng_session",
                Value:    tok,
                Path:     "/",
                HttpOnly: true,
                MaxAge:   s.Cfg.SessionHours * 3600,
        }
        if s.Cfg.InsecureCookies {
                cookie.SameSite = http.SameSiteLaxMode
        } else {
                cookie.SameSite = http.SameSiteNoneMode
                cookie.Secure = true
        }
        http.SetCookie(w, cookie)
        return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
        if c, err := r.Cookie("ng_session"); err == nil {
                s.Auth.Delete(r.Context(), c.Value)
        }
        http.SetCookie(w, &http.Cookie{Name: "ng_session", Value: "", Path: "/", MaxAge: -1})
        writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
        adminID, ok := r.Context().Value(adminKey).(int64)
        if !ok {
                writeErr(w, 401, "unauthorized")
                return
        }
        var username string
        err := s.Pool.QueryRow(r.Context(),
                `SELECT username FROM admins WHERE id=$1`,
                adminID).Scan(&username)
        if err != nil {
                writeErr(w, 401, "session expired")
                return
        }
        writeJSON(w, 200, map[string]any{"username": username, "role": "admin"})
}

func clientIP(r *http.Request) string {
        // Trust only the RIGHTMOST XFF entry: it is appended by the closest proxy
        // (the platform edge). The first entry is client-controlled and trivially
        // spoofed, which would let anyone bypass the login rate limiter by
        // rotating fake IPs.
        if v := r.Header.Get("X-Forwarded-For"); v != "" {
                parts := strings.Split(v, ",")
                return strings.TrimSpace(parts[len(parts)-1])
        }
        return r.RemoteAddr
}

// loginLimiter — fixed-window, per-IP (memory).
type loginLimiter struct {
        mu     sync.Mutex
        max    int
        window time.Duration
        hits   map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
        return &loginLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *loginLimiter) allow(key string) bool {
        l.mu.Lock()
        defer l.mu.Unlock()
        now := time.Now()
        keep := l.hits[key][:0]
        for _, t := range l.hits[key] {
                if now.Sub(t) < l.window {
                        keep = append(keep, t)
                }
        }
        l.hits[key] = keep
        if len(keep) >= l.max {
                return false
        }
        l.hits[key] = append(l.hits[key], now)
        // bound memory: drop keys with no recent hits once the map grows large
        // (spoofed-IP attempts would otherwise leak entries forever)
        if len(l.hits) > 4096 {
                for k, v := range l.hits {
                        if len(v) == 0 {
                                delete(l.hits, k)
                        }
                }
        }
        return true
}

func (l *loginLimiter) reset(key string) {
        l.mu.Lock()
        delete(l.hits, key)
        l.mu.Unlock()
}

// ───────────────────────── state / settings ─────────────────────────

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        panel, err := store.LoadPanelSettings(ctx, s.Pool, s.Cfg.RealitySNI, s.Cfg.SessionHours)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        reality := map[string]any{}
        if raw, err := store.GetJSON(ctx, s.Pool, "reality"); err == nil {
                _ = json.Unmarshal(raw, &reality)
        }
        var total, active int64
        var up, down int64
        if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM clients`).Scan(&total); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if err := s.Pool.QueryRow(ctx, `
                SELECT count(*) FROM clients c
                WHERE (c.expire_at IS NULL OR c.expire_at > now())
                  AND (c.quota = 0 OR c.quota > COALESCE((SELECT u.up+u.down FROM client_usage u WHERE u.client_id=c.id),0))
        `).Scan(&active); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if err := s.Pool.QueryRow(ctx,
                `SELECT COALESCE(SUM(up),0), COALESCE(SUM(down),0) FROM client_usage`).Scan(&up, &down); err != nil {
                writeErr(w, 500, err.Error())
                return
        }

        writeJSON(w, 200, map[string]any{
                "version":   Version,
                "protocols": panel["protocols"],
                "reality": map[string]any{
                        "pub": reality["pub"], "sid": reality["sid"], "sni": panel["reality_sni"],
                },
                "ports": map[string]any{"app": s.Cfg.AppPort, "api": s.Cfg.APIPort},
                "tcp": map[string]any{
                        "host": s.Cfg.TCPHost, "port": s.Cfg.TCPPublicPort,
                        "ready": s.Cfg.TCPHost != "" && s.Cfg.TCPPublicPort != "",
                },
                "clients": map[string]any{
                        "total": total, "active": active,
                },
                "traffic": map[string]any{"up": up, "down": down},
                "xray":    s.Sup.Info(),
        })
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
        panel, err := store.LoadPanelSettings(r.Context(), s.Pool, s.Cfg.RealitySNI, s.Cfg.SessionHours)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, panel)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
        var body map[string]any
        if !readJSON(w, r, &body) {
                return
        }
        ctx := r.Context()
        panel, err := store.LoadPanelSettings(ctx, s.Pool, s.Cfg.RealitySNI, s.Cfg.SessionHours)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if v, ok := body["protocols"].(map[string]any); ok {
                // panel comes from json.Unmarshal, so nested maps are map[string]any.
                // Merge the STORED toggles first — a partial body must not reset the
                // protocols the admin already disabled.
                merged := map[string]bool{}
                if cur, ok := panel["protocols"].(map[string]any); ok {
                        for k, bv := range cur {
                                if b, ok := bv.(bool); ok {
                                        merged[k] = b
                                }
                        }
                } else {
                        for k, b := range store.DefaultPanelSettings(s.Cfg.RealitySNI, s.Cfg.SessionHours)["protocols"].(map[string]bool) {
                                merged[k] = b
                        }
                }
                for k, bv := range v {
                        if b, ok := bv.(bool); ok {
                                merged[k] = b
                        }
                }
                panel["protocols"] = merged
        }
        if v, ok := body["reality_sni"].(string); ok && strings.TrimSpace(v) != "" {
                sni := strings.ToLower(strings.TrimSpace(v))
                // reality_sni feeds dest/serverNames of the builtin REALITY inbound
                // and every generated link — garbage here degrades the inbound and
                // produces dead links until manually fixed
                if !validPublicHost(sni) {
                        writeErr(w, 400, "reality_sni باید دامنه یا IP معتبر باشد")
                        return
                }
                panel["reality_sni"] = sni
        }
        if v, ok := body["cfg_fmt"].(string); ok {
                panel["cfg_fmt"] = v
        }
        if v, ok := body["address"].(string); ok {
                addr := strings.TrimSpace(v)
                if addr != "" && !validPublicHost(strings.ToLower(addr)) {
                        writeErr(w, 400, "address باید دامنه یا IP معتبر باشد")
                        return
                }
                panel["address"] = addr
        }
        if err := store.PutJSON(ctx, s.Pool, "panel", panel); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        // protocol/SNI changes must reach the running Xray process; Restart's
        // fingerprint check makes this a no-op when nothing actually changed.
        s.restartXray()
        writeJSON(w, 200, panel)
}

// validPublicHost accepts a lowercase FQDN, an IPv4 or a bare/bracketed IPv6 —
// the three legal shapes for a public address/SNI.
func validPublicHost(v string) bool {
        if v == "" {
                return false
        }
        if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
                v = v[1 : len(v)-1]
        }
        if net.ParseIP(v) != nil {
                return true
        }
        return hostRe.MatchString(v)
}

// restartXray rebuilds + restarts Xray detached from the HTTP request
// lifetime. If the admin's browser disconnects mid-request the rebuild must
// still happen: the collector only re-syncs on user-ID-set changes, so
// protocol/path/inbound changes would otherwise never be retried.
func (s *Server) restartXray() {
        if s.Sup == nil {
                return
        }
        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        if err := s.Sup.Restart(ctx, false); err != nil {
                log.Printf("api: xray restart: %v", err)
        }
}

// ───────────────────────── clients ─────────────────────────

type clientRow struct {
        ID        string   `json:"id"`
        Name      string   `json:"name"`
        Quota     int64    `json:"quota"`
        ExpireAt  *string  `json:"expire_at"`
        Protocols []string `json:"protocols"`
        Note      string   `json:"note"`
        SubToken  string   `json:"sub_token"`
        CreatedAt string   `json:"created_at"`
        Up        int64    `json:"up"`
        Down      int64    `json:"down"`
        Status    string   `json:"status"`
        UsagePct  *float64 `json:"usage_pct"`
}

const clientCols = `c.id::text, c.name, c.quota,
        COALESCE(c.expire_at, '1970-01-01'::timestamptz),
        c.protocols::text, c.note, c.sub_token, c.created_at,
        COALESCE(u.up,0), COALESCE(u.down,0)`

const clientFrom = `FROM clients c LEFT JOIN client_usage u ON u.client_id = c.id`

func scanClient(row pgx.Row) (*clientRow, error) {
        var (
                c        clientRow
                expireAt time.Time
                protoRaw []byte
                created  time.Time
        )
        if err := row.Scan(&c.ID, &c.Name, &c.Quota, &expireAt, &protoRaw, &c.Note,
                &c.SubToken, &created, &c.Up, &c.Down); err != nil {
                return nil, err
        }
        _ = json.Unmarshal(protoRaw, &c.Protocols)
        // a NULL jsonb column (raw-API user created without protocols) would
        // otherwise marshal back as JSON null and crash the panel's table —
        // normalize to the empty slice, which means "follow global" everywhere.
        if c.Protocols == nil {
                c.Protocols = []string{}
        }
        c.CreatedAt = created.UTC().Format(time.RFC3339)
        if expireAt.Year() > 1971 {
                iso := expireAt.UTC().Format(time.RFC3339)
                c.ExpireAt = &iso
        }
        switch {
        case c.Quota > 0 && c.Up+c.Down >= c.Quota:
                c.Status = "quota"
        case expireAt.Year() > 1971 && time.Now().After(expireAt):
                c.Status = "expired"
        default:
                c.Status = "active"
        }
        if c.Quota > 0 {
                p := math.Round(float64(c.Up+c.Down)/float64(c.Quota)*1000) / 10
                c.UsagePct = &p
        }
        return &c, nil
}

func (s *Server) listClients(w http.ResponseWriter, r *http.Request) {
        rows, err := s.Pool.Query(r.Context(),
                `SELECT `+clientCols+` `+clientFrom+` ORDER BY c.created_at DESC`)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        defer rows.Close()
        out := []*clientRow{}
        for rows.Next() {
                c, err := scanClient(rows)
                if err != nil {
                        writeErr(w, 500, err.Error())
                        return
                }
                out = append(out, c)
        }
        if err := rows.Err(); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, out)
}

func (s *Server) createClient(w http.ResponseWriter, r *http.Request) {
        var body struct {
                Name       string   `json:"name"`
                QuotaGB    float64  `json:"quota_gb"`
                ExpireDays int      `json:"expire_days"`
                Protocols  []string `json:"protocols"`
                Note       string   `json:"note"`
        }
        if !readJSON(w, r, &body) {
                return
        }
        body.Name = strings.TrimSpace(body.Name)
        if body.Name == "" {
                writeErr(w, 400, "name is required")
                return
        }
        // sanity bound: huge values would overflow int64 bytes and silently become
        // "unlimited" (negative quota clamped to 0)
        if body.QuotaGB < 0 || body.QuotaGB > 1<<20 {
                writeErr(w, 400, "quota_gb must be between 0 and 1048576")
                return
        }
        // make_interval(days => $3) overflows timestamptz around ~2^31 days and
        // makes Postgres 500 instead of rejecting the input — bound it (~100 years)
        if body.ExpireDays < 0 || body.ExpireDays > 36500 {
                writeErr(w, 400, "expire_days must be between 0 and 36500")
                return
        }
        if !s.validProtocolTags(r.Context(), body.Protocols) {
                writeErr(w, 400, "پروتکل نامعتبر است — فقط پروتکل‌های داخلی یا تگ اینباند سفارشی")
                return
        }
        protoRaw, _ := json.Marshal(body.Protocols)
        var expire interface{}
        if body.ExpireDays > 0 {
                expire = body.ExpireDays
        }
        quota := int64(body.QuotaGB * (1 << 30))
        if quota < 0 {
                quota = 0
        }
        token := auth.RandomHex(16)

        row := s.Pool.QueryRow(r.Context(), `
                WITH ins AS (
                        INSERT INTO clients(name, quota, expire_at, protocols, note, sub_token)
                        VALUES($1,$2,
                                CASE WHEN $3::int IS NULL THEN NULL ELSE now() + make_interval(days=>$3::int) END,
                                $4::jsonb, $5, $6)
                        RETURNING id, name, quota, COALESCE(expire_at,'1970-01-01'::timestamptz),
                                protocols::text, note, sub_token, created_at, 0, 0
                )
                SELECT * FROM ins`, body.Name, quota, expire, string(protoRaw), body.Note, token)

        c, err := scanClient(row)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        _, _ = s.Pool.Exec(r.Context(),
                `INSERT INTO client_usage(client_id) VALUES($1::uuid) ON CONFLICT DO NOTHING`, c.ID)
        // make the new credentials live IMMEDIATELY — waiting for the collector's
        // next sync (up to COLLECT_INTERVAL) handed the user dead links on connect
        s.restartXray()
        writeJSON(w, 200, c)
}

// validProtocolTags accepts the six builtin protocol keys plus the tags of
// existing custom inbounds (an unknown tag like "vless-relity" silently yields
// a user with no credentials and an empty link list, so reject it up front).
func (s *Server) validProtocolTags(ctx context.Context, protos []string) bool {
        if len(protos) == 0 {
                return true // nil/[] = follow global
        }
        known := map[string]bool{
                "vless-reality": true, "vless-ws": true, "vless-xhttp": true,
                "vless-hu": true, "vmess-ws": true, "trojan-ws": true,
        }
        rows, err := s.Pool.Query(ctx, `SELECT tag FROM inbounds`)
        if err == nil {
                defer rows.Close()
                for rows.Next() {
                        var tag string
                        if rows.Scan(&tag) == nil {
                                known[tag] = true
                        }
                }
        }
        for _, k := range protos {
                k = strings.TrimSpace(k)
                if k == "" || !known[k] {
                        return false
                }
        }
        return true
}

func (s *Server) patchClient(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        if _, err := uuid.Parse(id); err != nil {
                writeErr(w, 400, "invalid client id")
                return
        }
        var body struct {
                Name       *string  `json:"name"`
                QuotaGB    *float64 `json:"quota_gb"`
                ExpireDays *int     `json:"expire_days"`
                Protocols  []string `json:"protocols"`
                Note       *string  `json:"note"`
        }
        if !readJSON(w, r, &body) {
                return
        }
        var name, note, protoRaw interface{}
        if body.Name != nil {
                name = strings.TrimSpace(*body.Name)
                if name == "" {
                        // COALESCE($2, name) with '' would blank the stored name —
                        // createClient rejects empty, PATCH must stay consistent
                        writeErr(w, 400, "name is required")
                        return
                }
        }
        if body.Note != nil {
                note = *body.Note
        }
        var quota interface{}
        if body.QuotaGB != nil {
                if *body.QuotaGB < 0 || *body.QuotaGB > 1<<20 {
                        writeErr(w, 400, "quota_gb must be between 0 and 1048576")
                        return
                }
                q := int64(*body.QuotaGB * (1 << 30))
                if q < 0 {
                        q = 0
                }
                quota = q
        }
        if body.ExpireDays != nil && (*body.ExpireDays < 0 || *body.ExpireDays > 36500) {
                writeErr(w, 400, "expire_days must be between 0 and 36500")
                return
        }
        if body.Protocols != nil && !s.validProtocolTags(r.Context(), body.Protocols) {
                writeErr(w, 400, "پروتکل نامعتبر است — فقط پروتکل‌های داخلی یا تگ اینباند سفارشی")
                return
        }
        if body.Protocols != nil {
                raw, _ := json.Marshal(body.Protocols)
                protoRaw = string(raw)
        }
        tag, err := s.Pool.Exec(r.Context(), `
                UPDATE clients SET
                        name = COALESCE($2, name),
                        quota = COALESCE($3, quota),
                        expire_at = CASE
                                WHEN $4::int IS NULL THEN expire_at
                                WHEN $4::int <= 0 THEN NULL
                                ELSE now() + make_interval(days=>$4::int) END,
                        protocols = COALESCE($5::jsonb, protocols),
                        note = COALESCE($6, note)
                WHERE id = $1::uuid`, id, name, quota, body.ExpireDays, protoRaw, note)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if tag.RowsAffected() == 0 {
                writeErr(w, 404, "client not found")
                return
        }
        // protocol/protocol-membership changes must reach the running Xray process
        // (Restart's fingerprint check keeps this a no-op when nothing changed).
        s.restartXray()
        s.returnClient(w, r, id)
}

func (s *Server) deleteClient(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        if _, err := uuid.Parse(id); err != nil {
                writeErr(w, 400, "invalid client id")
                return
        }
        tag, err := s.Pool.Exec(r.Context(), `DELETE FROM clients WHERE id=$1::uuid`, id)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        if tag.RowsAffected() == 0 {
                writeErr(w, 404, "client not found")
                return
        }
        // the deleted user's credentials must stop working IMMEDIATELY — the
        // collector would otherwise leave the UUID valid for up to 30 more seconds
        s.restartXray()
        writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/clients/{id} — single client fetch (the route used to be missing:
// the request fell through to the SPA fallback and returned HTML).
func (s *Server) getClient(w http.ResponseWriter, r *http.Request) {
        id := r.PathValue("id")
        if _, err := uuid.Parse(id); err != nil {
                writeErr(w, 400, "invalid client id")
                return
        }
        row := s.Pool.QueryRow(r.Context(),
                `SELECT `+clientCols+` `+clientFrom+` WHERE c.id = $1::uuid`, id)
        c, err := scanClient(row)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        writeErr(w, 404, "client not found")
                } else {
                        writeErr(w, 500, err.Error())
                }
                return
        }
        writeJSON(w, 200, c)
}

func (s *Server) returnClient(w http.ResponseWriter, r *http.Request, id string) {
        row := s.Pool.QueryRow(r.Context(),
                `SELECT `+clientCols+` `+clientFrom+` WHERE c.id = $1::uuid`, id)
        c, err := scanClient(row)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, c)
}

// ───────────────────────── inbounds (read-only for now) ─────────────────────────

func (s *Server) listInbounds(w http.ResponseWriter, r *http.Request) {
        realityNet := s.Cfg.RealityNet
        if realityNet != "grpc" {
                realityNet = "tcp"
        }
        realityName := "Reality TCP"
        if realityNet == "grpc" {
                realityName = "Reality gRPC"
        }
        builtins := []map[string]any{
                {"tag": "vless-reality", "name": realityName, "protocol": "vless", "network": realityNet, "security": "reality", "port": s.Cfg.AppPort, "builtin": true},
                {"tag": "vless-ws", "name": "VLESS WS", "protocol": "vless", "network": "ws", "security": "none", "path": s.Cfg.WSPath, "builtin": true},
                {"tag": "vless-xhttp", "name": "VLESS XHTTP", "protocol": "vless", "network": "xhttp", "security": "none", "path": s.Cfg.XHTTPPath, "builtin": true},
                {"tag": "vless-hu", "name": "VLESS HTTPUpgrade", "protocol": "vless", "network": "httpupgrade", "security": "none", "path": s.Cfg.HUPath, "builtin": true},
                {"tag": "vmess-ws", "name": "VMess WS", "protocol": "vmess", "network": "ws", "security": "none", "path": s.Cfg.VmessPath, "builtin": true},
                {"tag": "trojan-ws", "name": "Trojan WS", "protocol": "trojan", "network": "ws", "security": "none", "path": s.Cfg.TrojanPath, "builtin": true},
        }
        rows, err := s.Pool.Query(r.Context(),
                `SELECT tag, name, protocol, network, security, COALESCE(port,0), COALESCE(lport,0), path, svc, sni FROM inbounds ORDER BY id`)
        if err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        defer rows.Close()
        customs := []map[string]any{}
        for rows.Next() {
                var tag, name, proto, network, security, path, svc, sni string
                var port, lport int
                if err := rows.Scan(&tag, &name, &proto, &network, &security, &port, &lport, &path, &svc, &sni); err != nil {
                        writeErr(w, 500, err.Error())
                        return
                }
                reachable := security != "reality" ||
                        // VPS / direct mode (no main Reality proxy) → own port is public
                        (s.Cfg.TCPHost == "" || s.Cfg.TCPPublicPort == "") ||
                        // mapped through the dedicated 2nd TCP Proxy
                        (s.Cfg.TCP2Host != "" && s.Cfg.TCP2Port != "" && port == s.Cfg.TCP2AppPort)
                customs = append(customs, map[string]any{
                        "tag": tag, "name": name, "protocol": proto, "network": network,
                        "security": security, "port": port, "lport": lport,
                        "path": path, "svc": svc, "sni": sni, "builtin": false,
                        "reachable": reachable,
                })
        }
        if err := rows.Err(); err != nil {
                writeErr(w, 500, err.Error())
                return
        }
        writeJSON(w, 200, map[string]any{"builtin": builtins, "custom": customs})
}
