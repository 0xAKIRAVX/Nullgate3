package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GetJSON returns the raw jsonb value stored under key (nil if absent).
func GetJSON(ctx context.Context, pool *pgxpool.Pool, key string) ([]byte, error) {
	var val []byte
	err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&val)
	if err != nil {
		return nil, err // pgx.ErrNoRows when absent
	}
	return val, nil
}

// PutJSON upserts a jsonb value under key.
func PutJSON(ctx context.Context, pool *pgxpool.Pool, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO settings(key, value, updated_at) VALUES($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, raw)
	return err
}

// DefaultPanelSettings is seeded on first boot and merged on read.
func DefaultPanelSettings(sni string, sessionHours int) map[string]any {
	return map[string]any{
		"protocols": map[string]bool{
			"vless-reality": true,
			"vless-ws":      true,
			"vless-xhttp":   true,
			"vless-hu":      true,
			"vmess-ws":      true,
			"trojan-ws":     true,
		},
		"reality_sni":   sni,
		"session_hours": sessionHours,
		"cfg_fmt":       "{name}-{label}",
		"address":       "",
	}
}

// EnsurePanelSettings seeds the 'panel' settings row if absent.
func EnsurePanelSettings(ctx context.Context, pool *pgxpool.Pool, sni string, sessionHours int) error {
	_, err := GetJSON(ctx, pool, "panel")
	if err == nil {
		return nil
	}
	return PutJSON(ctx, pool, "panel", DefaultPanelSettings(sni, sessionHours))
}

// EnsureReality generates Reality keys on first boot and stores them in
// Postgres, so they never change across redeploys.
func EnsureReality(ctx context.Context, pool *pgxpool.Pool, gen func() (map[string]any, error)) error {
	_, err := GetJSON(ctx, pool, "reality")
	if err == nil {
		return nil
	}
	keys, err := gen()
	if err != nil {
		return err
	}
	return PutJSON(ctx, pool, "reality", keys)
}

// PanelSettings is the typed view of the 'panel' settings row.
type PanelSettings struct {
	Protocols    map[string]bool `json:"protocols"`
	RealitySNI   string          `json:"reality_sni"`
	SessionHours int             `json:"session_hours"`
	CfgFmt       string          `json:"cfg_fmt"`
	Address      string          `json:"address"`
}

func LoadPanelSettings(ctx context.Context, pool *pgxpool.Pool, sni string, sessionHours int) (map[string]any, error) {
	raw, err := GetJSON(ctx, pool, "panel")
	if err != nil {
		return DefaultPanelSettings(sni, sessionHours), nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return DefaultPanelSettings(sni, sessionHours), nil
	}
	return m, nil
}

func nowPtr() *time.Time { t := time.Now(); return &t }

var _ = nowPtr
