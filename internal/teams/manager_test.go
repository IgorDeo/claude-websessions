package teams

import (
	"testing"

	"github.com/IgorDeo/claude-websessions/internal/notification"
)

func TestManager_NewAndList(t *testing.T) {
	bus := notification.NewBus()
	mgr := NewManager(nil, bus)

	teams := mgr.List()
	if len(teams) != 0 {
		t.Fatalf("expected 0 teams, got %d", len(teams))
	}
}

func TestManager_BuildTeam(t *testing.T) {
	bus := notification.NewBus()
	mgr := NewManager(nil, bus)

	cfg := TeamConfig{
		Name: "test-team",
		Members: []TeamConfigMember{
			{Name: "architect", AgentID: "agent-1", AgentType: "lead"},
			{Name: "frontend", AgentID: "agent-2", AgentType: "teammate"},
		},
	}

	team := mgr.buildTeam(cfg)
	if team.Name != "test-team" {
		t.Errorf("expected name 'test-team', got %q", team.Name)
	}
	if team.State != TeamActive {
		t.Errorf("expected state 'active', got %q", team.State)
	}
	if team.LeadID != "agent-1" {
		t.Errorf("expected lead ID 'agent-1', got %q", team.LeadID)
	}
	if len(team.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(team.Members))
	}
	if team.Members[0].Role != RoleLead {
		t.Errorf("expected first member to be lead, got %q", team.Members[0].Role)
	}
	if team.Members[1].Role != RoleTeammate {
		t.Errorf("expected second member to be teammate, got %q", team.Members[1].Role)
	}
}

func TestManager_GetNotFound(t *testing.T) {
	bus := notification.NewBus()
	mgr := NewManager(nil, bus)

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}
