package main

// DeterministicAgent wraps the existing Bot* functions behind the Agent interface.
// Used when no API key is configured.
type DeterministicAgent struct{}

func (DeterministicAgent) NightAction(g *Game, player Player) (*PlayerID, error) {
	switch player.Role {
	case RoleMafia:
		return BotPickKillTarget(g), nil
	case RoleDoctor:
		return BotPickProtectTarget(g, g.DoctorLastProtected), nil
	case RoleDetective:
		return BotPickInvestTarget(g, player.ID), nil
	default:
		return nil, nil
	}
}

func (DeterministicAgent) Discuss(g *Game, player Player, dayNumber int) (string, error) {
	return BotDayMessage(player, dayNumber), nil
}

func (DeterministicAgent) Vote(g *Game, player Player) (PlayerID, bool, error) {
	id, ok := BotPickVoteTarget(g, player)
	return id, ok, nil
}
