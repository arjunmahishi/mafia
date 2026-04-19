package main

// Agent represents a bot player's decision-making capability.
// Implemented by DeterministicAgent and AIAgent.
type Agent interface {
	// NightAction returns the target PlayerID for this agent's night role action.
	// For Mafia: kill target. For Doctor: protect target. For Detective: investigate target.
	NightAction(g *Game, player Player) (*PlayerID, error)

	// Discuss returns the agent's day-phase discussion message.
	Discuss(g *Game, player Player, dayNumber int) (string, error)

	// Vote returns the PlayerID this agent votes to eliminate and true,
	// or zero value and false if no valid target exists.
	Vote(g *Game, player Player) (PlayerID, bool, error)
}
