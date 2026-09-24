package config

import (
        "net"
        "net/url"
        "os"
        "strconv"
)

// Config holds all runtime configuration, sourced from environment variables
// (Railway-injected) with sane local defaults. Deploying on Railway needs ZERO
// manually-typed variables: DATABASE_URL arrives by connecting the postgres
// service in the UI, PORT and RAILWAY_TCP_PROXY_* / RAILWAY_TCP_APPLICATION_PORT
// are injected by the platform, everything else has a default.
type Config struct {
        HTTPPort     string // public API port (Railway PORT)
        DatabaseURL  string // Postgres connection string (required)
        RedisURL     string // optional; sessions/ratelimit fall back to memory
        RealitySNI   string // default Reality serverName
        SessionHours int    // admin session lifetime

        AppPort    int // Reality inbound port inside container (TCP Proxy target)
        APIPort    int // Xray stats API (dokodemo) port
        RealityNet string // builtin Reality transport: tcp | grpc (REALITY_NET, default tcp)
        RealitySvc string // gRPC serviceName when RealityNet=grpc (REALITY_GRPC_SERVICE)
        WSPath     string
        XHTTPPath  string
        HUPath     string
        VmessPath  string
        TrojanPath string

        TCPHost       string // Reality TCP Proxy public domain (RAILWAY_TCP_PROXY_DOMAIN)
        TCPPublicPort string // Reality TCP Proxy public port  (RAILWAY_TCP_PROXY_PORT)

        TCP2Host    string // 2nd Reality TCP Proxy public domain (TCP2_HOST)
        TCP2Port    string // 2nd Reality TCP Proxy public port   (TCP2_PORT)
        TCP2AppPort int    // internal port the 2nd proxy maps to (TCP2_APP_PORT, default 9001)

        XrayBin     string
        XrayVersion string
        WorkDir     string // writable dir for xray.json / xray.log
        CollectSecs int    // traffic collection interval

        CORSOrigin      string // "*" or exact web origin
        InsecureCookies bool   // dev mode over plain http
}

func get(k, d string) string {
        if v := os.Getenv(k); v != "" {
                return v
        }
        return d
}

func geti(k string, d int) int {
        if v := os.Getenv(k); v != "" {
                if n, err := strconv.Atoi(v); err == nil && n > 0 {
                        return n
                }
        }
        return d
}

// databaseURL resolves the Postgres DSN. Priority: DATABASE_URL, then a URL
// assembled from libpq-style PG* variables (some platforms/providers inject
// only those). This keeps deployments that expose PG* working with zero setup.
func databaseURL() string {
        if v := os.Getenv("DATABASE_URL"); v != "" {
                return v
        }
        host := os.Getenv("PGHOST")
        if host == "" {
                return ""
        }
        user := os.Getenv("PGUSER")
        pass := os.Getenv("PGPASSWORD")
        db := os.Getenv("PGDATABASE")
        if db == "" {
                db = user
        }
        port := os.Getenv("PGPORT")
        if port == "" {
                port = "5432"
        }
        u := url.URL{
                Scheme:   "postgres",
                Host:     net.JoinHostPort(host, port),
                Path:     "/" + db,
                User:     url.UserPassword(user, pass),
                RawQuery: "sslmode=" + defaultStr(os.Getenv("PGSSLMODE"), "disable"),
        }
        return u.String()
}

func defaultStr(v, d string) string {
        if v != "" {
                return v
        }
        return d
}

func Load() *Config {
        return &Config{
                HTTPPort:     get("PORT", "8080"),
                DatabaseURL:  databaseURL(),
                RedisURL:     get("REDIS_URL", ""),
                RealitySNI:   get("REALITY_SNI", "www.samsung.com"),
                SessionHours: geti("SESSION_HOURS", 24),

                AppPort:    geti("RAILWAY_TCP_APPLICATION_PORT", geti("TCP_APP_PORT", 9000)),
                APIPort:    geti("XRAY_API_PORT", 10085),
                RealityNet: get("REALITY_NET", "tcp"),
                RealitySvc: get("REALITY_GRPC_SERVICE", "nullgate"),
                WSPath:     get("WS_PATH", "/ws"),
                XHTTPPath:  get("XHTTP_PATH", "/xhttp"),
                HUPath:     get("HU_PATH", "/hu"),
                VmessPath:  get("VMESS_PATH", "/vmess"),
                TrojanPath: get("TROJAN_PATH", "/trojan"),

                TCPHost:       get("TCP_HOST", get("RAILWAY_TCP_PROXY_DOMAIN", "")),
                TCPPublicPort: get("TCP_PORT", get("RAILWAY_TCP_PROXY_PORT", "")),

                TCP2Host:    get("TCP2_HOST", get("RAILWAY_TCP2_PROXY_DOMAIN", "")),
                TCP2Port:    get("TCP2_PORT", get("RAILWAY_TCP2_PROXY_PORT", "")),
                TCP2AppPort: geti("TCP2_APP_PORT", 9001),

                XrayBin:     get("XRAY_BIN", "./xray/xray"),
                XrayVersion: get("XRAY_VERSION", "v26.3.27"),
                WorkDir:     get("NG_WORKDIR", "/tmp/nullgate"),
                CollectSecs: geti("COLLECT_INTERVAL", 30),

                CORSOrigin:      get("CORS_ORIGIN", "*"),
                InsecureCookies: get("COOKIE_INSECURE", "") == "1",
        }
}
