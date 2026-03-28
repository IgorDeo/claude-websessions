package teams

import "time"

// TeamState represents the current state of an agent team.
type TeamState string

const (
	TeamActive   TeamState = "active"
	TeamInactive TeamState = "inactive"
)

// MemberRole distinguishes the team lead from teammates.
type MemberRole string

const (
	RoleLead     MemberRole = "lead"
	RoleTeammate MemberRole = "teammate"
)

// TaskState represents the lifecycle state of a team task.
type TaskState string

const (
	TaskPending    TaskState = "pending"
	TaskInProgress TaskState = "in-progress"
	TaskCompleted  TaskState = "completed"
)

// Team represents a discovered Claude Code agent team.
type Team struct {
	Name       string
	State      TeamState
	LeadID     string // agentID of the lead
	Members    []Member
	Tasks      []Task
	Messages   []Message
	ConfigPath string
}

// Member represents one agent in an agent team.
type Member struct {
	Name      string
	AgentID   string
	AgentType string
	Role      MemberRole
	SessionID string // websessions session ID (empty if not yet mapped)
	Connected bool
}

// Task represents a work item in the shared task list.
type Task struct {
	ID          string
	Title       string
	Description string
	State       TaskState
	AssignedTo  string
	DependsOn   []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Message represents an inter-agent message.
type Message struct {
	From      string
	To        string // empty means broadcast
	Content   string
	Timestamp time.Time
}

// TeamConfig mirrors the on-disk config.json structure at ~/.claude/teams/{name}/config.json.
type TeamConfig struct {
	Name    string             `json:"name"`
	Members []TeamConfigMember `json:"members"`
}

// TeamConfigMember is a single member entry in the team config file.
type TeamConfigMember struct {
	Name      string `json:"name"`
	AgentID   string `json:"agentId"`
	AgentType string `json:"agentType"`
}
