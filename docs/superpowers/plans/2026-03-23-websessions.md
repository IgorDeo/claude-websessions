# websessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a web-based command center for managing multiple Claude Code CLI sessions with full interactive terminal access, notifications, and split-pane views.

**Architecture:** Single Go binary serving an htmx+Templ UI over HTTP. A Session Manager allocates PTYs and spawns `claude` processes, streaming output to browsers via WebSocket. A Discovery module scans for existing `claude` processes and enables takeover via kill + `--resume`. SQLite stores session history and notifications.

**Tech Stack:** Go 1.22+, chi router, Templ templates, gorilla/websocket, creack/pty, modernc.org/sqlite, htmx, xterm.js, split.js

**Spec:** `docs/superpowers/specs/2026-03-23-websessions-design.md`

---

## File Structure

```
websessions/
├── cmd/websessions/main.go                  # Entry point, wiring, signal handling
├── internal/
│   ├── config/
│   │   ├── config.go                        # Config struct + Load() from YAML
│   │   └── config_test.go
│   ├── store/
│   │   ├── store.go                         # SQLite schema, migrations, repository
│   │   └── store_test.go
│   ├── session/
│   │   ├── session.go                       # Session struct, state machine, ring buffer
│   │   ├── manager.go                       # SessionManager: create, list, kill, PTY lifecycle
│   │   ├── manager_test.go
│   │   └── ringbuf.go                       # Ring buffer implementation
│   │   └── ringbuf_test.go
│   ├── discovery/
│   │   ├── scanner.go                       # Process scanner (Linux /proc, macOS ps)
│   │   ├── scanner_test.go
│   │   ├── takeover.go                      # Kill + resume logic
│   │   └── takeover_test.go
│   ├── notification/
│   │   ├── bus.go                           # Event bus + SessionEvent types
│   │   ├── bus_test.go
│   │   ├── sink.go                          # NotificationSink interface + InAppSink
│   │   └── sink_test.go
│   └── server/
│       ├── server.go                        # HTTP server setup, routes, middleware
│       ├── server_test.go
│       ├── ws.go                            # WebSocket handler (terminal streaming)
│       ├── ws_test.go
│       ├── handlers.go                      # htmx handlers (sessions, tabs, notifications)
│       └── auth.go                          # Token auth middleware
├── web/
│   ├── embed.go                             # go:embed directives for static + templates
│   ├── static/
│   │   ├── htmx.min.js                      # Vendored htmx
│   │   ├── xterm.js                         # Vendored xterm.js
│   │   ├── xterm.css                        # Vendored xterm.js styles
│   │   ├── xterm-addon-fit.js               # Vendored xterm fit addon
│   │   ├── split.min.js                     # Vendored split.js
│   │   ├── app.js                           # Custom JS: WS connect, xterm init, split panes, notifications
│   │   └── style.css                        # App styles
│   └── templates/
│       ├── layout.templ                     # Base HTML layout (head, scripts, body shell)
│       ├── index.templ                      # Main page: sidebar + content area
│       ├── sidebar.templ                    # Session list partial
│       ├── terminal.templ                   # Terminal pane partial
│       ├── tabs.templ                       # Tab bar partial
│       ├── notifications.templ              # Notification dropdown partial
│       └── newsession.templ                 # New session modal/form
├── config.example.yaml
├── Makefile
├── Dockerfile
├── go.mod
└── go.sum
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`, `Makefile`, `cmd/websessions/main.go`, `config.example.yaml`

- [ ] **Step 1: Initialize Go module**

```bash
cd /home/igor.deoalves/Documentos/pessoal/websessions
go mod init github.com/igor-deoalves/websessions
```

- [ ] **Step 2: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: build run test clean generate

BINARY=websessions
BUILD_DIR=bin

build: generate
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/websessions

run: generate
	go run ./cmd/websessions

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR)

generate:
	templ generate

lint:
	golangci-lint run ./...
```

- [ ] **Step 3: Create minimal main.go**

Create `cmd/websessions/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("websessions starting...")
	os.Exit(0)
}
```

- [ ] **Step 4: Create config.example.yaml**

Create `config.example.yaml`:

```yaml
server:
  port: 8080
  host: 0.0.0.0
sessions:
  scan_interval: 30s
  output_buffer_size: 10MB
  default_dir: ~/projects
notifications:
  desktop: true
  events: [completed, errored, waiting]
auth:
  enabled: false
  token: ""
```

- [ ] **Step 5: Verify build**

```bash
go build ./cmd/websessions
```

Expected: builds with no errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod Makefile cmd/ config.example.yaml
git commit -m "feat: project scaffolding with go module, makefile, and entry point"
```

---

### Task 2: Config Loading

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write config tests**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Sessions.ScanInterval != 30*time.Second {
		t.Errorf("expected scan interval 30s, got %v", cfg.Sessions.ScanInterval)
	}
	if cfg.Sessions.OutputBufferSize != 10*1024*1024 {
		t.Errorf("expected buffer 10MB, got %d", cfg.Sessions.OutputBufferSize)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`
server:
  port: 9090
  host: 127.0.0.1
sessions:
  scan_interval: 10s
  output_buffer_size: 5MB
  default_dir: /tmp/test
notifications:
  desktop: false
  events: [completed]
auth:
  enabled: true
  token: "secret123"
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Auth.Token != "secret123" {
		t.Errorf("expected token secret123, got %s", cfg.Auth.Token)
	}
	if cfg.Sessions.OutputBufferSize != 5*1024*1024 {
		t.Errorf("expected buffer 5MB, got %d", cfg.Sessions.OutputBufferSize)
	}
}

func TestLoadFromEnvOverride(t *testing.T) {
	t.Setenv("WEBSESSIONS_AUTH_TOKEN", "envtoken")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Auth.Token != "envtoken" {
		t.Errorf("expected token envtoken, got %s", cfg.Auth.Token)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -v
```

Expected: compilation errors (package doesn't exist yet).

- [ ] **Step 3: Implement config package**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Sessions      SessionsConfig      `yaml:"sessions"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Auth          AuthConfig          `yaml:"auth"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

type SessionsConfig struct {
	ScanInterval     time.Duration `yaml:"-"`
	ScanIntervalRaw  string        `yaml:"scan_interval"`
	OutputBufferSize int64         `yaml:"-"`
	OutputBufferRaw  string        `yaml:"output_buffer_size"`
	DefaultDir       string        `yaml:"default_dir"`
}

type NotificationsConfig struct {
	Desktop bool     `yaml:"desktop"`
	Events  []string `yaml:"events"`
}

type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Sessions: SessionsConfig{
			ScanInterval:     30 * time.Second,
			ScanIntervalRaw:  "30s",
			OutputBufferSize: 10 * 1024 * 1024,
			OutputBufferRaw:  "10MB",
			DefaultDir:       "~/projects",
		},
		Notifications: NotificationsConfig{
			Desktop: true,
			Events:  []string{"completed", "errored", "waiting"},
		},
		Auth: AuthConfig{
			Enabled: false,
			Token:   "",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
		if err := cfg.parseRawFields(); err != nil {
			return nil, err
		}
	}

	cfg.applyEnvOverrides()
	return cfg, nil
}

func (c *Config) parseRawFields() error {
	if c.Sessions.ScanIntervalRaw != "" {
		d, err := time.ParseDuration(c.Sessions.ScanIntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing scan_interval: %w", err)
		}
		c.Sessions.ScanInterval = d
	}
	if c.Sessions.OutputBufferRaw != "" {
		size, err := parseByteSize(c.Sessions.OutputBufferRaw)
		if err != nil {
			return fmt.Errorf("parsing output_buffer_size: %w", err)
		}
		c.Sessions.OutputBufferSize = size
	}
	return nil
}

func (c *Config) applyEnvOverrides() {
	if token := os.Getenv("WEBSESSIONS_AUTH_TOKEN"); token != "" {
		c.Auth.Token = token
	}
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}

	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return n * multiplier, nil
}
```

- [ ] **Step 4: Add yaml.v3 dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/config/... -v
```

Expected: all 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: config loading with YAML parsing, defaults, and env overrides"
```

---

### Task 3: Ring Buffer

**Files:**
- Create: `internal/session/ringbuf.go`, `internal/session/ringbuf_test.go`

- [ ] **Step 1: Write ring buffer tests**

Create `internal/session/ringbuf_test.go`:

```go
package session_test

import (
	"testing"

	"github.com/igor-deoalves/websessions/internal/session"
)

func TestRingBuf_WriteAndRead(t *testing.T) {
	rb := session.NewRingBuf(1024)
	data := []byte("hello world")
	n, err := rb.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Errorf("wrote %d, expected %d", n, len(data))
	}
	got := rb.Bytes()
	if string(got) != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestRingBuf_Overflow(t *testing.T) {
	rb := session.NewRingBuf(10)
	rb.Write([]byte("abcdefghij")) // fills buffer
	rb.Write([]byte("XYZ"))        // overwrites first 3 bytes
	got := rb.Bytes()
	if string(got) != "defghijXYZ" {
		t.Errorf("got %q, want %q", got, "defghijXYZ")
	}
}

func TestRingBuf_Empty(t *testing.T) {
	rb := session.NewRingBuf(1024)
	got := rb.Bytes()
	if len(got) != 0 {
		t.Errorf("expected empty, got %d bytes", len(got))
	}
}

func TestRingBuf_ExactFill(t *testing.T) {
	rb := session.NewRingBuf(5)
	rb.Write([]byte("abcde"))
	got := rb.Bytes()
	if string(got) != "abcde" {
		t.Errorf("got %q, want %q", got, "abcde")
	}
}

func TestRingBuf_MultipleSmallWrites(t *testing.T) {
	rb := session.NewRingBuf(10)
	rb.Write([]byte("abc"))
	rb.Write([]byte("def"))
	rb.Write([]byte("ghij"))
	// buffer is full: "abcdefghij"
	rb.Write([]byte("kl"))
	// overwrites: "cdefghijkl"
	got := rb.Bytes()
	if string(got) != "cdefghijkl" {
		t.Errorf("got %q, want %q", got, "cdefghijkl")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement ring buffer**

Create `internal/session/ringbuf.go`:

```go
package session

import "sync"

// RingBuf is a thread-safe circular byte buffer.
type RingBuf struct {
	mu      sync.Mutex
	buf     []byte
	size    int
	w       int   // next write position
	written int64 // total bytes written (used to determine if buffer is full)
}

func NewRingBuf(size int) *RingBuf {
	return &RingBuf{
		buf:  make([]byte, size),
		size: size,
	}
}

func (r *RingBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(p)

	// If data is larger than buffer, only keep the tail
	if n >= r.size {
		copy(r.buf, p[n-r.size:])
		r.w = 0
		r.written += int64(n)
		return n, nil
	}

	// Write data, wrapping around if needed
	firstChunk := r.size - r.w
	if firstChunk >= n {
		copy(r.buf[r.w:], p)
	} else {
		copy(r.buf[r.w:], p[:firstChunk])
		copy(r.buf, p[firstChunk:])
	}

	r.w = (r.w + n) % r.size
	r.written += int64(n)

	return n, nil
}

func (r *RingBuf) isFull() bool {
	return r.written >= int64(r.size)
}

// Bytes returns the current buffer contents in order.
func (r *RingBuf) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isFull() {
		out := make([]byte, r.w)
		copy(out, r.buf[:r.w])
		return out
	}

	out := make([]byte, r.size)
	// Read from write position (oldest) to end, then start to write position
	n := copy(out, r.buf[r.w:])
	copy(out[n:], r.buf[:r.w])
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/session/... -v
```

Expected: all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat: ring buffer for session output history"
```

---

### Task 4: Session Types and State Machine

**Files:**
- Create: `internal/session/session.go`

- [ ] **Step 1: Define session types**

Create `internal/session/session.go`:

```go
package session

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/creack/pty"
)

type State string

const (
	StateDiscovered State = "discovered"
	StateTakeover   State = "takeover"
	StateCreated    State = "created"
	StateRunning    State = "running"
	StateWaiting    State = "waiting"
	StateCompleted  State = "completed"
	StateErrored    State = "errored"
)

type Session struct {
	mu sync.RWMutex

	ID        string
	ClaudeID  string // Claude Code session ID for --resume
	Name      string
	WorkDir   string
	State     State
	PID       int
	StartTime time.Time
	EndTime   time.Time
	ExitCode  int
	Error     string
	Owned     bool // true if launched/taken over by websessions

	pty    *os.File // PTY master
	proc   *os.Process
	output *RingBuf
}

var validTransitions = map[State][]State{
	StateDiscovered: {StateTakeover},
	StateTakeover:   {StateRunning, StateErrored},
	StateCreated:    {StateRunning},
	StateRunning:    {StateWaiting, StateCompleted, StateErrored},
	StateWaiting:    {StateRunning, StateCompleted, StateErrored},
}

func (s *Session) Transition(to State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	allowed, ok := validTransitions[s.State]
	if !ok {
		return fmt.Errorf("no transitions from state %s", s.State)
	}
	for _, valid := range allowed {
		if valid == to {
			s.State = to
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s -> %s", s.State, to)
}

func (s *Session) GetState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *Session) PTY() *os.File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pty
}

func (s *Session) Output() *RingBuf {
	return s.output
}

// SetPTY sets the PTY master file and process. Used by the manager after spawning.
func (s *Session) SetPTY(ptmx *os.File, proc *os.Process) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pty = ptmx
	s.proc = proc
	if proc != nil {
		s.PID = proc.Pid
	}
}

// Resize sends a window size change to the PTY.
func (s *Session) Resize(rows, cols uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pty == nil {
		return fmt.Errorf("no PTY attached")
	}
	return pty.Setsize(s.pty, &pty.Winsize{Rows: rows, Cols: cols})
}
```

- [ ] **Step 2: Add creack/pty dependency**

```bash
go get github.com/creack/pty
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/session/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/session/session.go go.mod go.sum
git commit -m "feat: session types, state machine, and PTY management"
```

---

### Task 5: Session Manager

**Files:**
- Create: `internal/session/manager.go`, `internal/session/manager_test.go`

- [ ] **Step 1: Write session manager tests**

Create `internal/session/manager_test.go`:

```go
package session_test

import (
	"testing"

	"github.com/igor-deoalves/websessions/internal/session"
)

func TestManager_CreateSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024) // 10MB buffer

	s, err := mgr.Create("test-session", "/tmp", "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.ID != "test-session" {
		t.Errorf("expected ID test-session, got %s", s.ID)
	}
	if s.GetState() != session.StateRunning {
		t.Errorf("expected state running, got %s", s.GetState())
	}
	if s.PID == 0 {
		t.Error("expected non-zero PID")
	}

	// Wait for short-lived process to finish
	mgr.Wait(s.ID)

	if s.GetState() != session.StateCompleted {
		t.Errorf("expected state completed, got %s", s.GetState())
	}
}

func TestManager_ListSessions(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)

	mgr.Create("s1", "/tmp", "echo", []string{"1"})
	mgr.Create("s2", "/tmp", "echo", []string{"2"})

	sessions := mgr.List()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManager_GetSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)

	mgr.Create("test", "/tmp", "echo", []string{"hi"})

	s, ok := mgr.Get("test")
	if !ok {
		t.Fatal("expected to find session")
	}
	if s.ID != "test" {
		t.Errorf("expected ID test, got %s", s.ID)
	}

	_, ok = mgr.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent session")
	}
}

func TestManager_KillSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)

	// Use sleep so it doesn't exit immediately
	s, err := mgr.Create("sleepy", "/tmp", "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Kill(s.ID)
	if err != nil {
		t.Fatalf("kill error: %v", err)
	}

	mgr.Wait(s.ID)

	if s.GetState() != session.StateErrored {
		t.Errorf("expected state errored after kill, got %s", s.GetState())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/... -v -run TestManager
```

Expected: compilation errors.

- [ ] **Step 3: Implement session manager**

Create `internal/session/manager.go`:

```go
package session

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

type StateChangeFunc func(s *Session, from, to State)

type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	bufferSize     int64
	onStateChange  StateChangeFunc
	onOutput       OutputFunc
}

func NewManager(bufferSize int64) *Manager {
	return &Manager{
		sessions:   make(map[string]*Session),
		bufferSize: bufferSize,
	}
}

func (m *Manager) OnStateChange(fn StateChangeFunc) {
	m.onStateChange = fn
}

func (m *Manager) Create(id, workDir, command string, args []string) (*Session, error) {
	s := &Session{
		ID:        id,
		Name:      id,
		WorkDir:   workDir,
		State:     StateCreated,
		StartTime: time.Now(),
		Owned:     true,
		output:    NewRingBuf(int(m.bufferSize)),
	}

	cmd := exec.Command(command, args...)
	cmd.Dir = workDir

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("starting PTY: %w", err)
	}

	s.SetPTY(ptmx, cmd.Process)

	from := s.State
	s.State = StateRunning
	if m.onStateChange != nil {
		m.onStateChange(s, from, StateRunning)
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	// Read PTY output in background
	go m.readPTY(s)

	return s, nil
}

func (m *Manager) readPTY(s *Session) {
	buf := make([]byte, 4096)
	for {
		n, err := s.PTY().Read(buf)
		if n > 0 {
			s.Output().Write(buf[:n])
			m.broadcastOutput(s.ID, buf[:n])
		}
		if err != nil {
			if err != io.EOF {
				slog.Debug("PTY read error", "session", s.ID, "error", err)
			}
			break
		}
	}
}

type OutputFunc func(sessionID string, data []byte)

func (m *Manager) OnOutput(fn OutputFunc) {
	m.onOutput = fn
}

func (m *Manager) broadcastOutput(sessionID string, data []byte) {
	if m.onOutput != nil {
		m.onOutput(sessionID, data)
	}
}

func (m *Manager) Wait(id string) {
	s, ok := m.Get(id)
	if !ok {
		return
	}

	proc := s.proc
	if proc == nil {
		return
	}

	state, err := proc.Wait()

	from := s.GetState()
	if err != nil || !state.Success() {
		s.mu.Lock()
		s.State = StateErrored
		s.EndTime = time.Now()
		if state != nil {
			s.ExitCode = state.ExitCode()
		}
		s.mu.Unlock()
		if m.onStateChange != nil {
			m.onStateChange(s, from, StateErrored)
		}
	} else {
		s.mu.Lock()
		s.State = StateCompleted
		s.EndTime = time.Now()
		s.ExitCode = 0
		s.mu.Unlock()
		if m.onStateChange != nil {
			m.onStateChange(s, from, StateCompleted)
		}
	}

	// Close PTY
	if p := s.PTY(); p != nil {
		p.Close()
	}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

func (m *Manager) Kill(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	s.mu.RLock()
	proc := s.proc
	s.mu.RUnlock()

	if proc == nil {
		return fmt.Errorf("session %s has no process", id)
	}

	if err := proc.Signal(os.Kill); err != nil {
		return fmt.Errorf("killing session %s: %w", id, err)
	}
	return nil
}

// AddDiscovered adds an externally-discovered session (metadata only).
func (m *Manager) AddDiscovered(id, claudeID, workDir string, pid int, startTime time.Time) *Session {
	s := &Session{
		ID:        id,
		ClaudeID:  claudeID,
		Name:      workDir,
		WorkDir:   workDir,
		State:     StateDiscovered,
		PID:       pid,
		StartTime: startTime,
		Owned:     false,
		output:    NewRingBuf(int(m.bufferSize)),
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	return s
}

// Remove removes a session from the registry.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// WriteInput writes data to the session's PTY (user input from browser).
func (m *Manager) WriteInput(id string, data []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	p := s.PTY()
	if p == nil {
		return fmt.Errorf("session %s has no PTY", id)
	}

	_, err := p.Write(data)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/session/... -v -run TestManager
```

Expected: all 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/session/manager.go internal/session/manager_test.go
git commit -m "feat: session manager with PTY lifecycle, create/list/kill operations"
```

---

### Task 6: SQLite Store

**Files:**
- Create: `internal/store/store.go`, `internal/store/store_test.go`

- [ ] **Step 1: Write store tests**

Create `internal/store/store_test.go`:

```go
package store_test

import (
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/store"
)

func TestStore_SaveAndListSessions(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rec := store.SessionRecord{
		ID:        "s1",
		ClaudeID:  "claude-abc",
		WorkDir:   "/home/user/project",
		StartTime: time.Now().Add(-5 * time.Minute),
		EndTime:   time.Now(),
		ExitCode:  0,
		Status:    "completed",
	}
	if err := s.SaveSession(rec); err != nil {
		t.Fatal(err)
	}

	records, err := s.ListSessions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ID != "s1" {
		t.Errorf("expected ID s1, got %s", records[0].ID)
	}
}

func TestStore_SaveAndListNotifications(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n := store.NotificationRecord{
		SessionID: "s1",
		EventType: "completed",
		Timestamp: time.Now(),
		Read:      false,
	}
	if err := s.SaveNotification(n); err != nil {
		t.Fatal(err)
	}

	records, err := s.ListNotifications(10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(records))
	}
	if records[0].EventType != "completed" {
		t.Errorf("expected event completed, got %s", records[0].EventType)
	}
}

func TestStore_MarkNotificationRead(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	n := store.NotificationRecord{
		SessionID: "s1",
		EventType: "errored",
		Timestamp: time.Now(),
		Read:      false,
	}
	if err := s.SaveNotification(n); err != nil {
		t.Fatal(err)
	}

	records, _ := s.ListNotifications(10, false)
	if err := s.MarkNotificationRead(records[0].ID); err != nil {
		t.Fatal(err)
	}

	unread, _ := s.ListNotifications(10, false)
	if len(unread) != 0 {
		t.Errorf("expected 0 unread, got %d", len(unread))
	}
}

func TestStore_SaveAuditLog(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.LogAudit("create_session", "s1", "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/store/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement store package**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type SessionRecord struct {
	ID        string
	ClaudeID  string
	WorkDir   string
	StartTime time.Time
	EndTime   time.Time
	ExitCode  int
	Status    string
	PID       int
}

type NotificationRecord struct {
	ID        int64
	SessionID string
	EventType string
	Timestamp time.Time
	Read      bool
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		claude_id TEXT,
		work_dir TEXT,
		start_time DATETIME,
		end_time DATETIME,
		exit_code INTEGER,
		status TEXT,
		pid INTEGER
	);

	CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		event_type TEXT,
		timestamp DATETIME,
		read BOOLEAN DEFAULT FALSE
	);

	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action TEXT,
		session_id TEXT,
		client_ip TEXT,
		timestamp DATETIME
	);
	`
	_, err := db.Exec(schema)
	return err
}

func (s *Store) SaveSession(r SessionRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, claude_id, work_dir, start_time, end_time, exit_code, status, pid)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ClaudeID, r.WorkDir, r.StartTime, r.EndTime, r.ExitCode, r.Status, r.PID,
	)
	return err
}

func (s *Store) ListSessions(limit int) ([]SessionRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, claude_id, work_dir, start_time, end_time, exit_code, status, pid
		 FROM sessions ORDER BY start_time DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.ClaudeID, &r.WorkDir, &r.StartTime, &r.EndTime, &r.ExitCode, &r.Status, &r.PID); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) SaveNotification(n NotificationRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO notifications (session_id, event_type, timestamp, read) VALUES (?, ?, ?, ?)`,
		n.SessionID, n.EventType, n.Timestamp, n.Read,
	)
	return err
}

func (s *Store) ListNotifications(limit int, includeRead bool) ([]NotificationRecord, error) {
	query := `SELECT id, session_id, event_type, timestamp, read FROM notifications`
	if !includeRead {
		query += ` WHERE read = FALSE`
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []NotificationRecord
	for rows.Next() {
		var r NotificationRecord
		if err := rows.Scan(&r.ID, &r.SessionID, &r.EventType, &r.Timestamp, &r.Read); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) MarkNotificationRead(id int64) error {
	_, err := s.db.Exec(`UPDATE notifications SET read = TRUE WHERE id = ?`, id)
	return err
}

func (s *Store) LogAudit(action, sessionID, clientIP string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (action, session_id, client_ip, timestamp) VALUES (?, ?, ?, ?)`,
		action, sessionID, clientIP, time.Now(),
	)
	return err
}
```

- [ ] **Step 4: Add SQLite dependency**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/store/... -v
```

Expected: all 4 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat: SQLite store for session history, notifications, and audit log"
```

---

### Task 7: Notification Event Bus

**Files:**
- Create: `internal/notification/bus.go`, `internal/notification/bus_test.go`, `internal/notification/sink.go`, `internal/notification/sink_test.go`

- [ ] **Step 1: Write event bus tests**

Create `internal/notification/bus_test.go`:

```go
package notification_test

import (
	"sync"
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/notification"
)

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := notification.NewBus()

	var received []notification.SessionEvent
	var mu sync.Mutex

	bus.Subscribe(func(e notification.SessionEvent) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(notification.SessionEvent{
		SessionID: "s1",
		Type:      notification.EventCompleted,
		Timestamp: time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].SessionID != "s1" {
		t.Errorf("expected session s1, got %s", received[0].SessionID)
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := notification.NewBus()

	var count1, count2 int
	var mu sync.Mutex

	bus.Subscribe(func(e notification.SessionEvent) {
		mu.Lock()
		count1++
		mu.Unlock()
	})
	bus.Subscribe(func(e notification.SessionEvent) {
		mu.Lock()
		count2++
		mu.Unlock()
	})

	bus.Publish(notification.SessionEvent{
		SessionID: "s1",
		Type:      notification.EventErrored,
		Timestamp: time.Now(),
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both subscribers to receive 1 event, got %d and %d", count1, count2)
	}
}
```

- [ ] **Step 2: Write sink tests**

Create `internal/notification/sink_test.go`:

```go
package notification_test

import (
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/notification"
)

func TestInAppSink_StoresEvents(t *testing.T) {
	sink := notification.NewInAppSink(100)

	event := notification.SessionEvent{
		SessionID: "s1",
		Type:      notification.EventWaiting,
		Timestamp: time.Now(),
	}

	if err := sink.Send(event); err != nil {
		t.Fatal(err)
	}

	events := sink.Pending()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
}

func TestInAppSink_UnreadCount(t *testing.T) {
	sink := notification.NewInAppSink(100)

	sink.Send(notification.SessionEvent{SessionID: "s1", Type: notification.EventCompleted, Timestamp: time.Now()})
	sink.Send(notification.SessionEvent{SessionID: "s2", Type: notification.EventErrored, Timestamp: time.Now()})

	if sink.UnreadCount() != 2 {
		t.Errorf("expected 2 unread, got %d", sink.UnreadCount())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/notification/... -v
```

Expected: compilation errors.

- [ ] **Step 4: Implement event bus**

Create `internal/notification/bus.go`:

```go
package notification

import (
	"sync"
	"time"
)

type EventType string

const (
	EventCompleted EventType = "completed"
	EventErrored   EventType = "errored"
	EventWaiting   EventType = "waiting"
)

type SessionEvent struct {
	SessionID string
	Type      EventType
	Timestamp time.Time
	Message   string
}

type SubscriberFunc func(SessionEvent)

type Bus struct {
	mu          sync.RWMutex
	subscribers []SubscriberFunc
}

func NewBus() *Bus {
	return &Bus{}
}

func (b *Bus) Subscribe(fn SubscriberFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, fn)
}

func (b *Bus) Publish(e SessionEvent) {
	b.mu.RLock()
	subs := make([]SubscriberFunc, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()

	for _, fn := range subs {
		go fn(e)
	}
}
```

- [ ] **Step 5: Implement sinks**

Create `internal/notification/sink.go`:

```go
package notification

import "sync"

type Sink interface {
	Send(event SessionEvent) error
}

type InAppSink struct {
	mu     sync.Mutex
	events []SessionEvent
	max    int
}

func NewInAppSink(maxEvents int) *InAppSink {
	return &InAppSink{
		events: make([]SessionEvent, 0),
		max:    maxEvents,
	}
}

func (s *InAppSink) Send(event SessionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	if len(s.events) > s.max {
		s.events = s.events[len(s.events)-s.max:]
	}
	return nil
}

func (s *InAppSink) Pending() []SessionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionEvent, len(s.events))
	copy(out, s.events)
	return out
}

func (s *InAppSink) UnreadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *InAppSink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/notification/... -v
```

Expected: all 4 tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/notification/
git commit -m "feat: notification event bus with in-app sink"
```

---

### Task 8: Process Discovery Scanner

**Files:**
- Create: `internal/discovery/scanner.go`, `internal/discovery/scanner_test.go`

- [ ] **Step 1: Write scanner tests**

Create `internal/discovery/scanner_test.go`:

```go
package discovery_test

import (
	"testing"

	"github.com/igor-deoalves/websessions/internal/discovery"
)

func TestParseProcessInfo(t *testing.T) {
	// Test parsing a mock cmdline
	info, err := discovery.ParseCmdline("/usr/local/bin/claude --resume abc123 --session-id xyz")
	if err != nil {
		t.Fatal(err)
	}
	if info.Binary != "/usr/local/bin/claude" {
		t.Errorf("expected claude binary, got %s", info.Binary)
	}
}

func TestParseProcessInfo_NotClaude(t *testing.T) {
	_, err := discovery.ParseCmdline("/usr/bin/vim somefile.go")
	if err == nil {
		t.Error("expected error for non-claude process")
	}
}

func TestIsClaudeBinary(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"/usr/local/bin/claude", true},
		{"/home/user/.npm/bin/claude", true},
		{"/usr/bin/vim", false},
		{"claude", true},
	}
	for _, tt := range tests {
		if got := discovery.IsClaudeBinary(tt.path); got != tt.expect {
			t.Errorf("IsClaudeBinary(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/discovery/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement scanner**

Create `internal/discovery/scanner.go`:

```go
package discovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type ProcessInfo struct {
	PID       int
	Binary    string
	WorkDir   string
	Args      []string
	ClaudeID  string
	StartTime time.Time
}

func IsClaudeBinary(path string) bool {
	base := filepath.Base(path)
	return base == "claude"
}

func ParseCmdline(cmdline string) (*ProcessInfo, error) {
	parts := strings.Fields(cmdline)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty cmdline")
	}

	if !IsClaudeBinary(parts[0]) {
		return nil, fmt.Errorf("not a claude process: %s", parts[0])
	}

	info := &ProcessInfo{
		Binary: parts[0],
		Args:   parts[1:],
	}

	// Extract known flags
	for i, arg := range parts {
		switch arg {
		case "--resume":
			if i+1 < len(parts) {
				info.ClaudeID = parts[i+1]
			}
		case "--session-id":
			if i+1 < len(parts) && info.ClaudeID == "" {
				info.ClaudeID = parts[i+1]
			}
		}
	}

	return info, nil
}

// Scan finds running claude processes on the system.
func Scan() ([]ProcessInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return scanLinux()
	case "darwin":
		return scanDarwin()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func scanLinux() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	var results []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}

		// cmdline uses null bytes as separators
		cmdline := strings.ReplaceAll(string(cmdlineBytes), "\x00", " ")
		cmdline = strings.TrimSpace(cmdline)

		info, err := ParseCmdline(cmdline)
		if err != nil {
			continue
		}

		info.PID = pid

		// Read working directory
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err == nil {
			info.WorkDir = cwd
		}

		results = append(results, *info)
	}

	return results, nil
}

func scanDarwin() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil, fmt.Errorf("running ps: %w", err)
	}

	var results []ProcessInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		command := strings.Join(fields[10:], " ")
		info, err := ParseCmdline(command)
		if err != nil {
			continue
		}

		info.PID = pid
		results = append(results, *info)
	}

	return results, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/discovery/... -v
```

Expected: all 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/scanner.go internal/discovery/scanner_test.go
git commit -m "feat: process discovery scanner for Linux and macOS"
```

---

### Task 9: Session Takeover

**Files:**
- Create: `internal/discovery/takeover.go`, `internal/discovery/takeover_test.go`

- [ ] **Step 1: Write takeover tests**

Create `internal/discovery/takeover_test.go`:

```go
package discovery_test

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/discovery"
)

func TestKillProcess(t *testing.T) {
	// Start a sleep process to kill
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	pid := cmd.Process.Pid

	err := discovery.KillProcess(pid, 2*time.Second)
	if err != nil {
		t.Fatalf("kill error: %v", err)
	}

	// Verify process is gone
	proc, err := os.FindProcess(pid)
	if err == nil {
		// On Unix, FindProcess always succeeds. Signal(0) checks if process exists.
		err = proc.Signal(syscall.Signal(0))
		if err == nil {
			t.Error("expected process to be dead")
		}
	}
}

func TestKillProcess_NonExistent(t *testing.T) {
	err := discovery.KillProcess(999999999, 1*time.Second)
	if err == nil {
		t.Error("expected error for non-existent PID")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/discovery/... -v -run TestKill
```

Expected: compilation errors.

- [ ] **Step 3: Implement takeover**

Create `internal/discovery/takeover.go`:

```go
package discovery

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// KillProcess sends SIGTERM, waits for exit, then SIGKILL if needed.
// Uses signal polling instead of Wait() since discovered processes are not children.
func KillProcess(pid int, timeout time.Duration) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", pid, err)
	}

	// Verify process exists
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	// Try graceful shutdown first
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to %d: %w", pid, err)
	}

	// Poll for exit (can't use Wait() on non-child processes)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process is gone
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		// Already dead
		return nil
	}

	// Wait briefly for SIGKILL to take effect
	time.Sleep(200 * time.Millisecond)
	return nil
}

// Takeover kills an existing claude process and returns the info needed to resume.
func Takeover(info ProcessInfo, timeout time.Duration) error {
	if info.PID == 0 {
		return fmt.Errorf("no PID to take over")
	}
	return KillProcess(info.PID, timeout)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/discovery/... -v -run TestKill
```

Expected: both tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/takeover.go internal/discovery/takeover_test.go
git commit -m "feat: session takeover with graceful SIGTERM + SIGKILL fallback"
```

---

### Task 10: Vendor Static Assets

**Files:**
- Create: `web/static/htmx.min.js`, `web/static/xterm.js`, `web/static/xterm.css`, `web/static/xterm-addon-fit.js`, `web/static/split.min.js`, `web/embed.go`

- [ ] **Step 1: Download htmx**

```bash
mkdir -p web/static
curl -sL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o web/static/htmx.min.js
```

- [ ] **Step 2: Download xterm.js and addons**

```bash
curl -sL https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.min.js -o web/static/xterm.js
curl -sL https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.min.css -o web/static/xterm.css
curl -sL https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js -o web/static/xterm-addon-fit.js
```

- [ ] **Step 3: Download split.js**

```bash
curl -sL https://unpkg.com/split.js@1.6.5/dist/split.min.js -o web/static/split.min.js
```

- [ ] **Step 4: Create embed.go**

Create `web/embed.go`:

```go
package web

import "embed"

//go:embed static/*
var Static embed.FS
```

- [ ] **Step 5: Verify embed compiles**

```bash
go build ./web/...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/
git commit -m "feat: vendor static assets (htmx, xterm.js, split.js) with go:embed"
```

---

### Task 11: Templ Templates

**Files:**
- Create: `web/templates/layout.templ`, `web/templates/index.templ`, `web/templates/sidebar.templ`, `web/templates/terminal.templ`, `web/templates/tabs.templ`, `web/templates/notifications.templ`, `web/templates/newsession.templ`

- [ ] **Step 1: Install templ CLI**

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

- [ ] **Step 2: Add templ dependency**

```bash
go get github.com/a-h/templ
```

- [ ] **Step 3: Create layout template**

Create `web/templates/layout.templ`:

```
package templates

templ Layout(title string) {
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		<title>{ title }</title>
		<link rel="stylesheet" href="/static/xterm.css"/>
		<link rel="stylesheet" href="/static/style.css"/>
		<script src="/static/htmx.min.js"></script>
		<script src="/static/xterm.js"></script>
		<script src="/static/xterm-addon-fit.js"></script>
		<script src="/static/split.min.js"></script>
	</head>
	<body>
		{ children... }
		<script src="/static/app.js"></script>
	</body>
	</html>
}
```

- [ ] **Step 4: Create index template**

Create `web/templates/index.templ`:

```
package templates

type SessionView struct {
	ID      string
	Name    string
	WorkDir string
	State   string
	Owned   bool
}

type NotificationView struct {
	ID        int64
	SessionID string
	EventType string
	Message   string
}

type PageData struct {
	Sessions      []SessionView
	Notifications []NotificationView
	UnreadCount   int
}

templ Index(data PageData) {
	@Layout("websessions") {
		<div class="app">
			<header class="topbar">
				<h1 class="topbar-title">websessions</h1>
				<div class="topbar-actions">
					<button
						class="notification-bell"
						hx-get="/notifications"
						hx-target="#notification-dropdown"
						hx-swap="innerHTML"
					>
						<span class="bell-icon">&#128276;</span>
						if data.UnreadCount > 0 {
							<span class="badge">{ itoa(data.UnreadCount) }</span>
						}
					</button>
					<div id="notification-dropdown" class="dropdown"></div>
				</div>
			</header>
			<div class="main">
				<aside id="sidebar" class="sidebar" hx-get="/sidebar" hx-trigger="every 5s" hx-swap="innerHTML">
					@Sidebar(data.Sessions)
				</aside>
				<div class="content">
					<div id="tab-bar" class="tab-bar">
						@Tabs(nil, "")
					</div>
					<div id="terminal-area" class="terminal-area">
						<div class="empty-state">
							<p>Select a session from the sidebar or create a new one</p>
						</div>
					</div>
				</div>
			</div>
			<footer class="footer">
				<button
					class="new-session-btn"
					hx-get="/sessions/new"
					hx-target="#modal"
					hx-swap="innerHTML"
				>
					+ New Session
				</button>
			</footer>
			<div id="modal"></div>
		</div>
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
```

Note: add `"fmt"` to the import block that templ generates.

- [ ] **Step 5: Create sidebar template**

Create `web/templates/sidebar.templ`:

```
package templates

templ Sidebar(sessions []SessionView) {
	<div class="session-list">
		for _, s := range sessions {
			<div
				class={ "session-item", stateClass(s.State) }
				hx-post={ "/sessions/" + s.ID + "/open" }
				hx-target="#terminal-area"
				hx-swap="innerHTML"
			>
				<span class="status-dot"></span>
				<div class="session-info">
					<span class="session-name">{ s.Name }</span>
					<span class="session-dir">{ s.WorkDir }</span>
				</div>
				if s.State == "discovered" {
					<button
						class="takeover-btn"
						hx-post={ "/sessions/" + s.ID + "/takeover" }
						hx-swap="none"
					>
						Take Over
					</button>
				}
			</div>
		}
	</div>
}

func stateClass(state string) string {
	switch state {
	case "running":
		return "state-running"
	case "completed":
		return "state-completed"
	case "errored":
		return "state-errored"
	case "waiting":
		return "state-waiting"
	case "discovered":
		return "state-discovered"
	default:
		return ""
	}
}
```

- [ ] **Step 6: Create terminal template**

Create `web/templates/terminal.templ`:

```
package templates

templ Terminal(sessionID string, sessionName string, workDir string, state string) {
	<div class="terminal-pane" data-session-id={ sessionID }>
		<div class="pane-header">
			<span class="pane-title">{ sessionName }</span>
			<span class="pane-status">{ state }</span>
			<span class="pane-dir">{ workDir }</span>
			<div class="pane-actions">
				<button class="split-h-btn" onclick={ splitH(sessionID) } title="Split horizontal">&#9776;</button>
				<button class="split-v-btn" onclick={ splitV(sessionID) } title="Split vertical">&#9783;</button>
				<button class="maximize-btn" title="Maximize">&#9744;</button>
			</div>
		</div>
		<div class="terminal-container" id={ "term-" + sessionID }></div>
	</div>
}

script splitH(sessionID string) {
	window.websessions.splitPane(sessionID, "horizontal");
}

script splitV(sessionID string) {
	window.websessions.splitPane(sessionID, "vertical");
}
```

- [ ] **Step 7: Create tabs template**

Create `web/templates/tabs.templ`:

```
package templates

templ Tabs(openSessions []SessionView, activeID string) {
	<div class="tabs">
		if openSessions != nil {
			for _, s := range openSessions {
				<button
					class={ "tab", tabActiveClass(s.ID, activeID) }
					hx-post={ "/sessions/" + s.ID + "/open" }
					hx-target="#terminal-area"
					hx-swap="innerHTML"
				>
					<span class={ "tab-dot", stateClass(s.State) }></span>
					{ s.Name }
					<span class="tab-close" hx-delete={ "/tabs/" + s.ID } hx-swap="none">&times;</span>
				</button>
			}
		}
		<button
			class="tab tab-new"
			hx-get="/sessions/new"
			hx-target="#modal"
			hx-swap="innerHTML"
		>+</button>
	</div>
}

func tabActiveClass(id, activeID string) string {
	if id == activeID {
		return "tab-active"
	}
	return ""
}
```

- [ ] **Step 8: Create notifications template**

Create `web/templates/notifications.templ`:

```
package templates

templ Notifications(notifications []NotificationView) {
	<div class="notification-list">
		if len(notifications) == 0 {
			<p class="no-notifications">No new notifications</p>
		}
		for _, n := range notifications {
			<div class={ "notification-item", "notif-" + n.EventType }>
				<span class="notif-type">{ n.EventType }</span>
				<span class="notif-session">{ n.SessionID }</span>
				<button
					class="notif-dismiss"
					hx-post={ fmt.Sprintf("/notifications/%d/read", n.ID) }
					hx-swap="outerHTML"
					hx-target="closest .notification-item"
				>&times;</button>
			</div>
		}
	</div>
}
```

Note: add `"fmt"` import.

- [ ] **Step 9: Create new session modal template**

Create `web/templates/newsession.templ`:

```
package templates

templ NewSessionModal(defaultDir string) {
	<div class="modal-overlay" onclick="this.remove()">
		<div class="modal-content" onclick="event.stopPropagation()">
			<h2>New Session</h2>
			<form hx-post="/sessions" hx-target="#sidebar" hx-swap="innerHTML">
				<div class="form-group">
					<label for="name">Session Name</label>
					<input type="text" id="name" name="name" placeholder="my-session" required/>
				</div>
				<div class="form-group">
					<label for="work_dir">Working Directory</label>
					<input type="text" id="work_dir" name="work_dir" value={ defaultDir } required/>
				</div>
				<div class="form-group">
					<label for="prompt">Initial Prompt (optional)</label>
					<textarea id="prompt" name="prompt" rows="3" placeholder="What should Claude work on?"></textarea>
				</div>
				<div class="form-actions">
					<button type="button" class="btn-cancel" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
					<button type="submit" class="btn-create">Create</button>
				</div>
			</form>
		</div>
	</div>
}
```

- [ ] **Step 10: Generate templ files**

```bash
templ generate ./web/templates/
```

Expected: generates `*_templ.go` files with no errors.

- [ ] **Step 11: Verify everything compiles**

```bash
go build ./web/...
```

Expected: no errors.

- [ ] **Step 12: Commit**

```bash
git add web/templates/
git commit -m "feat: templ templates for layout, sidebar, terminal, tabs, notifications, and new session modal"
```

---

### Task 12: CSS Styles

**Files:**
- Create: `web/static/style.css`

- [ ] **Step 1: Create stylesheet**

Create `web/static/style.css`:

```css
* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg-primary: #1a1b26;
  --bg-secondary: #24283b;
  --bg-tertiary: #292e42;
  --text-primary: #c0caf5;
  --text-secondary: #a9b1d6;
  --text-muted: #565f89;
  --accent: #7aa2f7;
  --green: #9ece6a;
  --red: #f7768e;
  --yellow: #e0af68;
  --blue: #7dcfff;
  --border: #3b4261;
}

html, body { height: 100%; background: var(--bg-primary); color: var(--text-primary); font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 14px; }

.app { display: flex; flex-direction: column; height: 100vh; }

/* Top bar */
.topbar { display: flex; align-items: center; justify-content: space-between; padding: 0.5rem 1rem; background: var(--bg-secondary); border-bottom: 1px solid var(--border); height: 48px; }
.topbar-title { font-size: 1rem; font-weight: 600; color: var(--accent); }
.topbar-actions { display: flex; align-items: center; gap: 0.5rem; }
.notification-bell { background: none; border: none; color: var(--text-primary); cursor: pointer; position: relative; font-size: 1.2rem; padding: 0.25rem 0.5rem; }
.badge { position: absolute; top: -4px; right: -4px; background: var(--red); color: white; font-size: 0.65rem; padding: 0.1rem 0.3rem; border-radius: 8px; }
.dropdown { position: absolute; right: 0; top: 100%; background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 4px; min-width: 280px; z-index: 100; }

/* Main layout */
.main { display: flex; flex: 1; overflow: hidden; }

/* Sidebar */
.sidebar { width: 240px; background: var(--bg-secondary); border-right: 1px solid var(--border); overflow-y: auto; flex-shrink: 0; }
.session-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 0.75rem; cursor: pointer; border-bottom: 1px solid var(--border); }
.session-item:hover { background: var(--bg-tertiary); }
.status-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.state-running .status-dot { background: var(--green); }
.state-completed .status-dot { background: var(--text-muted); }
.state-errored .status-dot { background: var(--red); }
.state-waiting .status-dot { background: var(--yellow); animation: pulse 1.5s infinite; }
.state-discovered .status-dot { background: var(--blue); }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
.session-info { display: flex; flex-direction: column; overflow: hidden; }
.session-name { font-size: 0.85rem; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.session-dir { font-size: 0.7rem; color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.takeover-btn { background: var(--accent); color: var(--bg-primary); border: none; padding: 0.2rem 0.5rem; border-radius: 3px; font-size: 0.7rem; cursor: pointer; white-space: nowrap; }

/* Content */
.content { flex: 1; display: flex; flex-direction: column; overflow: hidden; }

/* Tabs */
.tab-bar { display: flex; background: var(--bg-secondary); border-bottom: 1px solid var(--border); overflow-x: auto; }
.tab { display: flex; align-items: center; gap: 0.4rem; padding: 0.4rem 0.75rem; background: none; border: none; border-right: 1px solid var(--border); color: var(--text-secondary); cursor: pointer; font-size: 0.8rem; white-space: nowrap; }
.tab:hover { background: var(--bg-tertiary); }
.tab-active { background: var(--bg-primary); color: var(--text-primary); }
.tab-dot { width: 6px; height: 6px; border-radius: 50%; }
.tab-close { margin-left: 0.5rem; opacity: 0.5; }
.tab-close:hover { opacity: 1; }
.tab-new { color: var(--text-muted); border-right: none; }

/* Terminal area */
.terminal-area { flex: 1; display: flex; overflow: hidden; background: var(--bg-primary); }
.terminal-pane { display: flex; flex-direction: column; flex: 1; overflow: hidden; }
.pane-header { display: flex; align-items: center; gap: 0.5rem; padding: 0.25rem 0.5rem; background: var(--bg-tertiary); border-bottom: 1px solid var(--border); font-size: 0.75rem; }
.pane-title { font-weight: 500; }
.pane-status { color: var(--text-muted); }
.pane-dir { color: var(--text-muted); margin-left: auto; }
.pane-actions { display: flex; gap: 0.25rem; }
.pane-actions button { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 0.8rem; padding: 0.1rem 0.3rem; }
.pane-actions button:hover { color: var(--text-primary); }
.terminal-container { flex: 1; }
.empty-state { display: flex; align-items: center; justify-content: center; flex: 1; color: var(--text-muted); }

/* Footer */
.footer { display: flex; padding: 0.4rem 1rem; background: var(--bg-secondary); border-top: 1px solid var(--border); }
.new-session-btn { background: none; border: 1px solid var(--border); color: var(--text-secondary); padding: 0.3rem 0.75rem; border-radius: 4px; cursor: pointer; font-size: 0.8rem; }
.new-session-btn:hover { background: var(--bg-tertiary); color: var(--text-primary); }

/* Modal */
.modal-overlay { position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 200; }
.modal-content { background: var(--bg-secondary); border: 1px solid var(--border); border-radius: 8px; padding: 1.5rem; width: 400px; }
.modal-content h2 { margin-bottom: 1rem; font-size: 1.1rem; }
.form-group { margin-bottom: 1rem; }
.form-group label { display: block; margin-bottom: 0.3rem; font-size: 0.8rem; color: var(--text-secondary); }
.form-group input, .form-group textarea { width: 100%; background: var(--bg-primary); border: 1px solid var(--border); color: var(--text-primary); padding: 0.5rem; border-radius: 4px; font-family: inherit; font-size: 0.85rem; }
.form-actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
.btn-cancel { background: none; border: 1px solid var(--border); color: var(--text-secondary); padding: 0.4rem 1rem; border-radius: 4px; cursor: pointer; }
.btn-create { background: var(--accent); border: none; color: var(--bg-primary); padding: 0.4rem 1rem; border-radius: 4px; cursor: pointer; font-weight: 500; }

/* Notifications */
.notification-list { padding: 0.5rem; }
.notification-item { display: flex; align-items: center; gap: 0.5rem; padding: 0.4rem 0.5rem; border-bottom: 1px solid var(--border); font-size: 0.8rem; }
.notif-completed .notif-type { color: var(--green); }
.notif-errored .notif-type { color: var(--red); }
.notif-waiting .notif-type { color: var(--yellow); }
.notif-dismiss { background: none; border: none; color: var(--text-muted); cursor: pointer; margin-left: auto; }
.no-notifications { color: var(--text-muted); font-size: 0.8rem; text-align: center; padding: 1rem; }

/* Split panes (split.js gutter) */
.gutter { background-color: var(--border); background-repeat: no-repeat; background-position: 50%; }
.gutter.gutter-horizontal { background-image: url('data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAUAAAAeCAYAAADkftS9AAAAIklEQVQoU2M4c+bMfxAGAgYYmwGrIIiDjrELjpo5aiZeMwF+yNnOs5KSvgAAAABJRU5ErkJggg=='); cursor: col-resize; }
.gutter.gutter-vertical { background-image: url('data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAB4AAAAFAQMAAABo7865AAAABlBMVEVHcEzMzMzyAv2sAAAAAXRSTlMAQObYZgAAABBJREFUeF5jOAMEEAIEEFwAn3kMwQBgbEYAAAAASUVORK5CYII='); cursor: row-resize; }

/* Responsive */
@media (max-width: 768px) {
  .sidebar { display: none; }
  .content { width: 100%; }
}
```

- [ ] **Step 2: Commit**

```bash
git add web/static/style.css
git commit -m "feat: dark theme CSS with Tokyo Night color scheme"
```

---

### Task 13: Client-Side JavaScript

**Files:**
- Create: `web/static/app.js`

- [ ] **Step 1: Create app.js**

Create `web/static/app.js`:

```javascript
window.websessions = (function() {
  const terminals = {};  // sessionID -> { term, ws, fitAddon }
  const splitInstances = [];

  function connectSession(sessionID, containerID) {
    const container = document.getElementById(containerID);
    if (!container) return;

    const term = new Terminal({
      cursorBlink: true,
      theme: {
        background: '#1a1b26',
        foreground: '#c0caf5',
        cursor: '#c0caf5',
        selectionBackground: '#33467c',
      },
      fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
      fontSize: 14,
    });

    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    fitAddon.fit();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws/${sessionID}`);

    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      // Send initial size
      const dims = { type: 'resize', rows: term.rows, cols: term.cols };
      ws.send(JSON.stringify(dims));
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        // Could be a JSON notification
        try {
          const msg = JSON.parse(event.data);
          if (msg.type === 'notification') {
            handleNotification(msg);
          }
        } catch(e) {
          term.write(event.data);
        }
      }
    };

    ws.onclose = () => {
      term.write('\r\n\x1b[33m[Connection closed]\x1b[0m\r\n');
    };

    // Send user input to server
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    // Handle resize
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
      if (ws.readyState === WebSocket.OPEN) {
        const dims = { type: 'resize', rows: term.rows, cols: term.cols };
        ws.send(JSON.stringify(dims));
      }
    });
    resizeObserver.observe(container);

    terminals[sessionID] = { term, ws, fitAddon, resizeObserver };
  }

  function disconnectSession(sessionID) {
    const t = terminals[sessionID];
    if (!t) return;
    t.ws.close();
    t.resizeObserver.disconnect();
    t.term.dispose();
    delete terminals[sessionID];
  }

  function splitPane(sessionID, direction) {
    // The terminal-area contains panes. Splitting creates a new container.
    const area = document.getElementById('terminal-area');
    if (!area) return;

    // Request a new pane from server via htmx
    htmx.ajax('POST', `/sessions/${sessionID}/split?direction=${direction}`, {
      target: '#terminal-area',
      swap: 'innerHTML'
    });
  }

  function handleNotification(msg) {
    // Update badge count
    const badge = document.querySelector('.badge');
    if (badge) {
      const count = parseInt(badge.textContent || '0') + 1;
      badge.textContent = count;
    }

    // Desktop notification
    if (Notification.permission === 'granted') {
      new Notification(`websessions: ${msg.event}`, {
        body: `Session ${msg.sessionID}: ${msg.event}`,
        tag: `ws-${msg.sessionID}-${msg.event}`,
      });
    }
  }

  // Request notification permission on load
  if ('Notification' in window && Notification.permission === 'default') {
    Notification.requestPermission();
  }

  // Hook into htmx to initialize terminals after content swap
  document.addEventListener('htmx:afterSwap', (event) => {
    const panes = event.detail.target.querySelectorAll('.terminal-pane[data-session-id]');
    panes.forEach((pane) => {
      const sessionID = pane.dataset.sessionId;
      const containerID = `term-${sessionID}`;
      if (!terminals[sessionID]) {
        connectSession(sessionID, containerID);
      }
    });
  });

  return {
    connectSession,
    disconnectSession,
    splitPane,
    terminals,
  };
})();
```

- [ ] **Step 2: Commit**

```bash
git add web/static/app.js
git commit -m "feat: client-side JS for terminal WebSocket, split panes, and notifications"
```

---

### Task 14: HTTP Server and Routes

**Files:**
- Create: `internal/server/server.go`, `internal/server/handlers.go`, `internal/server/ws.go`, `internal/server/auth.go`, `internal/server/server_test.go`

- [ ] **Step 1: Write server tests**

Create `internal/server/server_test.go`:

```go
package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/server"
	"github.com/igor-deoalves/websessions/internal/session"
)

func newTestServer() *server.Server {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Sessions: config.SessionsConfig{OutputBufferSize: 1024, DefaultDir: "/tmp"},
		Auth:     config.AuthConfig{Enabled: false},
	}
	mgr := session.NewManager(1024)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	return server.New(cfg, mgr, bus, sink)
}

func TestServer_IndexPage(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Error("expected Content-Type header")
	}
}

func TestServer_StaticFiles(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServer_AuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Auth:   config.AuthConfig{Enabled: true, Token: "secret"},
	}
	mgr := session.NewManager(1024)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	srv := server.New(cfg, mgr, bus, sink)

	// Without token
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}

	// With token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with token, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/server/... -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement auth middleware**

Create `internal/server/auth.go`:

```go
package server

import (
	"net/http"
	"strings"
)

func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow static files and WebSocket upgrades without auth header check
			// (WebSocket sends token as query param)
			auth := r.Header.Get("Authorization")
			qToken := r.URL.Query().Get("token")

			bearerToken := ""
			if strings.HasPrefix(auth, "Bearer ") {
				bearerToken = strings.TrimPrefix(auth, "Bearer ")
			}

			if bearerToken != token && qToken != token {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Implement WebSocket handler**

Create `internal/server/ws.go`:

```go
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/igor-deoalves/websessions/internal/session"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsMessage struct {
	Type string `json:"type"`
	Rows int    `json:"rows,omitempty"`
	Cols int    `json:"cols,omitempty"`
}

type wsHub struct {
	mu      sync.RWMutex
	clients map[string]map[*websocket.Conn]bool // sessionID -> connections
}

func newWSHub() *wsHub {
	return &wsHub{
		clients: make(map[string]map[*websocket.Conn]bool),
	}
}

func (h *wsHub) add(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[sessionID] == nil {
		h.clients[sessionID] = make(map[*websocket.Conn]bool)
	}
	h.clients[sessionID][conn] = true
}

func (h *wsHub) remove(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[sessionID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, sessionID)
		}
	}
}

func (h *wsHub) broadcast(sessionID string, data []byte) {
	h.mu.RLock()
	conns := h.clients[sessionID]
	h.mu.RUnlock()

	for conn := range conns {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			slog.Debug("ws write error", "error", err)
			conn.Close()
			h.remove(sessionID, conn)
		}
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request, sessionID string, mgr *session.Manager) {
	sess, ok := mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	s.hub.add(sessionID, conn)
	defer s.hub.remove(sessionID, conn)

	// Replay ring buffer
	if buf := sess.Output().Bytes(); len(buf) > 0 {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		conn.WriteMessage(websocket.BinaryMessage, buf)
	}

	// Read user input
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		switch msgType {
		case websocket.TextMessage:
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err == nil && msg.Type == "resize" {
				sess.Resize(uint16(msg.Rows), uint16(msg.Cols))
				continue
			}
			// Text input
			mgr.WriteInput(sessionID, data)
		case websocket.BinaryMessage:
			mgr.WriteInput(sessionID, data)
		}
	}
}
```

- [ ] **Step 5: Implement HTTP handlers**

Create `internal/server/handlers.go`:

```go
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/igor-deoalves/websessions/internal/discovery"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/web/templates"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}

	data := templates.PageData{
		Sessions:    views,
		UnreadCount: s.sink.UnreadCount(),
	}

	templates.Index(data).Render(r.Context(), w)
}

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}
	templates.Sidebar(views).Render(r.Context(), w)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	workDir := r.FormValue("work_dir")
	prompt := r.FormValue("prompt")

	if name == "" || workDir == "" {
		http.Error(w, "name and work_dir required", http.StatusBadRequest)
		return
	}

	args := []string{}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}

	_, err := s.mgr.Create(name, workDir, "claude", args)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated sidebar
	s.handleSidebar(w, r)
}

func (s *Server) handleOpenSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	v := sessionToView(sess)
	templates.Terminal(v.ID, v.Name, v.WorkDir, v.State).Render(r.Context(), w)
}

func (s *Server) handleNewSessionModal(w http.ResponseWriter, r *http.Request) {
	templates.NewSessionModal(s.cfg.Sessions.DefaultDir).Render(r.Context(), w)
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	events := s.sink.Pending()
	views := make([]templates.NotificationView, len(events))
	for i, e := range events {
		views[i] = templates.NotificationView{
			SessionID: e.SessionID,
			EventType: string(e.Type),
		}
	}
	templates.Notifications(views).Render(r.Context(), w)
}

func sessionToView(s *session.Session) templates.SessionView {
	return templates.SessionView{
		ID:      s.ID,
		Name:    s.Name,
		WorkDir: s.WorkDir,
		State:   string(s.GetState()),
		Owned:   s.Owned,
	}
}

func (s *Server) handleTakeover(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if sess.GetState() != session.StateDiscovered {
		http.Error(w, "session is not in discovered state", http.StatusBadRequest)
		return
	}

	claudeID := sess.ClaudeID
	workDir := sess.WorkDir
	pid := sess.PID

	// Kill the original process
	if err := discovery.KillProcess(pid, 5*time.Second); err != nil {
		slog.Error("takeover kill failed", "session", sessionID, "error", err)
		http.Error(w, "failed to kill process: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the discovered session
	s.mgr.Remove(sessionID)

	// Resume with a new session
	args := []string{"--resume", claudeID}
	newSess, err := s.mgr.Create(sessionID, workDir, "claude", args)
	if err != nil {
		slog.Error("takeover resume failed", "session", sessionID, "error", err)
		http.Error(w, "failed to resume session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	v := sessionToView(newSess)
	templates.Terminal(v.ID, v.Name, v.WorkDir, v.State).Render(r.Context(), w)
}

func (s *Server) setupNotificationBridge() {
	s.bus.Subscribe(func(e notification.SessionEvent) {
		s.sink.Send(e)
		// Broadcast notification to all connected WS clients
		// (future: send JSON notification message through hub)
	})
}
```

- [ ] **Step 6: Implement server setup and routing**

Create `internal/server/server.go`:

```go
package server

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/web"
)

type Server struct {
	cfg     *config.Config
	mgr     *session.Manager
	bus     *notification.Bus
	sink    *notification.InAppSink
	hub     *wsHub
	handler http.Handler
}

func New(cfg *config.Config, mgr *session.Manager, bus *notification.Bus, sink *notification.InAppSink) *Server {
	s := &Server{
		cfg:  cfg,
		mgr:  mgr,
		bus:  bus,
		sink: sink,
		hub:  newWSHub(),
	}
	s.handler = s.routes()
	s.setupNotificationBridge()

	// Wire PTY output to WebSocket hub
	mgr.OnOutput(func(sessionID string, data []byte) {
		s.hub.broadcast(sessionID, data)
	})

	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()

	// Static files (served WITHOUT auth — CSS/JS must load)
	staticFS, _ := fs.Sub(web.Static, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// All other routes go in a group with auth middleware
	r.Group(func(r chi.Router) {
		if s.cfg.Auth.Enabled && s.cfg.Auth.Token != "" {
			r.Use(authMiddleware(s.cfg.Auth.Token))
		}

		// Pages
		r.Get("/", s.handleIndex)
		r.Get("/sidebar", s.handleSidebar)
		r.Get("/notifications", s.handleNotifications)

		// Sessions
		r.Get("/sessions/new", s.handleNewSessionModal)
		r.Post("/sessions", s.handleCreateSession)
		r.Post("/sessions/{sessionID}/open", func(w http.ResponseWriter, r *http.Request) {
			s.handleOpenSession(w, r, chi.URLParam(r, "sessionID"))
		})
		r.Post("/sessions/{sessionID}/takeover", func(w http.ResponseWriter, r *http.Request) {
			s.handleTakeover(w, r, chi.URLParam(r, "sessionID"))
		})

		// WebSocket
		r.Get("/ws/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
			s.handleWS(w, r, chi.URLParam(r, "sessionID"), s.mgr)
		})
	})

	return r
}
```

- [ ] **Step 7: Add chi and gorilla/websocket dependencies**

```bash
go get github.com/go-chi/chi/v5
go get github.com/gorilla/websocket
```

- [ ] **Step 8: Generate templ and run tests**

```bash
templ generate ./web/templates/ && go test ./internal/server/... -v
```

Expected: all 3 tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/server/ go.mod go.sum
git commit -m "feat: HTTP server with routes, WebSocket handler, auth middleware, and htmx handlers"
```

---

### Task 15: Main Entry Point and Wiring

**Files:**
- Modify: `cmd/websessions/main.go`

- [ ] **Step 1: Wire everything together**

Replace `cmd/websessions/main.go` with:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/discovery"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/server"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/internal/store"
)

func main() {
	// Parse flags
	configPath := ""
	logLevel := "info"
	for i, arg := range os.Args[1:] {
		switch arg {
		case "--config":
			if i+1 < len(os.Args)-1 {
				configPath = os.Args[i+2]
			}
		case "--log-level":
			if i+1 < len(os.Args)-1 {
				logLevel = os.Args[i+2]
			}
		}
	}

	// Setup logging
	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	// Load config (default to ~/.websessions/config.yaml if no --config flag)
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		defaultPath := homeDir + "/.websessions/config.yaml"
		if _, err := os.Stat(defaultPath); err == nil {
			configPath = defaultPath
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Open store
	homeDir, _ := os.UserHomeDir()
	dbDir := homeDir + "/.websessions"
	os.MkdirAll(dbDir, 0755)
	dbPath := dbDir + "/websessions.db"

	st, err := store.Open(dbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Create components
	mgr := session.NewManager(cfg.Sessions.OutputBufferSize)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)

	// Wire state changes to notification bus and store
	mgr.OnStateChange(func(s *session.Session, from, to session.State) {
		var eventType notification.EventType
		switch to {
		case session.StateCompleted:
			eventType = notification.EventCompleted
		case session.StateErrored:
			eventType = notification.EventErrored
		case session.StateWaiting:
			eventType = notification.EventWaiting
		default:
			return
		}

		event := notification.SessionEvent{
			SessionID: s.ID,
			Type:      eventType,
			Timestamp: time.Now(),
		}
		bus.Publish(event)

		// Persist to store
		st.SaveSession(store.SessionRecord{
			ID:        s.ID,
			ClaudeID:  s.ClaudeID,
			WorkDir:   s.WorkDir,
			StartTime: s.StartTime,
			EndTime:   s.EndTime,
			ExitCode:  s.ExitCode,
			Status:    string(to),
			PID:       s.PID,
		})
		st.SaveNotification(store.NotificationRecord{
			SessionID: s.ID,
			EventType: string(eventType),
			Timestamp: time.Now(),
		})
	})

	// Run initial discovery
	go func() {
		processes, err := discovery.Scan()
		if err != nil {
			slog.Warn("initial discovery scan failed", "error", err)
			return
		}
		for _, p := range processes {
			id := fmt.Sprintf("discovered-%d", p.PID)
			mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
			slog.Info("discovered claude session", "pid", p.PID, "dir", p.WorkDir)
		}
	}()

	// Periodic discovery
	if cfg.Sessions.ScanInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.Sessions.ScanInterval)
			defer ticker.Stop()
			for range ticker.C {
				processes, err := discovery.Scan()
				if err != nil {
					slog.Debug("discovery scan error", "error", err)
					continue
				}
				for _, p := range processes {
					// Skip if already tracked
					existing := mgr.List()
					found := false
					for _, s := range existing {
						if s.PID == p.PID {
							found = true
							break
						}
					}
					if !found {
						id := fmt.Sprintf("discovered-%d", p.PID)
						mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
						slog.Info("discovered new claude session", "pid", p.PID)
					}
				}
			}
		}()
	}

	// Create and start server
	srv := server.New(cfg, mgr, bus, sink)

	httpServer := &http.Server{
		Addr:    srv.Addr(),
		Handler: srv.Handler(),
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("websessions starting", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down...")

	// Persist session metadata
	for _, s := range mgr.List() {
		st.SaveSession(store.SessionRecord{
			ID:        s.ID,
			ClaudeID:  s.ClaudeID,
			WorkDir:   s.WorkDir,
			StartTime: s.StartTime,
			EndTime:   s.EndTime,
			ExitCode:  s.ExitCode,
			Status:    string(s.GetState()),
			PID:       s.PID,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)

	slog.Info("websessions stopped")
}
```

- [ ] **Step 2: Verify build**

```bash
templ generate ./web/templates/ && go build -o bin/websessions ./cmd/websessions
```

Expected: binary builds successfully.

- [ ] **Step 3: Smoke test**

```bash
./bin/websessions --log-level debug &
sleep 1
curl -s http://localhost:8080 | head -5
kill %1
```

Expected: HTML response from the server.

- [ ] **Step 4: Commit**

```bash
git add cmd/websessions/main.go
git commit -m "feat: main entry point wiring config, store, session manager, discovery, and HTTP server"
```

---

### Task 16: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Create multi-stage Dockerfile**

Create `Dockerfile`:

```dockerfile
FROM golang:1.22-alpine AS builder

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN templ generate ./web/templates/
RUN CGO_ENABLED=0 go build -o websessions ./cmd/websessions

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/websessions /usr/local/bin/websessions

EXPOSE 8080
ENTRYPOINT ["websessions"]
```

- [ ] **Step 2: Verify Docker build**

```bash
docker build -t websessions .
```

Expected: image builds successfully.

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "feat: multi-stage Dockerfile for single binary deployment"
```

---

### Task 17: Integration Test — End to End

**Files:**
- Create: `internal/server/integration_test.go`

- [ ] **Step 1: Write integration test**

Create `internal/server/integration_test.go`:

```go
//go:build integration

package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/server"
	"github.com/igor-deoalves/websessions/internal/session"
)

func TestIntegration_CreateSessionAndStream(t *testing.T) {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Sessions: config.SessionsConfig{OutputBufferSize: 1024 * 1024, DefaultDir: "/tmp"},
		Auth:     config.AuthConfig{Enabled: false},
	}
	mgr := session.NewManager(cfg.Sessions.OutputBufferSize)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	srv := server.New(cfg, mgr, bus, sink)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create a session that runs echo
	_, err := mgr.Create("test-echo", "/tmp", "echo", []string{"hello from integration test"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Wait for process to complete
	time.Sleep(500 * time.Millisecond)

	// Connect via WebSocket
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/test-echo"
	u, _ := url.Parse(wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	// Should receive ring buffer replay with our echo output
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}

	if !strings.Contains(string(msg), "hello from integration test") {
		t.Errorf("expected echo output in WS message, got: %q", string(msg))
	}

	// Verify index page loads
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run integration test**

```bash
go test ./internal/server/... -v -tags=integration -run TestIntegration
```

Expected: test passes.

- [ ] **Step 3: Commit**

```bash
git add internal/server/integration_test.go
git commit -m "test: integration test for session creation and WebSocket streaming"
```

---

### Task 18: CLAUDE.md

**Files:**
- Create: `CLAUDE.md`

- [ ] **Step 1: Create CLAUDE.md**

Create `CLAUDE.md`:

```markdown
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

websessions is a web-based command center for managing multiple Claude Code CLI sessions. Single Go binary serving an htmx+Templ UI. Full spec at `docs/superpowers/specs/2026-03-23-websessions-design.md`.

## Build & Run

```bash
make build          # builds to bin/websessions (runs templ generate first)
make run            # go run with templ generate
make test           # go test ./... -v
make lint           # golangci-lint
templ generate      # regenerate templ templates (required before build)
```

Run a single test:
```bash
go test ./internal/session/... -v -run TestManager_CreateSession
```

Integration tests (require `integration` build tag):
```bash
go test ./internal/server/... -v -tags=integration
```

## Architecture

```
cmd/websessions/main.go     → entry point, wires all components, signal handling
internal/config/             → YAML config loading with defaults and env overrides
internal/store/              → SQLite (modernc.org/sqlite, pure Go) for history/audit
internal/session/            → Session manager: PTY lifecycle, ring buffer, state machine
internal/discovery/          → Process scanner (/proc on Linux, ps on macOS), kill+resume takeover
internal/notification/       → Event bus with NotificationSink interface (in-app sink for v1)
internal/server/             → chi router, htmx handlers, WebSocket terminal streaming, auth middleware
web/templates/               → Templ files (.templ) — must run `templ generate` before building
web/static/                  → Vendored JS (htmx, xterm.js, split.js) + CSS, embedded via go:embed
```

Key patterns:
- Session state machine in `internal/session/session.go` — all transitions validated
- WebSocket hub in `internal/server/ws.go` — multiplexes PTY output to multiple browser clients
- Ring buffer in `internal/session/ringbuf.go` — replayed to new WS clients on connect
- State changes fire through notification.Bus → sinks + SQLite persistence
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add CLAUDE.md with build commands and architecture overview"
```
