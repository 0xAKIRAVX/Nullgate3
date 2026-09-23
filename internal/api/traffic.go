package api

import (
        "context"
        "log"
        "sync"
        "time"

        "github.com/jackc/pgx/v5/pgxpool"

        "nullgate/api/internal/config"
        "nullgate/api/internal/xray"
)

// LiveCounters holds the cached total up/down. Written from the collector
// ticker AND the PersistUsage hook (HTTP-handler goroutines), so every access
// is synchronized.
type LiveCounters struct {
        mu   sync.Mutex
        up   int64
        down int64
}

func (l *LiveCounters) Store(up, down int64) {
        l.mu.Lock()
        l.up, l.down = up, down
        l.mu.Unlock()
}

func (l *LiveCounters) Snapshot() (int64, int64) {
        l.mu.Lock()
        defer l.mu.Unlock()
        return l.up, l.down
}

// StartCollector runs the traffic loop: poll Xray's per-user counters (reset on
// read), accumulate them into Postgres, broadcast live totals, and restart Xray
// when the set of allowed users changed (limit hit, expiry, renewal) — the
// panel.py collect_usage + sync_xray + usage_loop trio, now in one goroutine.
func StartCollector(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool,
        sup *xray.Supervisor, hub *Hub, live *LiveCounters) {

        interval := time.Duration(cfg.CollectSecs) * time.Second
        if interval < 2*time.Second {
                interval = 2 * time.Second
        }
        // supervisor hook: persist counters right before any restart so the bytes
        // that die with the process are not lost (installed via the locked setter —
        // the watchdog goroutine may already be running)
        sup.SetPersistUsage(func() {
                cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
                defer cancel()
                collectOnce(cctx, pool, sup, hub, live, false)
        })

        go func() {
                t := time.NewTicker(interval)
                defer t.Stop()
                for {
                        select {
                        case <-ctx.Done():
                                return
                        case <-t.C:
                        }
                        cctx, cancel := context.WithTimeout(ctx, interval)
                        collectOnce(cctx, pool, sup, hub, live, true)
                        cancel()
                }
        }()
}

func collectOnce(ctx context.Context, pool *pgxpool.Pool, sup *xray.Supervisor,
        hub *Hub, live *LiveCounters, doSync bool) {

        if !sup.Running() {
                return
        }
        stats, err := sup.QueryStats(ctx)
        if err != nil {
                log.Printf("collector: statsquery: %v", err)
                return
        }
        for email, d := range stats {
                if d[0] == 0 && d[1] == 0 {
                        continue
                }
                if _, err := pool.Exec(ctx, `
                        INSERT INTO client_usage(client_id, up, down, updated_at)
                        VALUES($1::uuid, $2, $3, now())
                        ON CONFLICT (client_id) DO UPDATE
                          SET up = client_usage.up + $2, down = client_usage.down + $3, updated_at = now()`,
                        email, d[0], d[1]); err != nil {
                        log.Printf("collector: persist usage: %v", err)
                }
        }
        // refresh cached totals + push to the UI
        var up, down int64
        if err := pool.QueryRow(ctx,
                `SELECT COALESCE(SUM(up),0), COALESCE(SUM(down),0) FROM client_usage`).Scan(&up, &down); err == nil {
                if live != nil {
                        live.Store(up, down)
                }
                if hub != nil {
                        hub.Broadcast(map[string]any{
                                "type": "traffic", "up": up, "down": down,
                                "xray": sup.Info(),
                        })
                }
        }
        // sync: restart only when the set of allowed users changed
        if doSync {
                want, err := sup.B.LoadClients(ctx)
                if err == nil {
                        ids := make([]string, 0, len(want))
                        for _, c := range want {
                                ids = append(ids, c.ID)
                        }
                        cur := sup.ActiveSet()
                        if !equalStringSets(ids, cur) {
                                if err := sup.Restart(ctx, false); err != nil {
                                        log.Printf("collector: sync restart: %v", err)
                                }
                        }
                }
        }
}

func equalStringSets(a, b []string) bool {
        if len(a) != len(b) {
                return false
        }
        set := map[string]int{}
        for _, s := range a {
                set[s]++
        }
        for _, s := range b {
                set[s]--
                if set[s] < 0 {
                        return false
                }
        }
        return true
}
