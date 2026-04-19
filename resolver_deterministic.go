package main

import "fmt"

// --- Single-turn bot decision helpers (used by the step-based game driver) ---

// BotPickKillTarget picks the first alive non-mafia player (deterministic).
func BotPickKillTarget(g *Game) *PlayerID {
	for i := range g.Players {
		p := g.Players[i]
		if p.Alive && p.Role != RoleMafia {
			id := p.ID
			return &id
		}
	}
	return nil
}

// BotPickProtectTarget picks the first alive player that is not the same as
// lastProtected (deterministic). The doctor may protect themselves.
func BotPickProtectTarget(g *Game, lastProtected *PlayerID) *PlayerID {
	for i := range g.Players {
		p := g.Players[i]
		if !p.Alive {
			continue
		}
		if lastProtected != nil && p.ID == *lastProtected {
			continue
		}
		id := p.ID
		return &id
	}
	return nil
}

// BotPickInvestTarget picks the first alive player that is not the detective (deterministic).
func BotPickInvestTarget(g *Game, detectiveID PlayerID) *PlayerID {
	for i := range g.Players {
		p := g.Players[i]
		if p.Alive && p.ID != detectiveID {
			id := p.ID
			return &id
		}
	}
	return nil
}

// BotPickVoteTarget picks a vote target for a bot voter (deterministic).
// Mafia votes for first alive non-mafia; others vote for first alive mafia;
// fallback to first alive non-self.
func BotPickVoteTarget(g *Game, voter Player) (PlayerID, bool) {
	return deterministicVoteTarget(g, voter)
}

// BotDayMessage returns a stub discussion message for a bot player.
func BotDayMessage(player Player, dayNumber int) string {
	return fmt.Sprintf("[%s thinks carefully about who might be mafia...]", player.Name)
}

// --- Legacy whole-round resolvers (kept for existing tests/M2 compat) ---

type DeterministicResolver struct{}

func (DeterministicResolver) ResolveNight(g *Game) (*PlayerID, error) {
	return BotPickKillTarget(g), nil
}

func (DeterministicResolver) ResolveVote(g *Game) (*PlayerID, error) {
	votes := make(map[PlayerID]int)
	aliveCount := 0

	for i := range g.Players {
		if !g.Players[i].Alive {
			continue
		}

		aliveCount++
		target, ok := deterministicVoteTarget(g, g.Players[i])
		if !ok {
			continue
		}
		votes[target]++
	}

	if aliveCount <= 1 || len(votes) == 0 {
		return nil, nil
	}

	var winner PlayerID
	maxVotes := 0
	tie := false

	for i := range g.Players {
		id := g.Players[i].ID
		count := votes[id]
		if count == 0 {
			continue
		}
		if count > maxVotes {
			winner = id
			maxVotes = count
			tie = false
			continue
		}
		if count == maxVotes {
			tie = true
		}
	}

	if tie {
		return nil, nil
	}

	id := winner
	return &id, nil
}

func deterministicVoteTarget(g *Game, voter Player) (PlayerID, bool) {
	if voter.Role == RoleMafia {
		for i := range g.Players {
			target := g.Players[i]
			if target.Alive && target.ID != voter.ID && target.Role != RoleMafia {
				return target.ID, true
			}
		}
	}

	for i := range g.Players {
		target := g.Players[i]
		if target.Alive && target.ID != voter.ID && target.Role == RoleMafia {
			return target.ID, true
		}
	}

	for i := range g.Players {
		target := g.Players[i]
		if target.Alive && target.ID != voter.ID {
			return target.ID, true
		}
	}

	return 0, false
}

// TallyVotes resolves vote results from a collected vote map.
// Returns the eliminated player ID (plurality), or nil on tie.
// Uses player slice order to break iteration order deterministically.
func TallyVotes(g *Game, votes map[PlayerID]PlayerID) *PlayerID {
	counts := make(map[PlayerID]int)
	for _, target := range votes {
		counts[target]++
	}

	if len(counts) == 0 {
		return nil
	}

	var winner PlayerID
	maxVotes := 0
	tie := false

	// Iterate in player order for deterministic tie-breaking order
	for i := range g.Players {
		id := g.Players[i].ID
		count := counts[id]
		if count == 0 {
			continue
		}
		if count > maxVotes {
			winner = id
			maxVotes = count
			tie = false
		} else if count == maxVotes {
			tie = true
		}
	}

	if tie {
		return nil
	}

	id := winner
	return &id
}
