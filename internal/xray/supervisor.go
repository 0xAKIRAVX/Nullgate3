package xray

import (
        "archive/zip"
        "bytes"
        "context"
        "crypto/sha256"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "net/http"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "strconv"
        "strings"
        "sync"
        "sync/atomic"
        "syscall"
        "time"

        "github.com/jackc/pgx/v5/pgxpool"

        "nullgate/api/internal/config"
)

var errRealityMissing = errors.New("reality keys are not ready")

func envStr(name, d string) string {
        if v := os.Getenv(name); v != "" {
                return v
        }
        return d
}

func envInt(name string, d int) int {
        if v := os.Getenv(name); v != "" {
                if n, err := strconv.Atoi(v); err == nil && n > 0 {
                        return n
                }
        }
        return d
}

// proc represents one spawned Xray child.
type proc struct {
        cmd  *exec.Cmd
        done chan struct{} // closed by the reaper when Wait returns
}

// Supervisor owns the Xray child process: renders the config from Postgres,
// spawns/restarts the binary, watches it, and exposes stats.
type Supervisor struct {
        Cfg  *config.Config
        Pool *pgxpool.Pool
        B    *Builder

        // PersistUsage is called right before a restart so traffic counters that
        // die with the process are not lost (mirror of panel.py collect_usage).
        PersistUsage func()

        mu          sync.Mutex
        cur         *proc
        running     atomic.Bool
        restarts    atomic.Int64
        since       atomic.Int64
        fingerprint string
        active      []string
        stopped     atomic.Bool

        confPath string
        logPath  string
        logMu    sync.Mutex
}

func NewSupervisor(cfg *config.Config, pool *pgxpool.Pool) *Supervisor {
        return &Supervisor{
                Cfg:      cfg,
                Pool:     pool,
                B:        &Builder{Cfg: cfg, Pool: pool},
                confPath: filepath.Join(cfg.WorkDir, "xray.json"),
                logPath:  filepath.Join(cfg.WorkDir, "xray.log"),
        }
}

// ───────────────────────── binary provisioning ─────────────────────────

func xrayAssetName() string {
        switch runtime.GOARCH {
        case "arm64":
                return "Xray-linux-arm64-v8a.zip"
        case "386":
                return "Xray-linux-32.zip"
        default:
                return "Xray-linux-64.zip"
        }
}

// EnsureBinary downloads the pinned Xray release if the binary is missing.
func (s *Supervisor) EnsureBinary(ctx context.Context) error {
        if st, err := os.Stat(s.Cfg.XrayBin); err == nil && st.Mode().IsRegular() {
                return nil
        }
        dir := filepath.Dir(s.Cfg.XrayBin)
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return err
        }
        url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/%s",
                s.Cfg.XrayVersion, xrayAssetName())
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
        if err != nil {
                return err
        }
        client := &http.Client{Timeout: 10 * time.Minute}
        resp, err := client.Do(req)
        if err != nil {
                return fmt.Errorf("download %s: %w", url, err)
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
        }
        tmp, err := os.CreateTemp(dir, "xray-*.zip")
        if err != nil {
                return err
        }
        defer os.Remove(tmp.Name())
        if _, err := io.Copy(tmp, resp.Body); err != nil {
                tmp.Close()
                return err
        }
        tmp.Close()

        zr, err := zip.OpenReader(tmp.Name())
        if err != nil {
                return err
        }
        defer zr.Close()
        for _, f := range zr.File {
                if f.Name != "xray" {
                        continue
                }
                rc, err := f.Open()
                if err != nil {
                        return err
                }
                out, err := os.OpenFile(s.Cfg.XrayBin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
                if err != nil {
                        rc.Close()
                        return err
                }
                _, err = io.Copy(out, rc)
                rc.Close()
                out.Close()
                if err != nil {
                        return err
                }
                return nil
        }
        return fmt.Errorf("xray binary not found inside %s", xrayAssetName())
}

// ───────────────────────── process control ─────────────────────────

func (s *Supervisor) buildFingerprint(res *BuiltResult) string {
        raw, _ := json.Marshal(res.Config)
        sum := sha256.Sum256(raw)
        return hex.EncodeToString(sum[:8])
}

// Restart renders a fresh config from Postgres and (re)starts Xray unless the
// rendered config is byte-identical to the running one and force is false.
func (s *Supervisor) Restart(ctx context.Context, force bool) error {
        s.mu.Lock()
        defer s.mu.Unlock()
        return s.restartLocked(ctx, force)
}

func (s *Supervisor) restartLocked(ctx context.Context, force bool) error {
        res, err := s.B.Build(ctx)
        if err != nil {
                return fmt.Errorf("build config: %w", err)
        }
        fp := s.buildFingerprint(res)
        if !force && fp == s.fingerprint && s.running.Load() {
                s.active = res.Active
                return nil
        }
        if s.PersistUsage != nil {
                func() {
                        defer func() { _ = recover() }()
                        s.PersistUsage()
                }()
        }
        raw, _ := json.MarshalIndent(res.Config, "", "  ")
        if err := os.MkdirAll(s.Cfg.WorkDir, 0o755); err != nil {
                return err
        }
        if err := os.WriteFile(s.confPath, raw, 0o600); err != nil {
                return err
        }
        s.stopLocked()
        if err := s.spawnLocked(); err != nil {
                return err
        }
        s.fingerprint = fp
        s.active = res.Active
        return nil
}

func (s *Supervisor) spawnLocked() error {
        // keep Xray's stderr for the in-panel log viewer (rotate at ~512KB)
        s.logMu.Lock()
        if st, err := os.Stat(s.logPath); err == nil && st.Size() > 512*1024 {
                _ = os.Remove(s.logPath)
        }
        lf, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        s.logMu.Unlock()
        if err != nil {
                return err
        }
        defer lf.Close()
        cmd := exec.Command(s.Cfg.XrayBin, "run", "-c", s.confPath)
        // Xray v26 writes its log to stdout (older releases used stderr) — capture both
        cmd.Stdout = lf
        cmd.Stderr = lf
        if err := cmd.Start(); err != nil {
                return err
        }
        p := &proc{cmd: cmd, done: make(chan struct{})}
        s.cur = p
        s.running.Store(true)
        s.since.Store(time.Now().Unix())
        s.restarts.Add(1)
        // reap the process when it dies so Running() flips and the watchdog restarts it
        go func() {
                _ = cmd.Wait()
                close(p.done)
                s.mu.Lock()
                if s.cur == p {
                        s.cur = nil
                        s.running.Store(false)
                }
                s.mu.Unlock()
        }()
        return nil
}

func (s *Supervisor) stopLocked() {
        if s.cur == nil {
                return
        }
        p := s.cur
        if p.cmd.Process != nil {
                _ = p.cmd.Process.Signal(syscall.SIGTERM)
                select {
                case <-p.done:
                case <-time.After(5 * time.Second):
                        _ = p.cmd.Process.Kill()
                        <-p.done
                }
        }
        // the reaper clears cur/running unless a newer process already replaced it
        if s.cur == p {
                s.cur = nil
                s.running.Store(false)
        }
}

// Stop terminates Xray (called on shutdown).
func (s *Supervisor) Stop() {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.stopped.Store(true)
        s.stopLocked()
}

// Running reports whether the child process is alive.
func (s *Supervisor) Running() bool { return s.running.Load() }

// Info is the /api/state xray payload.
func (s *Supervisor) Info() map[string]any {
        return map[string]any{
                "running":  s.running.Load(),
                "restarts": s.restarts.Load(),
                "since":    s.since.Load(),
        }
}

// ActiveSet returns the client ids present in the running config.
func (s *Supervisor) ActiveSet() []string {
        s.mu.Lock()
        defer s.mu.Unlock()
        return append([]string(nil), s.active...)
}

// Watchdog restarts Xray whenever it died (mirror of panel.py watchdog).
func (s *Supervisor) Watchdog(ctx context.Context) {
        t := time.NewTicker(4 * time.Second)
        defer t.Stop()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-t.C:
                }
                dead := !s.running.Load() && !s.stopped.Load()
                if !dead {
                        continue
                }
                cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
                if err := s.Restart(cctx, false); err != nil {
                        _ = err // logged on next tick
                }
                cancel()
        }
}

// TailLog returns the last n lines of Xray's stderr, newest last.
func (s *Supervisor) TailLog(n int) []string {
        if n <= 0 || n > 2000 {
                n = 120
        }
        s.logMu.Lock()
        defer s.logMu.Unlock()
        data, err := os.ReadFile(s.logPath)
        if err != nil {
                return []string{}
        }
        lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
        if len(lines) > n {
                lines = lines[len(lines)-n:]
        }
        return lines
}

// ───────────────────────── traffic stats ─────────────────────────

// QueryStats shells out to `xray api statsquery` (same mechanism as panel.py);
// counters reset on read. Returns email → {up, down} delta.
func (s *Supervisor) QueryStats(ctx context.Context) (map[string][2]int64, error) {
        cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()
        cmd := exec.CommandContext(cctx, s.Cfg.XrayBin, "api", "statsquery",
                "--server=127.0.0.1:"+strconv.Itoa(s.Cfg.APIPort), "-pattern", "user>>>", "-reset")
        var stdout, stderr bytes.Buffer
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr
        if err := cmd.Run(); err != nil {
                return nil, fmt.Errorf("statsquery: %v: %s", err, strings.TrimSpace(stderr.String()))
        }
        var data struct {
                Stat []struct {
                        Name  string `json:"name"`
                        Value any    `json:"value"`
                } `json:"stat"`
        }
        if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
                return nil, err
        }
        res := map[string][2]int64{}
        for _, st := range data.Stat {
                p := strings.Split(st.Name, ">>>")
                if len(p) != 4 || p[0] != "user" {
                        continue
                }
                v, ok := toInt64(st.Value)
                if !ok {
                        continue
                }
                cur := res[p[1]]
                if p[3] == "uplink" {
                        cur[0] += v
                } else {
                        cur[1] += v
                }
                res[p[1]] = cur
        }
        return res, nil
}

func toInt64(v any) (int64, bool) {
        switch n := v.(type) {
        case float64:
                return int64(n), true
        case string:
                if i, err := strconv.ParseInt(n, 10, 64); err == nil {
                        return i, true
                }
        case int64:
                return n, true
        case int:
                return int64(n), true
        case json.Number:
                if i, err := n.Int64(); err == nil {
                        return i, true
                }
        }
        return 0, false
}
