package main

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

func TestPickIdentitiesCount(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"pick 4", 4, 4},
		{"pick 9", 9, 9},
		{"pick more than pool", 100, len(identityPool)},
		{"pick 0", 0, 0},
	}

	rng := rand.New(rand.NewSource(42))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := pickIdentities(tt.n, rng.Shuffle)
			if len(ids) != tt.want {
				t.Fatalf("pickIdentities(%d) returned %d, want %d", tt.n, len(ids), tt.want)
			}
		})
	}
}

func TestPickIdentitiesNoDuplicates(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	ids := pickIdentities(len(identityPool), rng.Shuffle)

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id.Name] {
			t.Fatalf("duplicate identity: %s", id.Name)
		}
		seen[id.Name] = true
	}
}

func TestDeterministicAgentImplementsAgent(t *testing.T) {
	// Compile-time check that DeterministicAgent implements Agent.
	var _ Agent = DeterministicAgent{}
}

func TestDeterministicAgentNightAction(t *testing.T) {
	g := newTestGame(t, 6)

	tests := []struct {
		name    string
		role    Role
		wantNil bool
	}{
		{"mafia picks target", RoleMafia, false},
		{"doctor picks target", RoleDoctor, false},
		{"detective picks target", RoleDetective, false},
		{"villager returns nil", RoleVillager, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var player Player
			for _, p := range g.Players {
				if p.Role == tt.role {
					player = p
					break
				}
			}

			agent := DeterministicAgent{}
			result, err := agent.NightAction(g, player)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil && result != nil {
				t.Fatalf("expected nil, got %v", *result)
			}
			if !tt.wantNil && result == nil {
				t.Fatal("expected non-nil target")
			}
		})
	}
}

func TestDeterministicAgentDiscuss(t *testing.T) {
	g := newTestGame(t, 5)
	player := g.Players[1]
	agent := DeterministicAgent{}

	msg, err := agent.Discuss(g, player, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !strings.Contains(msg, player.Name) {
		t.Fatalf("message should contain player name %q, got %q", player.Name, msg)
	}
}

func TestDeterministicAgentVote(t *testing.T) {
	g := newTestGame(t, 5)
	agent := DeterministicAgent{}

	for _, p := range g.Players {
		if !p.Alive {
			continue
		}
		target, ok, err := agent.Vote(g, p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected vote for player %s", p.Name)
		}
		if target == p.ID {
			t.Fatalf("player %s voted for themselves", p.Name)
		}
	}
}

func TestParseTargetResponseValid(t *testing.T) {
	valid := []PlayerID{PlayerID(1), PlayerID(2), PlayerID(3)}

	tests := []struct {
		name    string
		json    string
		want    PlayerID
		wantErr bool
	}{
		{"valid target", `{"target_id": 2}`, PlayerID(2), false},
		{"first target", `{"target_id": 1}`, PlayerID(1), false},
		{"invalid target", `{"target_id": 99}`, 0, true},
		{"bad json", `not json`, 0, true},
		{"empty json", `{}`, 0, true},
		{"extra fields ok", `{"target_id": 3, "reasoning": "sus"}`, PlayerID(3), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTargetResponse(tt.json, valid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTargetResponse(%q) error=%v, wantErr=%v", tt.json, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("parseTargetResponse(%q) = %v, want %v", tt.json, got, tt.want)
			}
		})
	}
}

func TestSystemPromptContainsRole(t *testing.T) {
	g := newTestGame(t, 6)
	identity := AgentIdentity{Name: "TestBot", Trait: "very suspicious"}

	tests := []struct {
		role     Role
		contains []string
	}{
		{RoleMafia, []string{"Mafia", "TestBot", "very suspicious"}},
		{RoleDoctor, []string{"Doctor", "TestBot", "very suspicious"}},
		{RoleDetective, []string{"Detective", "TestBot", "very suspicious"}},
		{RoleVillager, []string{"Villager", "TestBot", "very suspicious"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			var player Player
			for _, p := range g.Players {
				if p.Role == tt.role {
					player = p
					break
				}
			}

			prompt := systemPrompt(player, identity, g)
			for _, s := range tt.contains {
				if !strings.Contains(prompt, s) {
					t.Fatalf("system prompt missing %q for role %s", s, tt.role)
				}
			}
		})
	}
}

func TestGameContextListsPlayers(t *testing.T) {
	g := newTestGame(t, 5)
	ctx := gameContext(g)

	for _, p := range g.Players {
		if !strings.Contains(ctx, p.Name) {
			t.Fatalf("game context missing player %q", p.Name)
		}
	}
	if !strings.Contains(ctx, "alive") {
		t.Fatal("game context should mention alive status")
	}
}

func TestMakeAgentFactoryNilClient(t *testing.T) {
	factory := makeAgentFactory(nil)
	if factory != nil {
		t.Fatal("expected nil factory when client is nil")
	}
}

func TestNewGameAssignsAgentNames(t *testing.T) {
	g, err := NewGame(6, "", rand.New(rand.NewSource(42)), nil)
	if err != nil {
		t.Fatalf("NewGame error: %v", err)
	}

	// Human should be "You"
	if g.Players[0].Name != "You" {
		t.Fatalf("player 0 name=%q, want 'You'", g.Players[0].Name)
	}

	// Bots should have identity names (not "Player N")
	for i := 1; i < len(g.Players); i++ {
		p := g.Players[i]
		if strings.HasPrefix(p.Name, "Player ") {
			t.Fatalf("bot player %d still has generic name %q", i, p.Name)
		}
		if p.Agent == nil {
			t.Fatalf("bot player %d has nil agent", i)
		}
	}
}

func TestNewGameWithNilAgentFactoryDefaultsToDeterministic(t *testing.T) {
	g, err := NewGame(5, "", rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatalf("NewGame error: %v", err)
	}

	for i := 1; i < len(g.Players); i++ {
		if _, ok := g.Players[i].Agent.(DeterministicAgent); !ok {
			t.Fatalf("player %d agent type=%T, want DeterministicAgent", i, g.Players[i].Agent)
		}
	}
}

func TestNightActionTargets(t *testing.T) {
	g := newTestGame(t, 6)

	t.Run("mafia targets exclude mafia", func(t *testing.T) {
		var mafia Player
		for _, p := range g.Players {
			if p.Role == RoleMafia {
				mafia = p
				break
			}
		}
		targets := g.NightActionTargets(mafia)
		for _, tid := range targets {
			tp, _ := g.FindPlayer(tid)
			if tp.Role == RoleMafia {
				t.Fatalf("mafia target %d is also mafia", tid)
			}
		}
	})

	t.Run("doctor respects last protected", func(t *testing.T) {
		var doctor Player
		for _, p := range g.Players {
			if p.Role == RoleDoctor {
				doctor = p
				break
			}
		}
		lastProtected := g.Players[0].ID
		g.DoctorLastProtected = &lastProtected

		targets := g.NightActionTargets(doctor)
		for _, tid := range targets {
			if tid == lastProtected {
				t.Fatalf("doctor targets include last protected player %d", lastProtected)
			}
		}
		g.DoctorLastProtected = nil // cleanup
	})

	t.Run("detective excludes self", func(t *testing.T) {
		var detective Player
		for _, p := range g.Players {
			if p.Role == RoleDetective {
				detective = p
				break
			}
		}
		targets := g.NightActionTargets(detective)
		for _, tid := range targets {
			if tid == detective.ID {
				t.Fatalf("detective targets include self")
			}
		}
	})
}

func TestBuildMessagesOrdering(t *testing.T) {
	history := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("prev question"),
		openai.AssistantMessage("prev answer"),
	}

	msgs := buildMessages(history, "system text", "game state text", "user prompt text")

	// Expected: system, game state (user), history user, history assistant, user prompt
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	// Verify message types via union fields
	if msgs[0].OfSystem == nil {
		t.Fatal("message 0 should be system")
	}
	if msgs[1].OfUser == nil {
		t.Fatal("message 1 should be user (game state)")
	}
	if msgs[2].OfUser == nil {
		t.Fatal("message 2 should be user (history)")
	}
	if msgs[3].OfAssistant == nil {
		t.Fatal("message 3 should be assistant (history)")
	}
	if msgs[4].OfUser == nil {
		t.Fatal("message 4 should be user (prompt)")
	}
}

func TestVoteTargetsExcludeSelf(t *testing.T) {
	g := newTestGame(t, 5)
	for _, p := range g.Players {
		targets := g.VoteTargets(p)
		for _, tid := range targets {
			if tid == p.ID {
				t.Fatalf("vote targets for %s include self", p.Name)
			}
		}
	}
}
