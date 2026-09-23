package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
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

	mu  sync.Mutex
	mem map[string]memSess
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
			return "", err
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
	if m.rdb != nil {
		if v, err := m.rdb.Get(ctx, "sess:"+token).Int64(); err == nil {
			return v, true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.mem[token]
	if !ok || time.Now().After(s.exp) {
		return 0, false
	}
	return s.admin, true
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
