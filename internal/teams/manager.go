package teams

import (
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/session"
)

// Manager tracks discovered agent teams and correlates members with sessions.
type Manager struct {
	mu      sync.RWMutex
	teams   map[string]*Team
	sessMgr *session.Manager
	bus     *notification.Bus

	// seenMessages tracks message timestamps to avoid re-publishing duplicates.
	seenMessages map[string]bool
}

// NewManager creates a team manager that correlates team members with sessions.
func NewManager(sessMgr *session.Manager, bus *notification.Bus) *Manager {
	return &Manager{
		teams:        make(map[string]*Team),
		sessMgr:      sessMgr,
		bus:          bus,
		seenMessages: make(map[string]bool),
	}
}

// Scan rescans the filesystem for agent teams and updates internal state.
func (m *Manager) Scan() error {
	configs, err := ScanTeams()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[string]bool)
	for _, cfg := range configs {
		seen[cfg.Name] = true

		existing, exists := m.teams[cfg.Name]
		if !exists {
			team := m.buildTeam(cfg)
			m.teams[cfg.Name] = team
			if m.bus != nil {
				m.bus.Publish(notification.SessionEvent{
					Type:      notification.EventTeamDiscovered,
					TeamName:  cfg.Name,
					Timestamp: time.Now(),
					Message:   "Agent team discovered: " + cfg.Name,
				})
			}
			slog.Info("discovered agent team", "name", cfg.Name, "members", len(cfg.Members))
		} else {
			m.updateTeam(existing, cfg)
		}
	}

	// Mark teams that disappeared as inactive
	for name, team := range m.teams {
		if !seen[name] && team.State == TeamActive {
			team.State = TeamInactive
		}
	}

	// Scan tasks and messages for each active team
	for _, team := range m.teams {
		if team.State != TeamActive {
			continue
		}
		m.scanTeamTasks(team)
		m.scanTeamMessages(team)
	}

	return nil
}

// buildTeam constructs a Team from a discovered TeamConfig.
func (m *Manager) buildTeam(cfg TeamConfig) *Team {
	team := &Team{
		Name:  cfg.Name,
		State: TeamActive,
	}
	for _, mc := range cfg.Members {
		role := RoleTeammate
		if mc.AgentType == "lead" {
			role = RoleLead
			team.LeadID = mc.AgentID
		}
		member := Member{
			Name:      mc.Name,
			AgentID:   mc.AgentID,
			AgentType: mc.AgentType,
			Role:      role,
		}
		m.correlateMemberSession(&member)
		team.Members = append(team.Members, member)
	}
	return team
}

// updateTeam refreshes an existing team from a re-scanned config.
func (m *Manager) updateTeam(team *Team, cfg TeamConfig) {
	team.State = TeamActive
	// Rebuild member list to pick up new members or dropped ones
	newMembers := make([]Member, 0, len(cfg.Members))
	for _, mc := range cfg.Members {
		role := RoleTeammate
		if mc.AgentType == "lead" {
			role = RoleLead
			team.LeadID = mc.AgentID
		}
		member := Member{
			Name:      mc.Name,
			AgentID:   mc.AgentID,
			AgentType: mc.AgentType,
			Role:      role,
		}
		// Preserve existing session mapping if agent ID matches
		for _, old := range team.Members {
			if old.AgentID == mc.AgentID {
				member.SessionID = old.SessionID
				break
			}
		}
		m.correlateMemberSession(&member)
		newMembers = append(newMembers, member)
	}
	team.Members = newMembers
}

// correlateMemberSession tries to match a team member to a websessions session
// by comparing the member's AgentID against session ClaudeIDs.
func (m *Manager) correlateMemberSession(member *Member) {
	if m.sessMgr == nil || member.AgentID == "" {
		return
	}
	for _, s := range m.sessMgr.List() {
		if s.ClaudeID == member.AgentID {
			member.SessionID = s.ID
			member.Connected = true
			s.TeamName = member.Name
			s.TeamRole = string(member.Role)
			return
		}
	}
	member.Connected = false
}

// scanTeamTasks reads tasks for the team's lead session from disk.
func (m *Manager) scanTeamTasks(team *Team) {
	if team.LeadID == "" {
		return
	}
	tasks, err := ScanTasks(team.LeadID)
	if err != nil {
		slog.Debug("failed to scan team tasks", "team", team.Name, "error", err)
		return
	}

	// Detect new completions
	oldCompleted := make(map[string]bool)
	for _, t := range team.Tasks {
		if t.State == TaskCompleted {
			oldCompleted[t.ID] = true
		}
	}

	team.Tasks = tasks

	if m.bus != nil {
		for _, t := range tasks {
			if t.State == TaskCompleted && !oldCompleted[t.ID] {
				m.bus.Publish(notification.SessionEvent{
					Type:      notification.EventTaskCompleted,
					TeamName:  team.Name,
					Timestamp: time.Now(),
					Message:   "Task completed: " + t.Title,
					Metadata:  map[string]string{"task_id": t.ID, "assigned_to": t.AssignedTo},
				})
			}
		}
	}
}

// scanTeamMessages reads new messages from the team's mailbox.
func (m *Manager) scanTeamMessages(team *Team) {
	messages, err := ScanMailbox(team.Name)
	if err != nil {
		slog.Debug("failed to scan team messages", "team", team.Name, "error", err)
		return
	}

	for _, msg := range messages {
		key := msg.From + "|" + msg.To + "|" + msg.Timestamp.String()
		if m.seenMessages[key] {
			continue
		}
		m.seenMessages[key] = true
		team.Messages = append(team.Messages, msg)
		if m.bus != nil {
			m.bus.Publish(notification.SessionEvent{
				Type:      notification.EventTeamMessage,
				TeamName:  team.Name,
				Timestamp: msg.Timestamp,
				Message:   msg.From + " → " + msg.To + ": " + msg.Content,
				Metadata:  map[string]string{"from": msg.From, "to": msg.To},
			})
		}
	}
}

// CheckClaudeVersion verifies that Claude Code is installed and meets the
// minimum version requirement for agent teams (v2.1.32+).
// Returns the version string and whether it meets the requirement.
func CheckClaudeVersion() (version string, supported bool) {
	out, err := exec.Command("claude", "--version").Output()
	if err != nil {
		return "", false
	}
	ver := strings.TrimSpace(string(out))
	// Version format varies; check if it contains a version number
	// Agent teams require v2.1.32+, but we do a best-effort check
	if ver == "" {
		return "", false
	}
	// Parse major.minor.patch from the version string
	parts := strings.Fields(ver)
	for _, p := range parts {
		if isVersionString(p) {
			return p, compareVersion(p, "2.1.32") >= 0
		}
	}
	return ver, true // assume compatible if we can't parse
}

func isVersionString(s string) bool {
	for _, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return strings.Contains(s, ".")
}

func compareVersion(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		ai, bi := 0, 0
		for _, c := range aParts[i] {
			ai = ai*10 + int(c-'0')
		}
		for _, c := range bParts[i] {
			bi = bi*10 + int(c-'0')
		}
		if ai != bi {
			if ai > bi {
				return 1
			}
			return -1
		}
	}
	return len(aParts) - len(bParts)
}

// Get returns a team by name.
func (m *Manager) Get(name string) (*Team, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.teams[name]
	return t, ok
}

// List returns all known teams.
func (m *Manager) List() []*Team {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Team, 0, len(m.teams))
	for _, t := range m.teams {
		result = append(result, t)
	}
	return result
}
