package main

import "fmt"

type RoundResolver interface {
	ResolveNight(*Game) (*PlayerID, error)
	ResolveVote(*Game) (*PlayerID, error)
}

func (g *Game) FindPlayer(id PlayerID) (*Player, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}

	for i := range g.Players {
		if g.Players[i].ID == id {
			return &g.Players[i], nil
		}
	}

	return nil, fmt.Errorf("player %v not found", id)
}

func (g *Game) AlivePlayerIDs() []PlayerID {
	if g == nil {
		return nil
	}

	alive := make([]PlayerID, 0, len(g.Players))
	for i := range g.Players {
		if g.Players[i].Alive {
			alive = append(alive, g.Players[i].ID)
		}
	}

	return alive
}

// events returns the game's event log, or nil if not set.
func (g *Game) events() []string {
	if g.EventLog == nil {
		return nil
	}
	return *g.EventLog
}

// NightActionTargets returns the valid target IDs for a player's night role action.
func (g *Game) NightActionTargets(player Player) []PlayerID {
	var targets []PlayerID
	switch player.Role {
	case RoleMafia:
		for _, p := range g.Players {
			if p.Alive && p.Role != RoleMafia {
				targets = append(targets, p.ID)
			}
		}
	case RoleDoctor:
		for _, p := range g.Players {
			if !p.Alive {
				continue
			}
			if g.DoctorLastProtected != nil && p.ID == *g.DoctorLastProtected {
				continue
			}
			targets = append(targets, p.ID)
		}
	case RoleDetective:
		for _, p := range g.Players {
			if p.Alive && p.ID != player.ID {
				targets = append(targets, p.ID)
			}
		}
	}
	return targets
}

// VoteTargets returns valid vote target IDs (alive, not self).
func (g *Game) VoteTargets(player Player) []PlayerID {
	var targets []PlayerID
	for _, p := range g.Players {
		if p.Alive && p.ID != player.ID {
			targets = append(targets, p.ID)
		}
	}
	return targets
}

// HumanPlayer returns the human player (always exists, may be dead).
func (g *Game) HumanPlayer() *Player {
	for i := range g.Players {
		if g.Players[i].IsHuman {
			return &g.Players[i]
		}
	}
	return nil
}

// FindByRole returns the first alive player with the given role, or nil.
func (g *Game) FindByRole(role Role) *Player {
	for i := range g.Players {
		if g.Players[i].Alive && g.Players[i].Role == role {
			return &g.Players[i]
		}
	}
	return nil
}



func (g *Game) Eliminate(id PlayerID, cause EliminationCause) error {
	_ = cause

	if g == nil {
		return fmt.Errorf("game is nil")
	}

	player, err := g.FindPlayer(id)
	if err != nil {
		return err
	}
	if !player.Alive {
		return fmt.Errorf("player %v is already dead", id)
	}

	player.Alive = false
	player.RoleRevealed = true
	eliminatedID := player.ID
	g.LastEliminated = &eliminatedID

	return nil
}

func (g *Game) Start() error {
	if g == nil {
		return fmt.Errorf("game is nil")
	}

	if g.Phase != PhaseSetup {
		return fmt.Errorf("cannot start game from phase %v", g.Phase)
	}

	g.DayNumber++
	g.Phase = PhaseNight
	return nil
}

func (g *Game) AdvancePhase() error {
	if g == nil {
		return fmt.Errorf("game is nil")
	}

	switch g.Phase {
	case PhaseSetup:
		g.DayNumber++
		g.Phase = PhaseNight
	case PhaseNight:
		g.Phase = PhaseDay
	case PhaseDay:
		g.Phase = PhaseVote
	case PhaseVote:
		g.Phase = PhaseWinCheck
	case PhaseWinCheck:
		winner, won := g.CheckWin()
		if won {
			g.Winner = winner
			g.Phase = PhaseEnded
		} else {
			g.DayNumber++
			g.Phase = PhaseNight
		}
	case PhaseEnded:
		return fmt.Errorf("game already ended")
	default:
		return fmt.Errorf("unknown phase %v", g.Phase)
	}

	return nil
}

func (g *Game) RunUntilEnd(resolver RoundResolver, maxCycles int) (Winner, error) {
	if g == nil {
		return WinnerNone, fmt.Errorf("game is nil")
	}

	if resolver == nil {
		return g.Winner, fmt.Errorf("resolver cannot be nil")
	}
	if maxCycles <= 0 {
		return g.Winner, fmt.Errorf("maxCycles must be positive")
	}

	for cycle := 0; cycle < maxCycles; cycle++ {
		if g.Phase == PhaseEnded {
			return g.Winner, nil
		}

		switch g.Phase {
		case PhaseNight:
			targetID, err := resolver.ResolveNight(g)
			if err != nil {
				return g.Winner, err
			}
			if targetID != nil {
				if err := g.Eliminate(*targetID, CauseNightKill); err != nil {
					return g.Winner, err
				}
				winner, won := g.CheckWin()
				if won {
					g.Winner = winner
					g.Phase = PhaseEnded
					return winner, nil
				}
			}
		case PhaseVote:
			targetID, err := resolver.ResolveVote(g)
			if err != nil {
				return g.Winner, err
			}
			if targetID != nil {
				if err := g.Eliminate(*targetID, CauseVote); err != nil {
					return g.Winner, err
				}
				winner, won := g.CheckWin()
				if won {
					g.Winner = winner
					g.Phase = PhaseEnded
					return winner, nil
				}
			}
		}

		if err := g.AdvancePhase(); err != nil {
			return g.Winner, err
		}
		if g.Phase == PhaseEnded {
			return g.Winner, nil
		}
	}

	return g.Winner, fmt.Errorf("maxCycles exceeded")
}
