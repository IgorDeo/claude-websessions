package session

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type StateChangeFunc func(s *Session, from, to State)
type OutputFunc func(sessionID string, data []byte)

type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	bufferSize    int64
	onStateChange StateChangeFunc
	onOutput      OutputFunc
}

func NewManager(bufferSize int64) *Manager {
	return &Manager{
		sessions:   make(map[string]*Session),
		bufferSize: bufferSize,
	}
}

func (m *Manager) OnStateChange(fn StateChangeFunc) { m.onStateChange = fn }
func (m *Manager) OnOutput(fn OutputFunc)            { m.onOutput = fn }

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

	s := &Session{
		ID: id, Name: id, WorkDir: workDir,
		State: StateCreated, StartTime: time.Now(), Owned: true,
		output: NewRingBuf(int(m.bufferSize)),
	}
	cmd := exec.Command(resolvedCmd, args...)
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
	go m.readPTY(s)
	return s, nil
}

var waitingPatterns = []string{
	"Allow", "Deny",
	"Do you want to",
	"[Y/n]", "[y/N]",
	"? (y/n)",
}

func (m *Manager) checkWaitingState(s *Session, data []byte) {
	line := string(data)
	currentState := s.GetState()

	if currentState == StateWaiting {
		// Any new substantial output means we're no longer waiting
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

func (m *Manager) readPTY(s *Session) {
	buf := make([]byte, 4096)
	for {
		n, err := s.PTY().Read(buf)
		if n > 0 {
			s.Output().Write(buf[:n])
			if m.onOutput != nil {
				m.onOutput(s.ID, buf[:n])
			}
			m.checkWaitingState(s, buf[:n])
		}
		if err != nil {
			if err != io.EOF {
				slog.Debug("PTY read error", "session", s.ID, "error", err)
			}
			break
		}
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
	// Sort: running/waiting first, then discovered, then offline, then completed/errored.
	// Within each group, sort by name.
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
	return proc.Signal(os.Kill)
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

	m.Remove(id)

	args := []string{}
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
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

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
