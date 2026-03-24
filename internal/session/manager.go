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
	"time"

	"github.com/creack/pty"
	"github.com/igor-deoalves/websessions/internal/discovery"
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
}

func NewManager(bufferSize int64) *Manager {
	return &Manager{
		sessions:    make(map[string]*Session),
		bufferSize:  bufferSize,
		stopReaders: make(map[string]chan struct{}),
	}
}

func (m *Manager) OnStateChange(fn StateChangeFunc) { m.onStateChange = fn }
func (m *Manager) OnOutput(fn OutputFunc)            { m.onOutput = fn }

// Create creates a new session inside a tmux session.
func (m *Manager) Create(id, workDir, command string, args []string) (*Session, error) {
	// Expand ~ in workDir
	if len(workDir) > 0 && workDir[0] == '~' {
		home, _ := os.UserHomeDir()
		workDir = home + workDir[1:]
	}

	// Validate directory exists
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("working directory does not exist: %s", workDir)
	}

	// Resolve command path (handles symlinks)
	resolvedCmd, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s", command)
	}

	tmuxName := TmuxSessionName(id)

	// Kill any existing tmux session with this name
	if tmuxSessionExists(tmuxName) {
		tmuxKillSession(tmuxName)
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

// startReader attaches to the tmux session and reads output.
// Uses `tmux pipe-pane` to stream output, or attaches a reader PTY.
func (m *Manager) startReader(s *Session) {
	stop := make(chan struct{})
	m.mu.Lock()
	m.stopReaders[s.ID] = stop
	m.mu.Unlock()

	go func() {
		// Attach to the tmux session in read-only mode.
		// Set a large initial PTY so tmux doesn't constrain the window.
		cmd := exec.Command("tmux", "attach-session", "-t", s.TmuxSession, "-r")
		ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 200})
		if err != nil {
			slog.Error("failed to attach to tmux session", "session", s.ID, "error", err)
			return
		}
		s.SetReaderPTY(ptmx)
		defer func() {
			s.SetReaderPTY(nil)
			ptmx.Close()
			cmd.Process.Kill()
		}()

		buf := make([]byte, 4096)
		for {
			select {
			case <-stop:
				return
			default:
			}

			n, err := ptmx.Read(buf)
			if n > 0 {
				s.Output().Write(buf[:n])
				if m.onOutput != nil {
					m.onOutput(s.ID, buf[:n])
				}
				m.checkWaitingState(s, buf[:n])
			}
			if err != nil {
				break
			}
		}

		// Reader stopped — check if tmux session is still alive
		if !tmuxSessionExists(s.TmuxSession) {
			from := s.GetState()
			if s.Killed {
				return
			}
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
	sort.Slice(result, func(i, j int) bool {
		pi, pj := statePriority(result[i].GetState()), statePriority(result[j].GetState())
		if pi != pj {
			return pi < pj
		}
		return result[i].Name < result[j].Name
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
		tmuxKillSession(s.TmuxSession)
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
func (m *Manager) AddOffline(id, name, claudeID, workDir string) *Session {
	s := &Session{
		ID: id, ClaudeID: claudeID, Name: name, WorkDir: workDir,
		State: StateOffline, Owned: false,
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

	// Start reading output
	m.startReader(s)

	return s
}

// Restart creates a new claude session in the same directory, replacing an offline session.
func (m *Manager) Restart(id string) (*Session, error) {
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

	if claudeID == "" && workDir != "" {
		claudeID = discovery.ResolveClaudeSessionID(workDir)
	}

	m.Remove(id)

	args := []string{"--name", name}
	if claudeID != "" {
		args = append(args, "--resume", claudeID)
	}

	newSess, err := m.Create(id, workDir, "claude", args)
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

// WriteInput sends input to the session's tmux pane.
func (m *Manager) WriteInput(id string, data []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
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
