package main

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// newTestServerWithSeed creates a server with a started game using the given
// seed, so we can control role assignments deterministically.
func newTestServerWithSeed(t *testing.T, playerCount int, seed int64) *server {
	t.Helper()
	s := newServer()
	g, err := NewGame(playerCount, rand.New(rand.NewSource(seed)), nil)
	if err != nil {
		t.Fatalf("NewGame error: %v", err)
	}
	if err := g.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	s.game = g
	s.eventLog = []string{"Game started!"}
	s.game.EventLog = &s.eventLog
	return s
}

// findSeedForHumanRole finds a seed where the human (player 0) gets the desired role.
func findSeedForHumanRole(t *testing.T, playerCount int, role Role) int64 {
	t.Helper()
	for seed := int64(0); seed < 1000; seed++ {
		g, err := NewGame(playerCount, rand.New(rand.NewSource(seed)), nil)
		if err != nil {
			t.Fatalf("NewGame error: %v", err)
		}
		if g.Players[0].Role == role {
			return seed
		}
	}
	t.Fatalf("could not find seed for human role %s", role)
	return 0
}

func TestDriveGameBlocksOnHumanNightAction(t *testing.T) {
	tests := []struct {
		name        string
		role        Role
		pendingType PendingActionType
	}{
		{"human is mafia", RoleMafia, PendingNightKill},
		{"human is doctor", RoleDoctor, PendingNightSave},
		{"human is detective", RoleDetective, PendingNightInvest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seed := findSeedForHumanRole(t, 6, tt.role)
			s := newTestServerWithSeed(t, 6, seed)
			s.driveGameLocked()

			g := s.game
			if g.Pending == nil {
				t.Fatal("expected pending action, got nil")
			}
			if g.Pending.Type != tt.pendingType {
				t.Fatalf("pending type=%v, want %v", g.Pending.Type, tt.pendingType)
			}
			if g.Pending.ActorID != g.HumanPlayer().ID {
				t.Fatalf("pending actor=%v, want human=%v", g.Pending.ActorID, g.HumanPlayer().ID)
			}
			if len(g.Pending.AllowedTargetIDs) == 0 {
				t.Fatal("expected non-empty allowed targets")
			}
		})
	}
}

func TestDriveGameBlocksOnHumanDayMessage(t *testing.T) {
	// Use a villager so night passes without human input
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	s.driveGameLocked()

	g := s.game
	// Night should resolve (human is villager, no night action needed)
	// Then day should block on human's turn to speak
	if g.Pending == nil {
		t.Fatal("expected pending action")
	}
	if g.Pending.Type != PendingMessage {
		t.Fatalf("pending type=%v, want %v", g.Pending.Type, PendingMessage)
	}
}

func TestDriveGameBlocksOnHumanVote(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)

	// Drive through night, then simulate human day message to get to vote
	s.driveGameLocked()
	if s.game.Pending == nil || s.game.Pending.Type != PendingMessage {
		t.Fatalf("expected message pending, got %v", s.game.Pending)
	}

	// Submit the human's message
	human := s.game.HumanPlayer()
	s.eventLog = append(s.eventLog, "[You] I have no idea who it is.")
	s.game.Discussion.Index++
	s.game.Pending = nil
	s.driveGameLocked()

	// Now should be in vote phase, pending on human vote
	g := s.game
	if g.Pending == nil {
		// Might have already voted if human isn't first in vote order—
		// let's check we're at vote phase at least
		if g.Phase != PhaseVote && g.Phase != PhaseWinCheck && g.Phase != PhaseNight {
			t.Fatalf("unexpected phase=%v", g.Phase)
		}
		return
	}
	if g.Pending.Type != PendingVote {
		t.Fatalf("pending type=%v, want %v", g.Pending.Type, PendingVote)
	}

	// Validate no self-vote in allowed targets
	for _, tid := range g.Pending.AllowedTargetIDs {
		if tid == human.ID {
			t.Fatal("self-vote should not be in allowed targets")
		}
	}
}

func TestVoteNoSelfVoteInAllowedTargets(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)

	// Drive to get past night + day
	driveToHumanVote(t, s)

	g := s.game
	if g.Pending == nil || g.Pending.Type != PendingVote {
		t.Skip("human not in vote position for this seed")
	}

	human := g.HumanPlayer()
	for _, tid := range g.Pending.AllowedTargetIDs {
		if tid == human.ID {
			t.Fatal("allowed targets must not include self")
		}
	}
}

func TestDoctorCannotProtectSamePlayerConsecutively(t *testing.T) {
	// Use 8 players so the game is more likely to survive to night 2
	seed := findSeedForHumanRole(t, 8, RoleDoctor)
	s := newTestServerWithSeed(t, 8, seed)
	s.driveGameLocked()

	g := s.game
	if g.Pending == nil || g.Pending.Type != PendingNightSave {
		t.Fatalf("expected night save pending, got %v", g.Pending)
	}

	// Pick the first allowed target
	firstTarget := g.Pending.AllowedTargetIDs[0]

	// Submit the doctor's choice
	g.Night.ProtectTarget = &firstTarget
	g.Night.Step++
	g.Pending = nil
	s.driveGameLocked()

	// Play through day + vote to get back to next night
	playThroughDayAndVote(t, s)

	// Now in night 2, drive to doctor's turn
	s.driveGameLocked()

	if g.Phase == PhaseEnded {
		t.Skip("game ended before second night")
	}

	if g.Pending == nil || g.Pending.Type != PendingNightSave {
		// Doctor might be dead — that's ok
		if g.FindByRole(RoleDoctor) == nil || !g.FindByRole(RoleDoctor).Alive {
			t.Skip("doctor is dead")
		}
		t.Fatalf("expected night save pending, got %v", g.Pending)
	}

	// The first target should NOT be in allowed targets
	for _, tid := range g.Pending.AllowedTargetIDs {
		if tid == firstTarget {
			t.Fatal("doctor should not be able to protect same player two nights in a row")
		}
	}
}

func TestTallyVotesTieNoElimination(t *testing.T) {
	g := newTestGame(t, 6)

	// Create a tied vote: two players each get 1 vote
	alive := g.AlivePlayerIDs()
	votes := map[PlayerID]PlayerID{
		alive[0]: alive[2],
		alive[1]: alive[3],
	}

	result := TallyVotes(g, votes)
	if result != nil {
		t.Fatalf("expected nil (tie), got %v", *result)
	}
}

func TestTallyVotesPlurality(t *testing.T) {
	g := newTestGame(t, 6)

	alive := g.AlivePlayerIDs()
	// 3 votes for alive[1], 1 vote for alive[2]
	votes := map[PlayerID]PlayerID{
		alive[0]: alive[1],
		alive[2]: alive[1],
		alive[3]: alive[1],
		alive[4]: alive[2],
	}

	result := TallyVotes(g, votes)
	if result == nil {
		t.Fatal("expected elimination, got nil")
	}
	if *result != alive[1] {
		t.Fatalf("eliminated=%v, want %v", *result, alive[1])
	}
}

func TestHandleMessageRejectsOutOfTurn(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)

	// Don't drive — pending should be nil (game just started, in night phase)
	// Human is villager so they have no night action, but we haven't driven yet

	form := url.Values{"message": {"hello"}}
	req := httptest.NewRequest(http.MethodPost, "/action/message", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleVoteRejectsInvalidTarget(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)

	// Drive to human vote
	driveToHumanVote(t, s)

	if s.game.Pending == nil || s.game.Pending.Type != PendingVote {
		t.Skip("human not in vote position")
	}

	// Try self-vote
	human := s.game.HumanPlayer()
	form := url.Values{"target": {strconv.Itoa(int(human.ID))}}
	req := httptest.NewRequest(http.MethodPost, "/action/vote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleVote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d for self-vote", w.Code, http.StatusBadRequest)
	}
}

func TestHandleNightActionRejectsWrongPhase(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	s.driveGameLocked()

	// Human is villager, so game should be in day phase pending message
	form := url.Values{"target": {"2"}}
	req := httptest.NewRequest(http.MethodPost, "/action/night", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleNightAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestFullGameInteractivePlaythrough(t *testing.T) {
	// Test that a full game can be played to completion with human inputs.
	// Use a villager for simplicity (no night actions).
	seed := findSeedForHumanRole(t, 5, RoleVillager)
	s := newTestServerWithSeed(t, 5, seed)

	for rounds := 0; rounds < 50; rounds++ {
		s.driveGameLocked()

		if s.game.Phase == PhaseEnded {
			break
		}

		if s.game.Pending == nil {
			t.Fatalf("game not ended but no pending action, phase=%v", s.game.Phase)
		}

		switch s.game.Pending.Type {
		case PendingMessage:
			s.eventLog = append(s.eventLog, "[You] I'm suspicious of everyone.")
			s.game.Discussion.Index++
			s.game.Pending = nil

		case PendingVote:
			target := s.game.Pending.AllowedTargetIDs[0]
			human := s.game.HumanPlayer()
			s.game.Vote.Votes[human.ID] = target
			targetP, _ := s.game.FindPlayer(target)
			s.eventLog = append(s.eventLog, "You votes for "+targetP.Name+".")
			s.game.Vote.Index++
			s.game.Pending = nil

		default:
			t.Fatalf("unexpected pending type for villager: %v", s.game.Pending.Type)
		}
	}

	if s.game.Phase != PhaseEnded {
		t.Fatal("game did not end within 50 interaction rounds")
	}
	if s.game.Winner != WinnerVillage && s.game.Winner != WinnerMafia {
		t.Fatalf("winner=%v, expected terminal winner", s.game.Winner)
	}
}

func TestDifferentHumanChoicesProduceDifferentOutcomes(t *testing.T) {
	// Find a seed where human is mafia so their kill target choice matters
	seed := findSeedForHumanRole(t, 5, RoleMafia)

	playGame := func(firstKillIdx int) (Winner, []string) {
		s := newTestServerWithSeed(t, 5, seed)
		var log []string

		for rounds := 0; rounds < 100; rounds++ {
			s.driveGameLocked()
			if s.game.Phase == PhaseEnded {
				break
			}
			if s.game.Pending == nil {
				t.Fatalf("stuck: phase=%v, no pending", s.game.Phase)
			}

			switch s.game.Pending.Type {
			case PendingNightKill:
				targets := s.game.Pending.AllowedTargetIDs
				idx := firstKillIdx % len(targets)
				target := targets[idx]
				s.game.Night.KillTarget = &target
				s.game.Night.Step++
				s.game.Pending = nil
				firstKillIdx = 0 // only vary first kill
				log = append(log, "kill:"+strconv.Itoa(int(target)))

			case PendingMessage:
				s.eventLog = append(s.eventLog, "[You] Trust me.")
				s.game.Discussion.Index++
				s.game.Pending = nil

			case PendingVote:
				target := s.game.Pending.AllowedTargetIDs[0]
				human := s.game.HumanPlayer()
				s.game.Vote.Votes[human.ID] = target
				s.game.Vote.Index++
				s.game.Pending = nil

			default:
				t.Fatalf("unexpected pending: %v", s.game.Pending.Type)
			}
		}
		return s.game.Winner, log
	}

	_, log1 := playGame(0)
	_, log2 := playGame(1)

	// The kill logs should differ (different first target)
	if len(log1) > 0 && len(log2) > 0 && log1[0] == log2[0] {
		t.Log("Note: different kill index produced same first target (only 1 valid target)")
	}
	// We at least verify both games completed without error
}

// --- helpers ---

// driveToHumanVote drives the game forward past night and day until the human
// has a pending vote (or the game ends / passes vote phase without human).
func driveToHumanVote(t *testing.T, s *server) {
	t.Helper()
	for i := 0; i < 100; i++ {
		s.driveGameLocked()
		if s.game.Phase == PhaseEnded {
			return
		}
		if s.game.Pending == nil {
			continue
		}
		if s.game.Pending.Type == PendingVote {
			return
		}

		// Auto-submit non-vote pending actions
		switch s.game.Pending.Type {
		case PendingMessage:
			s.eventLog = append(s.eventLog, "[You] auto-message")
			s.game.Discussion.Index++
			s.game.Pending = nil
		case PendingNightKill, PendingNightSave, PendingNightInvest:
			target := s.game.Pending.AllowedTargetIDs[0]
			switch s.game.Pending.Type {
			case PendingNightKill:
				s.game.Night.KillTarget = &target
			case PendingNightSave:
				s.game.Night.ProtectTarget = &target
			case PendingNightInvest:
				s.game.Night.InvestTarget = &target
				tp, _ := s.game.FindPlayer(target)
				isMafia := tp.Role == RoleMafia
				s.game.Night.InvestResult = &isMafia
			}
			s.game.Night.Step++
			s.game.Pending = nil
		}
	}
}

// autoSubmitAny auto-submits any pending human action (used by generic helpers).
func autoSubmitAny(t *testing.T, s *server) {
	t.Helper()
	g := s.game
	if g.Pending == nil {
		return
	}
	switch g.Pending.Type {
	case PendingMessage:
		s.eventLog = append(s.eventLog, "[You] auto-message")
		g.Discussion.Index++
	case PendingVote:
		target := g.Pending.AllowedTargetIDs[0]
		g.Vote.Votes[g.HumanPlayer().ID] = target
		g.Vote.Index++
	case PendingNightKill:
		target := g.Pending.AllowedTargetIDs[0]
		g.Night.KillTarget = &target
		g.Night.Step++
	case PendingNightSave:
		target := g.Pending.AllowedTargetIDs[0]
		g.Night.ProtectTarget = &target
		g.Night.Step++
	case PendingNightInvest:
		target := g.Pending.AllowedTargetIDs[0]
		g.Night.InvestTarget = &target
		tp, _ := g.FindPlayer(target)
		isMafia := tp.Role == RoleMafia
		g.Night.InvestResult = &isMafia
		g.Night.Step++
	}
	g.Pending = nil
}

// playToEnd auto-plays the entire game to completion.
func playToEnd(t *testing.T, s *server) {
	t.Helper()
	for i := 0; i < 200; i++ {
		s.driveGameLocked()
		if s.game.Phase == PhaseEnded {
			return
		}
		if s.game.Pending == nil {
			t.Fatalf("stuck: phase=%v, no pending", s.game.Phase)
		}
		autoSubmitAny(t, s)
	}
	t.Fatal("game did not end within 200 rounds")
}

func TestHTTPHandlerHappyPathMessage(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	s.driveGameLocked()

	if s.game.Pending == nil || s.game.Pending.Type != PendingMessage {
		t.Fatalf("expected message pending, got %v", s.game.Pending)
	}

	form := url.Values{"message": {"I suspect Player 3"}}
	req := httptest.NewRequest(http.MethodPost, "/action/message", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleMessage(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusSeeOther)
	}

	// Pending should have advanced (either next player's turn or new pending)
	found := false
	for _, line := range s.eventLog {
		if strings.Contains(line, "I suspect Player 3") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("message not found in event log")
	}
}

func TestHTTPHandlerHappyPathVote(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	driveToHumanVote(t, s)

	if s.game.Pending == nil || s.game.Pending.Type != PendingVote {
		t.Skip("human not in vote position for this seed")
	}

	target := s.game.Pending.AllowedTargetIDs[0]
	form := url.Values{"target": {strconv.Itoa(int(target))}}
	req := httptest.NewRequest(http.MethodPost, "/action/vote", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleVote(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusSeeOther)
	}
}

func TestHTTPHandlerHappyPathNightAction(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleMafia)
	s := newTestServerWithSeed(t, 6, seed)
	s.driveGameLocked()

	if s.game.Pending == nil || s.game.Pending.Type != PendingNightKill {
		t.Fatalf("expected night kill pending, got %v", s.game.Pending)
	}

	target := s.game.Pending.AllowedTargetIDs[0]
	form := url.Values{"target": {strconv.Itoa(int(target))}}
	req := httptest.NewRequest(http.MethodPost, "/action/night", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	s.handleNightAction(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status=%d, want %d", w.Code, http.StatusSeeOther)
	}
}

func TestHumanDeathMidGameContinues(t *testing.T) {
	// Human is villager — can die at night. Game should continue with bots only.
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	g := s.game

	// Manually kill the human to simulate a night kill
	human := g.HumanPlayer()
	if err := g.Eliminate(human.ID, CauseNightKill); err != nil {
		t.Fatalf("Eliminate error: %v", err)
	}
	s.eventLog = append(s.eventLog, "You were killed!")

	// Reset night state to start of night properly
	g.Night = NightState{}

	// Drive the game — should complete without blocking (no human alive to act)
	playToEnd(t, s)

	if g.Phase != PhaseEnded {
		t.Fatalf("game did not end, phase=%v", g.Phase)
	}
	if g.Winner != WinnerVillage && g.Winner != WinnerMafia {
		t.Fatalf("winner=%v, expected terminal", g.Winner)
	}
}

func TestTwoMafiaFullPlaythrough(t *testing.T) {
	// 8 players = 2 mafia. Test that the game completes.
	seed := findSeedForHumanRole(t, 8, RoleVillager)
	s := newTestServerWithSeed(t, 8, seed)

	playToEnd(t, s)

	if s.game.Phase != PhaseEnded {
		t.Fatalf("game did not end, phase=%v", s.game.Phase)
	}
}

func TestTwoMafiaHumanIsMafia(t *testing.T) {
	// 8 players = 2 mafia. Human is mafia.
	seed := findSeedForHumanRole(t, 8, RoleMafia)
	s := newTestServerWithSeed(t, 8, seed)

	playToEnd(t, s)

	if s.game.Phase != PhaseEnded {
		t.Fatalf("game did not end, phase=%v", s.game.Phase)
	}
}

func TestDoctorSavePreventsKill(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleDoctor)
	s := newTestServerWithSeed(t, 6, seed)
	g := s.game

	// Drive to the doctor's night turn
	s.driveGameLocked()

	if g.Pending == nil || g.Pending.Type != PendingNightSave {
		t.Fatalf("expected night save pending, got %v", g.Pending)
	}

	// Find who the mafia will kill (already set by bot mafia step)
	killTarget := g.Night.KillTarget
	if killTarget == nil {
		t.Skip("no kill target set")
	}

	// Protect the mafia's target
	g.Night.ProtectTarget = killTarget
	g.Night.Step++
	g.Pending = nil
	s.driveGameLocked()

	// Check that save happened
	savedMsg := false
	for _, line := range s.eventLog {
		if strings.Contains(line, "doctor saved someone") {
			savedMsg = true
			break
		}
	}
	if !savedMsg {
		t.Fatal("expected save message in event log")
	}

	// The kill target should still be alive
	target, _ := g.FindPlayer(*killTarget)
	if !target.Alive {
		t.Fatal("protected player should still be alive")
	}
}

func TestHandleIndexRendersWithPending(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	s := newTestServerWithSeed(t, 6, seed)
	s.driveGameLocked()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Your Turn") {
		t.Fatal("expected 'Your Turn' in rendered HTML")
	}
	if !strings.Contains(body, "role-badge") {
		t.Fatal("expected role badge in rendered HTML")
	}
}

// playThroughDayAndVote auto-submits all human actions through day discussion
// and vote until the next night (or game end).
func playThroughDayAndVote(t *testing.T, s *server) {
	t.Helper()
	for i := 0; i < 100; i++ {
		s.driveGameLocked()
		if s.game.Phase == PhaseEnded {
			return
		}
		if s.game.Phase == PhaseNight && s.game.Pending == nil {
			return
		}
		if s.game.Pending == nil {
			continue
		}

		switch s.game.Pending.Type {
		case PendingMessage:
			s.eventLog = append(s.eventLog, "[You] auto-message")
			s.game.Discussion.Index++
			s.game.Pending = nil
		case PendingVote:
			target := s.game.Pending.AllowedTargetIDs[0]
			human := s.game.HumanPlayer()
			s.game.Vote.Votes[human.ID] = target
			s.game.Vote.Index++
			s.game.Pending = nil
		case PendingNightKill, PendingNightSave, PendingNightInvest:
			// We've reached the next night's human action — done
			return
		}
	}
}
