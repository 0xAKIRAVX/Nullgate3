package main

import (
        "context"
        "log"
        "net/http"
        "os"
        "os/signal"
        "syscall"
        "time"

        "github.com/redis/go-redis/v9"
        "golang.org/x/net/http2"
        "golang.org/x/net/http2/h2c"

        "nullgate/api/internal/api"
        "nullgate/api/internal/auth"
        "nullgate/api/internal/config"
        "nullgate/api/internal/db"
        "nullgate/api/internal/store"
        "nullgate/api/internal/xray"
)

func main() {
        cfg := config.Load()
        ctx, stop := context.WithCancel(context.Background())
        defer stop()

        if cfg.DatabaseURL == "" {
                log.Fatal("DATABASE_URL is not set.\n" +
                        "  Railway (no typing needed): open the postgres service → Connect → pick this api service;\n" +
                        "  DATABASE_URL (and PG*) are injected automatically. Nothing else to configure.\n" +
                        "  Local: set DATABASE_URL, or PGHOST/PGUSER/PGPASSWORD/PGDATABASE/PGPORT (auto-assembled).")
        }

        pool, err := db.Connect(ctx, cfg.DatabaseURL)
        if err != nil {
                log.Fatalf("postgres: %v", err)
        }
        defer pool.Close()
        log.Print("postgres: connected")

        if err := db.Migrate(ctx, pool); err != nil {
                log.Fatalf("migrate: %v", err)
        }
        log.Print("postgres: migrations applied")

        var rdb *redis.Client
        if cfg.RedisURL != "" {
                opt, err := redis.ParseURL(cfg.RedisURL)
                if err == nil {
                        rdb = redis.NewClient(opt)
                        pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
                        if err := rdb.Ping(pctx).Err(); err != nil {
                                log.Printf("redis: unreachable (%v) — falling back to memory sessions", err)
                                rdb = nil
                        } else {
                                log.Print("redis: connected")
                        }
                        cancel()
                } else {
                        log.Printf("redis: bad REDIS_URL (%v) — using memory sessions", err)
                }
        } else {
                log.Print("redis: REDIS_URL not set — using memory sessions")
        }

        if err := store.EnsurePanelSettings(ctx, pool, cfg.RealitySNI, cfg.SessionHours); err != nil {
                log.Fatalf("settings seed: %v", err)
        }
        if err := store.EnsureReality(ctx, pool, func() (map[string]any, error) {
                keys, err := xray.GenerateReality()
                if err != nil {
                        return nil, err
                }
                log.Printf("reality: generated new keypair (pub=%s sid=%s)", keys.Pub, keys.SID)
                return map[string]any{"priv": keys.Priv, "pub": keys.Pub, "sid": keys.SID}, nil
        }); err != nil {
                log.Fatalf("reality seed: %v", err)
        }

        // ── Xray engine: binary, supervisor, traffic collector, ws hub ──────────
        sup := xray.NewSupervisor(cfg, pool)
        bctx, bcancel := context.WithTimeout(ctx, 10*time.Minute)
        if err := sup.EnsureBinary(bctx); err != nil {
                log.Printf("xray binary: %v (the panel API still runs; Xray will retry on watchdog)", err)
        }
        bcancel()
        if err := sup.Restart(ctx, true); err != nil {
                log.Printf("xray start: %v (watchdog keeps retrying)", err)
        } else {
                log.Printf("xray: started %v", sup.Info())
        }
        go sup.Watchdog(ctx)

        hub := api.NewHub(cfg.CORSOrigin)
        live := &api.LiveCounters{}
        api.StartCollector(ctx, cfg, pool, sup, hub, live)

        am := auth.NewManager(rdb, cfg.SessionHours)
        srv := api.NewServer(cfg, pool, am, sup, hub)
        srv.SyncCustomRoutes()

        // h2c: serve cleartext HTTP/2 alongside HTTP/1.1 — gRPC clients behind the
        // platform edge reach the gRPC inbounds end-to-end over HTTP/2
        handler := h2c.NewHandler(srv.Handler(), &http2.Server{})

        httpServer := &http.Server{
                Addr:              ":" + cfg.HTTPPort,
                Handler:           handler,
                ReadHeaderTimeout: 10 * time.Second,
                // no WriteTimeout: long-lived websocket/xhttp tunnels must not be cut
        }

        go func() {
                log.Printf("NullGate %s listening on :%s", api.Version, cfg.HTTPPort)
                if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                        log.Fatalf("http: %v", err)
                }
        }()

        sig := make(chan os.Signal, 1)
        signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
        <-sig
        log.Print("shutting down…")
        sup.Stop() // persist handled by PersistUsage hook inside Restart; kill child now
        sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        _ = httpServer.Shutdown(sctx)
}
