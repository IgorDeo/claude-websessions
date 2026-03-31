package session

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/IgorDeo/claude-websessions/internal/discovery"
	"github.com/IgorDeo/claude-websessions/internal/docker"
)

type StateChangeFunc func(s *Session, from, to State)
type OutputFunc func(sessionID string, data []byte)

type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	bufferSize    int64
	onStateChange StateChangeFunc
	onOutput      OutputFunc
	stopReaders   map[string]chan struct{} // signal to stop reading for a session
	killedPIDs    map[int]time.Time       // PIDs of killed sessions, with kill time
}

func NewManager(bufferSize int64) *Manager {
	return &Manager{
		sessions:    make(map[string]*Session),
		bufferSize:  bufferSize,
		stopReaders: make(map[string]chan struct{}),
		killedPIDs:  make(map[int]time.Time),
	}
}

// TrackKilledPID records a PID as killed so discovery skips it.
func (m *Manager) TrackKilledPID(pid int) {
	if pid <= 0 {
		return
	}
	m.mu.Lock()
	m.killedPIDs[pid] = time.Now()
	m.mu.Unlock()
}

// WasKilled reports whether a PID was recently killed (within the last 5 minutes).
func (m *Manager) WasKilled(pid int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.killedPIDs[pid]
	if !ok {
		return false
	}
	// Expire after 5 minutes so stale entries don't accumulate
	if time.Since(t) > 5*time.Minute {
		return false
	}
	return true
}

// CreateOptions holds optional parameters for session creation.
type CreateOptions struct {
	Sandboxed bool
}

func (m *Manager) OnStateChange(fn StateChangeFunc) { m.onStateChange = fn }
func (m *Manager) OnOutput(fn OutputFunc)            { m.onOutput = fn }

// Create creates a new session inside a tmux session.
// When opts.Sandboxed is true, the session runs inside a Docker Desktop sandbox VM.
func (m *Manager) Create(id, workDir, command string, args []string, opts ...*CreateOptions) (*Session, error) {
	var opt *CreateOptions
	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	}
	sandboxed := opt != nil && opt.Sandboxed

	// Expand ~ in workDir
	if len(workDir) > 0 && workDir[0] == '~' {
		home, _ := os.UserHomeDir()
		workDir = home + workDir[1:]
	}

	// Validate directory exists
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("working directory does not exist: %s", workDir)
	}

	if sandboxed {
		// Sandbox mode: return session immediately in "starting" state,
		// then provision the sandbox VM asynchronously to avoid blocking the UI.
		s := &Session{
			ID:          id,
			Name:        id,
			WorkDir:     workDir,
			State:       StateStarting,
			StartTime:   time.Now(),
			Owned:       true,
			Sandboxed:   true,
			output:      NewRingBuf(int(m.bufferSize)),
		}

		m.mu.Lock()
		m.sessions[id] = s
		m.mu.Unlock()

		if m.onStateChange != nil {
			m.onStateChange(s, StateCreated, StateStarting)
		}

		go m.provisionSandbox(s, workDir, args)
		return s, nil
	}

	// Non-sandbox path: resolve command and create tmux session synchronously
	var resolvedCmd string
	resolvedCmd, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s", command)
	}

	tmuxName := TmuxSessionName(id)

	// Kill any existing tmux session with this name
	if tmuxSessionExists(tmuxName) {
		_ = tmuxKillSession(tmuxName)
	}

	// Create tmux session
	if err := tmuxCreateSession(tmuxName, workDir, resolvedCmd, args); err != nil {
		return nil, fmt.Errorf("creating tmux session: %w", err)
	}

	s := &Session{
		ID:          id,
		Name:        id,
		WorkDir:     workDir,
		State:       StateRunning,
		StartTime:   time.Now(),
		Owned:       true,
		TmuxSession: tmuxName,
		output:      NewRingBuf(int(m.bufferSize)),
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	if m.onStateChange != nil {
		m.onStateChange(s, StateCreated, StateRunning)
	}

	// Start reading output from tmux via a PTY attached to the session
	m.startReader(s)

	return s, nil
}

// provisionSandbox runs Docker sandbox setup asynchronously, then starts the tmux session.
func (m *Manager) provisionSandbox(s *Session, workDir string, args []string) {
	var sandboxName string

	existing, err := docker.FindSandboxForWorkDir(workDir)
	if err != nil {
		slog.Error("sandbox: failed to check existing", "id", s.ID, "error", err)
		m.failSession(s, fmt.Sprintf("checking sandbox: %v", err))
		return
	}
	if existing != nil {
		sandboxName = existing.Name
	} else {
		name, err := docker.SandboxCreate(workDir)
		if err != nil {
			slog.Error("sandbox: failed to create", "id", s.ID, "error", err)
			m.failSession(s, fmt.Sprintf("creating sandbox: %v", err))
			return
		}
		sandboxName = name
		if err := docker.SandboxCopyCredentials(sandboxName); err != nil {
			slog.Warn("sandbox: failed to copy credentials", "id", s.ID, "error", err)
		}
	}

	s.mu.Lock()
	s.SandboxName = sandboxName
	s.mu.Unlock()

	// Build the tmux command: docker sandbox run <name> -- <agent_args>
	fullArgs := append([]string{"sandbox", "run", sandboxName, "--"}, args...)
	tmuxName := TmuxSessionName(s.ID)

	if tmuxSessionExists(tmuxName) {
		_ = tmuxKillSession(tmuxName)
	}
	if err := tmuxCreateSession(tmuxName, workDir, "docker", fullArgs); err != nil {
		slog.Error("sandbox: failed to create tmux session", "id", s.ID, "error", err)
		m.failSession(s, fmt.Sprintf("creating tmux session: %v", err))
		return
	}

	s.mu.Lock()
	s.TmuxSession = tmuxName
	s.State = StateRunning
	s.mu.Unlock()

	if m.onStateChange != nil {
		m.onStateChange(s, StateStarting, StateRunning)
	}

	m.startReader(s)
}

// failSession transitions a session to errored state.
func (m *Manager) failSession(s *Session, errMsg string) {
	from := s.GetState()
	s.mu.Lock()
	s.State = StateErrored
	s.Error = errMsg
	s.EndTime = time.Now()
	s.mu.Unlock()
	if m.onStateChange != nil {
		m.onStateChange(s, from, StateErrored)
	}
}

// startReader attaches to the tmux session and reads output.
// Uses `tmux pipe-pane` to stream output, or attaches a reader PTY.
func (m *Manager) startReader(s *Session) {
	stop := make(chan struct{})
	m.mu.Lock()
	m.stopReaders[s.ID] = stop
	m.mu.Unlock()

	go func() {
		// Attach to the tmux session in read-write mode so that input
		// (including mouse events) can be forwarded through the PTY.
		// This enables pane selection in Agent Teams mode where Claude Code
		// creates multiple tmux panes for subagents.
		cmd := exec.Command("tmux", "attach-session", "-t", s.TmuxSession)
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 200})
		if err != nil {
			slog.Error("failed to attach to tmux session", "session", s.ID, "error", err)
			return
		}
		s.SetReaderPTY(ptmx)

		// Fast exit detection: tmux wait-for signals immediately when pane dies,
		// rather than waiting for the PTY EOF which has inherent delay.
		paneDone := make(chan struct{})
		waitCmd := exec.Command("tmux", "wait-for", s.TmuxSession+"-done")
		if waitErr := waitCmd.Start(); waitErr == nil {
			go func() {
				_ = waitCmd.Wait()
				close(paneDone)
			}()
		} else {
			// Fallback: if wait-for fails to start, paneDone never closes
			// and we rely on normal PTY EOF detection.
			slog.Debug("tmux wait-for failed to start, using PTY EOF detection", "error", waitErr)
		}

		defer func() {
			s.SetReaderPTY(nil)
			_ = ptmx.Close()
			_ = cmd.Process.Kill()
			// Clean up wait-for process if still running
			if waitCmd.Process != nil {
				_ = waitCmd.Process.Kill()
			}
		}()

		// Read PTY output in a separate goroutine so we can select on multiple channels.
		type readResult struct {
			buf []byte
			err error
		}
		readCh := make(chan readResult, 1)
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					data := make([]byte, n)
					copy(data, buf[:n])
					readCh <- readResult{buf: data}
				}
				if err != nil {
					readCh <- readResult{err: err}
					return
				}
			}
		}()

		for {
			select {
			case <-stop:
				return
			case <-paneDone:
				// Pane died — drain remaining buffered output then transition state.
				drainTimer := time.After(200 * time.Millisecond)
			drain:
				for {
					select {
					case res := <-readCh:
						if len(res.buf) > 0 {
							_, _ = s.Output().Write(res.buf)
							if m.onOutput != nil {
								m.onOutput(s.ID, res.buf)
							}
						}
						if res.err != nil {
							break drain
						}
					case <-drainTimer:
						break drain
					}
				}
				goto exited
			case res := <-readCh:
				if len(res.buf) > 0 {
					_, _ = s.Output().Write(res.buf)
					if m.onOutput != nil {
						m.onOutput(s.ID, res.buf)
					}
					m.checkWaitingState(s, res.buf)
				}
				if res.err != nil {
					goto exited
				}
			}
		}

	exited:
		// Reader stopped — check if we were explicitly stopped by Kill
		select {
		case <-stop:
			return // Kill handles state transition and cleanup
		default:
		}

		// Natural exit — check if tmux session is still alive
		if !tmuxSessionExists(s.TmuxSession) {
			from := s.GetState()
			s.mu.Lock()
			s.State = StateCompleted
			s.EndTime = time.Now()
			s.mu.Unlock()
			if m.onStateChange != nil {
				m.onStateChange(s, from, StateCompleted)
			}
			m.Remove(s.ID)
		}
	}()
}

// stopReader stops the output reader for a session.
func (m *Manager) stopReader(id string) {
	m.mu.Lock()
	if ch, ok := m.stopReaders[id]; ok {
		close(ch)
		delete(m.stopReaders, id)
	}
	m.mu.Unlock()
}

// Patterns that indicate Claude is waiting for user input.
var waitingPatterns = []string{
	"Do you want to proceed?",
	"[Y/n]", "[y/N]",
	"? (y/n)",
	"(yes/no)",
}

func (m *Manager) checkWaitingState(s *Session, data []byte) {
	line := string(data)
	currentState := s.GetState()

	if currentState == StateWaiting {
		if len(data) > 10 {
			from := currentState
			s.mu.Lock()
			s.State = StateRunning
			s.mu.Unlock()
			if m.onStateChange != nil {
				m.onStateChange(s, from, StateRunning)
			}
		}
		return
	}

	if currentState != StateRunning {
		return
	}

	for _, pattern := range waitingPatterns {
		if strings.Contains(line, pattern) {
			from := currentState
			s.mu.Lock()
			s.State = StateWaiting
			s.mu.Unlock()
			if m.onStateChange != nil {
				m.onStateChange(s, from, StateWaiting)
			}
			return
		}
	}
}

// Wait is kept for compatibility but is a no-op for tmux sessions.
// The reader goroutine handles state transitions.
func (m *Manager) Wait(id string) {
	// For tmux sessions, the reader goroutine handles completion detection.
	// This is kept for compatibility with the kill handler.
	time.Sleep(500 * time.Millisecond) // brief wait for tmux cleanup
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
	sort.SliceStable(result, func(i, j int) bool {
		pi, pj := statePriority(result[i].GetState()), statePriority(result[j].GetState())
		if pi != pj {
			return pi < pj
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func statePriority(s State) int {
	switch s {
	case StateRunning, StateWaiting:
		return 0
	case StateCreated:
		return 1
	case StateDiscovered:
		return 2
	case StateOffline:
		return 3
	case StateCompleted:
		return 4
	case StateErrored:
		return 5
	default:
		return 6
	}
}

// Kill kills a session's tmux session and removes it from the manager.
func (m *Manager) Kill(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	m.stopReader(id)
	if s.TmuxSession != "" {
		_ = tmuxKillSession(s.TmuxSession)
	} else if s.PID > 0 {
		// No tmux session (discovered/external) — kill the process directly.
		_ = syscall.Kill(s.PID, syscall.SIGTERM)
	}
	// Stop sandbox VM asynchronously to avoid blocking the UI
	if s.Sandboxed && s.SandboxName != "" {
		sandboxName := s.SandboxName
		go func() {
			if err := docker.SandboxStop(sandboxName); err != nil {
				slog.Warn("failed to stop sandbox", "name", sandboxName, "error", err)
			}
			if err := docker.SandboxRemove(sandboxName); err != nil {
				slog.Warn("failed to remove sandbox", "name", sandboxName, "error", err)
			}
		}()
	}
	// Set terminal state
	from := s.GetState()
	s.mu.Lock()
	s.EndTime = time.Now()
	if s.Killed {
		s.State = StateErrored // will be saved as "killed" by the handler
	} else {
		s.State = StateErrored
	}
	s.mu.Unlock()
	if m.onStateChange != nil {
		m.onStateChange(s, from, s.GetState())
	}
	// Track killed PID so discovery doesn't re-add it
	if s.PID > 0 {
		m.mu.Lock()
		m.killedPIDs[s.PID] = time.Now()
		m.mu.Unlock()
	}
	m.Remove(id)
	return nil
}

func (m *Manager) AddDiscovered(id, claudeID, workDir string, pid int, startTime time.Time) *Session {
	name := filepath.Base(workDir)
	if name == "" || name == "." {
		name = workDir
	}
	s := &Session{
		ID: id, ClaudeID: claudeID, Name: name, WorkDir: workDir,
		State: StateDiscovered, PID: pid, StartTime: startTime, Owned: false,
		output: NewRingBuf(int(m.bufferSize)),
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s
}

// AddOffline adds a session from a previous server run (loaded from SQLite).
func (m *Manager) AddOffline(id, name, claudeID, workDir, teamName, teamRole string) *Session {
	s := &Session{
		ID: id, ClaudeID: claudeID, Name: name, WorkDir: workDir,
		State: StateOffline, Owned: false,
		TeamName: teamName, TeamRole: teamRole,
		output: NewRingBuf(int(m.bufferSize)),
	}
	if name == "" {
		s.Name = filepath.Base(workDir)
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s
}

// Reattach reconnects to an existing tmux session (e.g., after server restart).
func (m *Manager) Reattach(id, name, claudeID, workDir, tmuxName string) *Session {
	s := &Session{
		ID:          id,
		ClaudeID:    claudeID,
		Name:        name,
		WorkDir:     workDir,
		State:       StateRunning,
		StartTime:   time.Now(),
		Owned:       true,
		TmuxSession: tmuxName,
		output:      NewRingBuf(int(m.bufferSize)),
	}
	if name == "" {
		s.Name = filepath.Base(workDir)
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	// Ensure tmux session has correct config (mouse, prefix, etc.)
	tmuxConfigureSession(tmuxName)

	// Start reading output
	m.startReader(s)

	return s
}

// Restart creates a new claude session in the same directory, replacing an offline session.
func (m *Manager) Restart(id string, opts ...*CreateOptions) (*Session, error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if s.GetState() != StateOffline {
		return nil, fmt.Errorf("session %s is not offline", id)
	}

	name := s.Name
	workDir := s.WorkDir
	claudeID := s.ClaudeID
	sandboxed := s.Sandboxed

	// Merge explicit opts with stored sandbox flag
	var opt *CreateOptions
	if len(opts) > 0 && opts[0] != nil {
		opt = opts[0]
	} else if sandboxed {
		opt = &CreateOptions{Sandboxed: true}
	}

	if claudeID == "" && workDir != "" {
		claudeID = discovery.ResolveClaudeSessionID(workDir)
	}

	m.Remove(id)

	args := []string{"--name", name}
	if claudeID != "" {
		args = append(args, "--resume", claudeID)
	}

	newSess, err := m.Create(id, workDir, "claude", args, opt)
	if err != nil {
		return nil, err
	}
	newSess.Name = name
	return newSess, nil
}

func (m *Manager) Remove(id string) {
	m.stopReader(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

// isTerminalResponse checks if data is a terminal response sequence
// that should be filtered out (e.g., DA responses from xterm.js).
func isTerminalResponse(data []byte) bool {
	s := string(data)
	// DA1 response: ESC [ ? ... c
	if strings.HasPrefix(s, "\033[?") && strings.HasSuffix(s, "c") {
		return true
	}
	// DA2 response: ESC [ > ... c
	if strings.HasPrefix(s, "\033[>") && strings.HasSuffix(s, "c") {
		return true
	}
	// Raw DA response without ESC (sometimes sent as plain text)
	if strings.HasPrefix(s, ">0;") && strings.HasSuffix(s, "c") {
		return true
	}
	// DSR response: ESC [ ... R (cursor position report)
	if strings.HasPrefix(s, "\033[") && strings.HasSuffix(s, "R") {
		// Check it's digits and semicolons between
		inner := s[2 : len(s)-1]
		for _, ch := range inner {
			if ch != ';' && (ch < '0' || ch > '9') {
				return false
			}
		}
		return true
	}
	return false
}

// WriteInput sends input to the session's tmux pane.
func (m *Manager) WriteInput(id string, data []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	// Filter out terminal capability responses from xterm.js
	if isTerminalResponse(data) {
		return nil
	}

	// Write directly to the bidirectional PTY master. This correctly
	// forwards all bytes including mouse events, which enables tmux pane
	// selection in Agent Teams mode (where Claude Code creates multiple
	// tmux panes for subagents). Without this, input only reaches the
	// last active pane, not the main pane where permissions are requested.
	if pty := s.GetReaderPTY(); pty != nil {
		_, err := pty.Write(data)
		return err
	}

	// Fallback to send-keys for sessions without a bidirectional PTY
	if s.TmuxSession == "" {
		return fmt.Errorf("session %s has no tmux session", id)
	}
	return tmuxSendKeys(s.TmuxSession, string(data))
}

// RecoverTmuxSessions finds existing ws-* tmux sessions and reattaches to them.
func (m *Manager) RecoverTmuxSessions() int {
	sessions, err := tmuxListSessions()
	if err != nil || len(sessions) == 0 {
		return 0
	}

	count := 0
	for _, tmuxName := range sessions {
		// Extract session ID from tmux name: "ws-myproject" -> "myproject"
		id := strings.TrimPrefix(tmuxName, tmuxPrefix)

		// Skip if already tracked
		if _, ok := m.Get(id); ok {
			continue
		}

		// Try to get the pane's current directory
		workDir, _ := tmuxRun("display-message", "-t", tmuxName, "-p", "#{pane_current_path}")

		name := filepath.Base(workDir)
		if name == "" || name == "." {
			name = id
		}

		claudeID := ""
		if workDir != "" {
			claudeID = discovery.ResolveClaudeSessionID(workDir)
		}

		m.Reattach(id, name, claudeID, workDir, tmuxName)
		slog.Info("reattached to tmux session", "id", id, "tmux", tmuxName, "dir", workDir)
		count++
	}
	return count
}
