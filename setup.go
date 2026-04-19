package main

import (
	"fmt"
	"math/rand"
	"time"
)

// NewAgentFunc creates an Agent for a bot player given its role and identity.
// Returning nil means the player gets no agent (only valid for humans).
type NewAgentFunc func(role Role, identity AgentIdentity) Agent

func NewGame(totalPlayers int, humanName string, rng *rand.Rand, newAgent NewAgentFunc) (*Game, error) {
	if err := ValidatePlayerCount(totalPlayers); err != nil {
		return nil, err
	}

	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	// Default to deterministic agents if no factory provided.
	if newAgent == nil {
		newAgent = func(role Role, identity AgentIdentity) Agent {
			return DeterministicAgent{}
		}
	}

	// Pick identities for bot players (totalPlayers - 1 bots).
	identities := pickIdentities(totalPlayers-1, rng.Shuffle)

	if humanName == "" {
		humanName = "You"
	}

	players := make([]Player, totalPlayers)
	players[0] = Player{
		ID:           PlayerID(1),
		Name:         humanName,
		IsHuman:      true,
		Alive:        true,
		RoleRevealed: false,
	}

	for i := 1; i < totalPlayers; i++ {
		identity := identities[i-1]
		players[i] = Player{
			ID:           PlayerID(i + 1),
			Name:         identity.Name,
			IsHuman:      false,
			Alive:        true,
			RoleRevealed: false,
		}
	}

	if err := AssignRoles(players, rng); err != nil {
		return nil, err
	}

	// Assign agents to bot players now that roles are known.
	for i := 1; i < totalPlayers; i++ {
		players[i].Agent = newAgent(players[i].Role, identities[i-1])
	}

	return &Game{
		Players:   players,
		Phase:     PhaseSetup,
		DayNumber: 0,
		Winner:    WinnerNone,
		RNG:       rng,
	}, nil
}

func ValidatePlayerCount(totalPlayers int) error {
	if totalPlayers < 5 || totalPlayers > 10 {
		return fmt.Errorf("invalid total players: %d (must be between 5 and 10)", totalPlayers)
	}

	return nil
}

func RoleCounts(totalPlayers int) (mafia int, doctor int, detective int, villager int) {
	doctor = 1
	detective = 1
	mafia = totalPlayers / 4
	if mafia < 1 {
		mafia = 1
	}
	villager = totalPlayers - mafia - doctor - detective

	return mafia, doctor, detective, villager
}

func AssignRoles(players []Player, rng *rand.Rand) error {
	if rng == nil {
		return fmt.Errorf("rng cannot be nil")
	}

	totalPlayers := len(players)
	if err := ValidatePlayerCount(totalPlayers); err != nil {
		return err
	}

	mafiaCount, doctorCount, detectiveCount, villagerCount := RoleCounts(totalPlayers)
	if villagerCount < 0 {
		return fmt.Errorf("invalid role counts for %d players", totalPlayers)
	}

	roles := make([]Role, 0, totalPlayers)
	for i := 0; i < mafiaCount; i++ {
		roles = append(roles, RoleMafia)
	}
	for i := 0; i < doctorCount; i++ {
		roles = append(roles, RoleDoctor)
	}
	for i := 0; i < detectiveCount; i++ {
		roles = append(roles, RoleDetective)
	}
	for i := 0; i < villagerCount; i++ {
		roles = append(roles, RoleVillager)
	}

	if len(roles) != totalPlayers {
		return fmt.Errorf("role assignment mismatch: roles=%d players=%d", len(roles), totalPlayers)
	}

	rng.Shuffle(len(roles), func(i, j int) {
		roles[i], roles[j] = roles[j], roles[i]
	})

	for i := range players {
		players[i].Role = roles[i]
	}

	return nil
}
