package config

import (
        "os"
        "strconv"
)

// Config holds all runtime configuration, sourced from environment variables
// (Railway-injected) with sane local defaults.
type Config struct {
        HTTPPort     string // public API port (Railway PORT)
        DatabaseURL  string // Postgres connection string (required)
        RedisURL     string // optional; sessions/ratelimit fall back to memory
        Secret       string // optional HMAC secret (sub-token signing)
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

func Load() *Config {
        return &Config{
                HTTPPort:     get("PORT", "8080"),
                DatabaseURL:  get("DATABASE_URL", ""),
                RedisURL:     get("REDIS_URL", ""),
                Secret:       get("SECRET", ""),
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
