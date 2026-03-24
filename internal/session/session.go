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
	StateOffline    State = "offline"
)

type Session struct {
	mu sync.RWMutex

	ID        string
	ClaudeID  string
	Name      string
	WorkDir   string
	State     State
	PID       int
	StartTime time.Time
	EndTime   time.Time
	ExitCode  int
	Error     string
	Owned     bool
	Killed    bool // true if intentionally killed by user — suppresses error notification

	pty    *os.File
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

func (s *Session) SetPTY(ptmx *os.File, proc *os.Process) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pty = ptmx
	s.proc = proc
	if proc != nil {
		s.PID = proc.Pid
	}
}

func (s *Session) Resize(rows, cols uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pty == nil {
		return fmt.Errorf("no PTY attached")
	}
	return pty.Setsize(s.pty, &pty.Winsize{Rows: rows, Cols: cols})
}
