//go:build e2e

package e2e

import (
        "bufio"
        "io"
        "os"
        "os/exec"
)

func osWriteFile(path string, data []byte) error {
        f, err := os.Create(path)
        if err != nil {
                return err
        }
        defer f.Close()
        _, err = f.Write(data)
        return err
}

// execProc wraps a child process with its stderr tail (for failure reports).
type execProc struct {
        Process *os.Process
        tail    *ringBuf
        waitCh  chan struct{}
}

func startProc(name string, args ...string) (*execProc, error) {
        cmd := exec.Command(name, args...)
        stdout, err := cmd.StdoutPipe()
        if err != nil {
                return nil, err
        }
        stderr, err := cmd.StderrPipe()
        if err != nil {
                return nil, err
        }
        if err := cmd.Start(); err != nil {
                return nil, err
        }
        p := &execProc{Process: cmd.Process, tail: newRing(200), waitCh: make(chan struct{})}
        scan := func(rd io.Reader) {
                sc := bufio.NewScanner(rd)
                sc.Buffer(make([]byte, 64*1024), 1024*1024)
                for sc.Scan() {
                        p.tail.add(sc.Text())
                }
        }
        go func() {
                defer close(p.waitCh)
                done := make(chan struct{}, 2)
                go func() { scan(stdout); done <- struct{}{} }()
                go func() { scan(stderr); done <- struct{}{} }()
                <-done
                <-done
        }()
        return p, nil
}

// ringBuf keeps the last n lines of a stream for error messages.
type ringBuf struct {
        mu    chan struct{}
        lines []string
        n     int
}

func newRing(n int) *ringBuf { return &ringBuf{mu: make(chan struct{}, 1), n: n} }

func (r *ringBuf) add(line string) {
        r.mu <- struct{}{}
        r.lines = append(r.lines, line)
        if len(r.lines) > r.n {
                r.lines = r.lines[len(r.lines)-r.n:]
        }
        <-r.mu
}

func (r *ringBuf) snapshot() []string {
        r.mu <- struct{}{}
        defer func() { <-r.mu }()
        out := make([]string, len(r.lines))
        copy(out, r.lines)
        return out
}
