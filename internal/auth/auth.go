package auth

import (
        "context"
        "crypto/rand"
        "encoding/hex"
        "errors"
        "log"
        "sync"
        "sync/atomic"
        "time"

        "github.com/redis/go-redis/v9"
        "golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

type memSess struct {
        admin int64
        exp   time.Time
}

// Manager stores admin sessions in Redis when available and always keeps an
// in-memory mirror, so the panel keeps working if Redis blips.
type Manager struct {
        rdb *redis.Client
        ttl time.Duration

        mu   sync.Mutex
        mem  map[string]memSess
        gets atomic.Uint32
}

func NewManager(rdb *redis.Client, sessionHours int) *Manager {
        return &Manager{
                rdb: rdb,
                ttl: time.Duration(sessionHours) * time.Hour,
                mem: map[string]memSess{},
        }
}

func randomToken() (string, error) {
        b := make([]byte, 32)
        if _, err := rand.Read(b); err != nil {
                return "", err
        }
        return hex.EncodeToString(b), nil
}

func RandomHex(n int) string {
        b := make([]byte, n)
        _, _ = rand.Read(b)
        return hex.EncodeToString(b)
}

func (m *Manager) TTL() time.Duration { return m.ttl }

func (m *Manager) Create(ctx context.Context, adminID int64) (string, error) {
        tok, err := randomToken()
        if err != nil {
                return "", err
        }
        if m.rdb != nil {
                if err := m.rdb.Set(ctx, "sess:"+tok, adminID, m.ttl).Err(); err != nil {
                        // a Redis blip must not lock the admin out — the memory mirror keeps
                        // the panel working (single instance loses nothing)
                        log.Printf("auth: redis set failed (%v) — memory session only", err)
                }
        }
        m.mu.Lock()
        m.mem[tok] = memSess{admin: adminID, exp: time.Now().Add(m.ttl)}
        m.mu.Unlock()
        return tok, nil
}

func (m *Manager) Get(ctx context.Context, token string) (int64, bool) {
        if token == "" {
                return 0, false
        }
        // occasionally sweep expired entries so abandoned sessions don't leak
        if m.gets.Add(1)%512 == 0 {
                m.sweepExpired()
        }
        if m.rdb != nil {
                if v, err := m.rdb.Get(ctx, "sess:"+token).Int64(); err == nil {
                        return v, true
                }
        }
        m.mu.Lock()
        defer m.mu.Unlock()
        s, ok := m.mem[token]
        if !ok {
                return 0, false
        }
        if time.Now().After(s.exp) {
                delete(m.mem, token) // evict instead of keeping it forever
                return 0, false
        }
        return s.admin, true
}

func (m *Manager) sweepExpired() {
        m.mu.Lock()
        defer m.mu.Unlock()
        now := time.Now()
        for k, v := range m.mem {
                if now.After(v.exp) {
                        delete(m.mem, k)
                }
        }
}

// RedisAlive reports whether the Redis session backend is configured and
// reachable (used by /api/health).
func (m *Manager) RedisAlive(ctx context.Context) bool {
        if m == nil || m.rdb == nil {
                return false
        }
        return m.rdb.Ping(ctx).Err() == nil
}

func (m *Manager) Delete(ctx context.Context, token string) {
        if m.rdb != nil {
                m.rdb.Del(ctx, "sess:"+token)
        }
        m.mu.Lock()
        delete(m.mem, token)
        m.mu.Unlock()
}

func HashPassword(pw string) (string, error) {
        b, err := bcrypt.GenerateFromPassword([]byte(pw), 10)
        return string(b), err
}

func CheckPassword(hash, pw string) bool {
        return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
