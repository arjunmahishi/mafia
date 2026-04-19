package main

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
)

func TestValidatePlayerCountBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		wantErr bool
	}{
		{name: "4 invalid", total: 4, wantErr: true},
		{name: "5 valid", total: 5, wantErr: false},
		{name: "10 valid", total: 10, wantErr: false},
		{name: "11 invalid", total: 11, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlayerCount(tt.total)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePlayerCount(%d) error=%v, wantErr=%v", tt.total, err, tt.wantErr)
			}
		})
	}
}

func TestRoleCountsN5ToN10(t *testing.T) {
	tests := []struct {
		n                                      int
		mafia, doctor, detective, villager int
	}{
		{n: 5, mafia: 1, doctor: 1, detective: 1, villager: 2},
		{n: 6, mafia: 1, doctor: 1, detective: 1, villager: 3},
		{n: 7, mafia: 1, doctor: 1, detective: 1, villager: 4},
		{n: 8, mafia: 2, doctor: 1, detective: 1, villager: 4},
		{n: 9, mafia: 2, doctor: 1, detective: 1, villager: 5},
		{n: 10, mafia: 2, doctor: 1, detective: 1, villager: 6},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.n), func(t *testing.T) {
			mafia, doctor, detective, villager := RoleCounts(tt.n)
			if mafia != tt.mafia || doctor != tt.doctor || detective != tt.detective || villager != tt.villager {
				t.Fatalf(
					"RoleCounts(%d)=(%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.n,
					mafia,
					doctor,
					detective,
					villager,
					tt.mafia,
					tt.doctor,
					tt.detective,
					tt.villager,
				)
			}
		})
	}
}

func TestNewGameInitialState(t *testing.T) {
	g := newTestGame(t, 8)

	if g.Phase != PhaseSetup {
		t.Fatalf("initial phase=%v, want %v", g.Phase, PhaseSetup)
	}
	if g.Winner != WinnerNone {
		t.Fatalf("initial winner=%v, want %v", g.Winner, WinnerNone)
	}
	if g.DayNumber != 0 {
		t.Fatalf("initial day number=%d, want 0", g.DayNumber)
	}

	humanCount := 0
	mafiaCount, doctorCount, detectiveCount, villagerCount := 0, 0, 0, 0
	for _, p := range g.Players {
		if p.IsHuman {
			humanCount++
		}
		if !p.Alive {
			t.Fatalf("player %d should start alive", p.ID)
		}

		switch p.Role {
		case RoleMafia:
			mafiaCount++
		case RoleDoctor:
			doctorCount++
		case RoleDetective:
			detectiveCount++
		case RoleVillager:
			villagerCount++
		default:
			t.Fatalf("unexpected role %q", p.Role)
		}
	}

	if humanCount != 1 {
		t.Fatalf("human count=%d, want 1", humanCount)
	}

	wantMafia, wantDoctor, wantDetective, wantVillager := RoleCounts(8)
	if mafiaCount != wantMafia || doctorCount != wantDoctor || detectiveCount != wantDetective || villagerCount != wantVillager {
		t.Fatalf(
			"initial role counts=(%d,%d,%d,%d), want (%d,%d,%d,%d)",
			mafiaCount,
			doctorCount,
			detectiveCount,
			villagerCount,
			wantMafia,
			wantDoctor,
			wantDetective,
			wantVillager,
		)
	}
}

func TestAdvancePhaseSequenceNoWinner(t *testing.T) {
	g := newTestGame(t, 6)
	want := []Phase{PhaseNight, PhaseDay, PhaseVote, PhaseWinCheck, PhaseNight}

	for i, phase := range want {
		if err := g.AdvancePhase(); err != nil {
			t.Fatalf("step %d AdvancePhase() error: %v", i+1, err)
		}
		if g.Phase != phase {
			t.Fatalf("step %d phase=%v, want %v", i+1, g.Phase, phase)
		}
	}
}

func TestVillageWinPath(t *testing.T) {
	g := newTestGame(t, 7)

	for _, player := range g.Players {
		if player.Role == RoleMafia {
			if err := g.Eliminate(player.ID, CauseVote); err != nil {
				t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
			}
		}
	}

	advanceToPhase(t, g, PhaseWinCheck)
	if err := g.AdvancePhase(); err != nil {
		t.Fatalf("AdvancePhase() from win_check error: %v", err)
	}

	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}
	if g.Winner != WinnerVillage {
		t.Fatalf("winner=%v, want %v", g.Winner, WinnerVillage)
	}
}

func TestMafiaWinPath(t *testing.T) {
	g := newTestGame(t, 5)

	for _, player := range g.Players {
		if player.Role == RoleMafia {
			continue
		}

		if err := g.Eliminate(player.ID, CauseNightKill); err != nil {
			t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
		}

		mafiaAlive, villageAlive := g.AliveCounts()
		if mafiaAlive >= villageAlive {
			break
		}
	}

	advanceToPhase(t, g, PhaseWinCheck)
	if err := g.AdvancePhase(); err != nil {
		t.Fatalf("AdvancePhase() from win_check error: %v", err)
	}

	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}
	if g.Winner != WinnerMafia {
		t.Fatalf("winner=%v, want %v", g.Winner, WinnerMafia)
	}
}

func TestEliminateBehavior(t *testing.T) {
	g := newTestGame(t, 5)
	target := g.Players[1]

	if err := g.Eliminate(target.ID, CauseVote); err != nil {
		t.Fatalf("Eliminate(%d) error: %v", target.ID, err)
	}

	updated, err := g.FindPlayer(target.ID)
	if err != nil {
		t.Fatalf("FindPlayer(%d) error: %v", target.ID, err)
	}
	if updated.Alive {
		t.Fatalf("player %d should be dead", target.ID)
	}
	if !updated.RoleRevealed {
		t.Fatalf("player %d role should be revealed", target.ID)
	}
	if g.LastEliminated == nil {
		t.Fatal("LastEliminated should be set")
	}
	if *g.LastEliminated != target.ID {
		t.Fatalf("LastEliminated=%d, want %d", *g.LastEliminated, target.ID)
	}
}

func TestEliminateInvalidCases(t *testing.T) {
	g := newTestGame(t, 5)

	if err := g.Eliminate(PlayerID(99999), CauseVote); err == nil {
		t.Fatal("expected error when eliminating unknown player")
	}

	id := g.Players[0].ID
	if err := g.Eliminate(id, CauseVote); err != nil {
		t.Fatalf("first Eliminate(%d) error: %v", id, err)
	}
	if err := g.Eliminate(id, CauseVote); err == nil {
		t.Fatal("expected error when eliminating already-dead player")
	}
}

func TestAliveCountsMixedAliveDead(t *testing.T) {
	g := newTestGame(t, 8)

	removedMafia := 0
	removedVillage := 0
	for _, player := range g.Players {
		switch {
		case removedMafia < 1 && player.Role == RoleMafia:
			if err := g.Eliminate(player.ID, CauseNightKill); err != nil {
				t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
			}
			removedMafia++
		case removedVillage < 2 && player.Role != RoleMafia:
			if err := g.Eliminate(player.ID, CauseVote); err != nil {
				t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
			}
			removedVillage++
		}
	}

	mafiaAlive, villageAlive := g.AliveCounts()
	if mafiaAlive != 1 || villageAlive != 4 {
		t.Fatalf("AliveCounts()=(%d,%d), want (1,4)", mafiaAlive, villageAlive)
	}
}

func TestAdvancePhaseFromEndedReturnsError(t *testing.T) {
	g := newTestGame(t, 7)
	for _, player := range g.Players {
		if player.Role == RoleMafia {
			if err := g.Eliminate(player.ID, CauseVote); err != nil {
				t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
			}
		}
	}

	advanceToPhase(t, g, PhaseWinCheck)
	if err := g.AdvancePhase(); err != nil {
		t.Fatalf("AdvancePhase() from win_check error: %v", err)
	}
	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}

	if err := g.AdvancePhase(); err == nil {
		t.Fatal("expected error from AdvancePhase when game already ended")
	}
	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v after error, want %v", g.Phase, PhaseEnded)
	}
}

func TestRunUntilEndDeterministicResolver(t *testing.T) {
	g := newTestGame(t, 5)

	winner, err := g.RunUntilEnd(deterministicResolver{}, 100)
	if err != nil {
		t.Fatalf("RunUntilEnd() error: %v", err)
	}
	if winner != WinnerVillage && winner != WinnerMafia {
		t.Fatalf("winner=%v, expected terminal winner", winner)
	}
	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}
	if g.Winner != winner {
		t.Fatalf("game winner=%v, returned winner=%v", g.Winner, winner)
	}
}

func TestRunUntilEndNightEliminationImmediateMafiaWin(t *testing.T) {
	g := newTestGame(t, 5)

	removedVillage := 0
	for _, player := range g.Players {
		if player.Role == RoleMafia {
			continue
		}
		if err := g.Eliminate(player.ID, CauseVote); err != nil {
			t.Fatalf("Eliminate(%d) error: %v", player.ID, err)
		}
		removedVillage++
		if removedVillage == 2 {
			break
		}
	}

	advanceToPhase(t, g, PhaseNight)

	var nightTarget PlayerID
	for _, player := range g.Players {
		if player.Alive && player.Role != RoleMafia {
			nightTarget = player.ID
			break
		}
	}

	resolver := fixedResolver{nightTarget: &nightTarget}
	winner, err := g.RunUntilEnd(resolver, 10)
	if err != nil {
		t.Fatalf("RunUntilEnd() error: %v", err)
	}
	if winner != WinnerMafia {
		t.Fatalf("winner=%v, want %v", winner, WinnerMafia)
	}
	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}
	if g.Winner != WinnerMafia {
		t.Fatalf("game winner=%v, want %v", g.Winner, WinnerMafia)
	}
}

func TestRunUntilEndNightEliminationImmediateVillageWin(t *testing.T) {
	g := newTestGame(t, 5)

	advanceToPhase(t, g, PhaseNight)

	var mafiaTarget PlayerID
	for _, player := range g.Players {
		if player.Alive && player.Role == RoleMafia {
			mafiaTarget = player.ID
			break
		}
	}

	resolver := fixedResolver{nightTarget: &mafiaTarget}
	winner, err := g.RunUntilEnd(resolver, 10)
	if err != nil {
		t.Fatalf("RunUntilEnd() error: %v", err)
	}
	if winner != WinnerVillage {
		t.Fatalf("winner=%v, want %v", winner, WinnerVillage)
	}
	if g.Phase != PhaseEnded {
		t.Fatalf("phase=%v, want %v", g.Phase, PhaseEnded)
	}
	if g.Winner != WinnerVillage {
		t.Fatalf("game winner=%v, want %v", g.Winner, WinnerVillage)
	}
}

func TestRunUntilEndValidationErrors(t *testing.T) {
	g := newTestGame(t, 5)

	tests := []struct {
		name      string
		resolver  RoundResolver
		maxCycles int
		wantErr   string
	}{
		{name: "nil resolver", resolver: nil, maxCycles: 1, wantErr: "resolver cannot be nil"},
		{name: "zero max cycles", resolver: noOpResolver{}, maxCycles: 0, wantErr: "maxCycles must be positive"},
		{name: "negative max cycles", resolver: noOpResolver{}, maxCycles: -1, wantErr: "maxCycles must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := g.RunUntilEnd(tt.resolver, tt.maxCycles)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error=%q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunUntilEndPropagatesResolverErrors(t *testing.T) {
	nightErr := errors.New("night resolver failed")
	voteErr := errors.New("vote resolver failed")

	tests := []struct {
		name     string
		phase    Phase
		resolver RoundResolver
		wantErr  error
	}{
		{name: "night resolver error", phase: PhaseNight, resolver: fixedResolver{nightErr: nightErr}, wantErr: nightErr},
		{name: "vote resolver error", phase: PhaseVote, resolver: fixedResolver{voteErr: voteErr}, wantErr: voteErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGame(t, 5)
			advanceToPhase(t, g, tt.phase)

			_, err := g.RunUntilEnd(tt.resolver, 10)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunUntilEndMaxCyclesExceeded(t *testing.T) {
	g := newTestGame(t, 5)

	winner, err := g.RunUntilEnd(noOpResolver{}, 1)
	if err == nil {
		t.Fatal("expected error when maxCycles exceeded")
	}
	if err.Error() != "maxCycles exceeded" {
		t.Fatalf("error=%q, want %q", err.Error(), "maxCycles exceeded")
	}
	if winner != WinnerNone {
		t.Fatalf("winner=%v, want %v", winner, WinnerNone)
	}
}

type deterministicResolver struct{}

func (deterministicResolver) ResolveNight(g *Game) (*PlayerID, error) {
	for _, player := range g.Players {
		if player.Alive && player.Role != RoleMafia {
			id := player.ID
			return &id, nil
		}
	}
	return nil, nil
}

func (deterministicResolver) ResolveVote(g *Game) (*PlayerID, error) {
	for _, player := range g.Players {
		if player.Alive && player.Role == RoleMafia {
			id := player.ID
			return &id, nil
		}
	}
	return nil, nil
}

type fixedResolver struct {
	nightTarget *PlayerID
	nightErr    error
	voteTarget  *PlayerID
	voteErr     error
}

func (r fixedResolver) ResolveNight(*Game) (*PlayerID, error) {
	if r.nightErr != nil {
		return nil, r.nightErr
	}
	return r.nightTarget, nil
}

func (r fixedResolver) ResolveVote(*Game) (*PlayerID, error) {
	if r.voteErr != nil {
		return nil, r.voteErr
	}
	return r.voteTarget, nil
}

type noOpResolver struct{}

func (noOpResolver) ResolveNight(*Game) (*PlayerID, error) { return nil, nil }

func (noOpResolver) ResolveVote(*Game) (*PlayerID, error) { return nil, nil }

func newTestGame(t *testing.T, totalPlayers int) *Game {
	t.Helper()

	g, err := NewGame(totalPlayers, "", rand.New(rand.NewSource(1)), nil)
	if err != nil {
		t.Fatalf("NewGame(%d) error: %v", totalPlayers, err)
	}

	return g
}

func advanceToPhase(t *testing.T, g *Game, want Phase) {
	t.Helper()

	for i := 0; i < 10 && g.Phase != want; i++ {
		if err := g.AdvancePhase(); err != nil {
			t.Fatalf("AdvancePhase() error while moving to %v: %v", want, err)
		}
	}

	if g.Phase != want {
		t.Fatalf("phase=%v, want %v", g.Phase, want)
	}
}
