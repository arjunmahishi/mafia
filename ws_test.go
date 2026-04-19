package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// newTestWSServer creates an httptest server with a started game and zero bot delay.
func newTestWSServer(t *testing.T, playerCount int, seed int64) (*httptest.Server, *server) {
	t.Helper()
	s := newServer()
	s.botDelay = 0 // no delays in tests

	g, err := NewGame(playerCount, "", rand.New(rand.NewSource(seed)), nil)
	if err != nil {
		t.Fatalf("NewGame error: %v", err)
	}
	if err := g.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	s.game = g
	s.eventLog = []string{"Game started!"}
	s.game.EventLog = &s.eventLog

	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return ts, s
}

// wsClient wraps a websocket connection and a background reader goroutine
// that feeds messages into a channel without closing the connection on timeout.
type wsClient struct {
	conn *websocket.Conn
	ch   <-chan wsMessage
	done context.CancelFunc
}

// dialWS connects a WebSocket client to the test server and starts a background
// reader goroutine. The returned wsClient must be used for reading messages.
func dialWS(t *testing.T, ts *httptest.Server, s *server) *wsClient {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial error: %v", err)
	}

	// Wait for the server hub to register the connection.
	deadline := time.Now().Add(2 * time.Second)
	for !s.hub.connected() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.hub.connected() {
		c.CloseNow()
		t.Fatal("ws hub did not register connection in time")
	}

	// Start a long-lived reader goroutine.
	readCtx, readCancel := context.WithCancel(context.Background())
	ch := make(chan wsMessage, 200)
	go func() {
		defer close(ch)
		for {
			_, data, err := c.Read(readCtx)
			if err != nil {
				return
			}
			var msg wsMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				return
			}
			ch <- msg
		}
	}()

	t.Cleanup(func() {
		readCancel()
		c.CloseNow()
	})

	return &wsClient{conn: c, ch: ch, done: readCancel}
}

// drain collects messages from the background reader for the given duration.
func (wc *wsClient) drain(d time.Duration) []wsMessage {
	var msgs []wsMessage
	timer := time.After(d)
	for {
		select {
		case msg, ok := <-wc.ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, msg)
		case <-timer:
			return msgs
		}
	}
}

// postForm sends a POST request with form data to the test server.
func postForm(t *testing.T, ts *httptest.Server, path string, values url.Values) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+path, "application/x-www-form-urlencoded", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("POST %s error: %v", path, err)
	}
	return resp
}

func TestWSConnectReceivesInitialState(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	ts, s := newTestWSServer(t, 6, seed)

	// Drive the game so there's some state
	s.mu.Lock()
	s.driveGameLocked()
	s.mu.Unlock()

	wc := dialWS(t, ts, s)

	// Should receive initial state messages: phase-info, player-list, event-log, action-panel, lobby hide, game-area show
	msgs := wc.drain(2 * time.Second)

	if len(msgs) < 4 {
		t.Fatalf("expected at least 4 initial messages, got %d", len(msgs))
	}

	// Check we got the key targets
	targets := make(map[string]bool)
	for _, m := range msgs {
		targets[m.Target] = true
	}

	for _, want := range []string{"phase-info", "player-list", "event-log", "action-panel"} {
		if !targets[want] {
			t.Errorf("missing initial message for target %q", want)
		}
	}
}

func TestWSStartGameBroadcastsState(t *testing.T) {
	s := newServer()
	s.botDelay = 0
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)

	wc := dialWS(t, ts, s)

	// No game yet — initial snapshot should be minimal (game is nil)
	// Drain any initial messages
	wc.drain(500 * time.Millisecond)

	// Start a game via HTTP POST
	resp := postForm(t, ts, "/start", url.Values{"player_count": {"5"}})
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Should receive game state broadcasts
	msgs := wc.drain(3 * time.Second)

	if len(msgs) == 0 {
		t.Fatal("expected WS messages after game start, got none")
	}

	// Should include event log entries
	foundEvent := false
	for _, m := range msgs {
		if m.Target == "event-log" {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Error("expected event-log message after game start")
	}
}

func TestWSFullGamePlaythrough(t *testing.T) {
	s := newServer()
	s.botDelay = 0
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)

	wc := dialWS(t, ts, s)

	// Drain initial messages (no game)
	wc.drain(500 * time.Millisecond)

	// Find a seed for human villager so we only need message + vote actions
	seed := findSeedForHumanRole(t, 5, RoleVillager)

	// Override the server's game directly for deterministic testing
	g, err := NewGame(5, "", rand.New(rand.NewSource(seed)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	s.game = g
	s.eventLog = []string{"Game started!"}
	s.eventLog = append(s.eventLog, "You are the villager.")
	s.game.EventLog = &s.eventLog
	s.sendFullStateLocked(context.Background())
	s.mu.Unlock()

	s.driveGameAsync()

	// Play the game by responding to pending actions
	for rounds := 0; rounds < 100; rounds++ {
		// Wait for messages to settle
		wc.drain(500 * time.Millisecond)

		s.mu.Lock()
		phase := s.game.Phase
		pending := s.game.Pending
		s.mu.Unlock()

		if phase == PhaseEnded {
			break
		}

		if pending == nil {
			continue
		}

		switch pending.Type {
		case PendingMessage:
			resp := postForm(t, ts, "/action/message", url.Values{"message": {"I'm suspicious of everyone."}})
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("message POST returned %d", resp.StatusCode)
			}

		case PendingVote:
			target := pending.AllowedTargetIDs[0]
			resp := postForm(t, ts, "/action/vote", url.Values{"target": {strconv.Itoa(int(target))}})
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("vote POST returned %d", resp.StatusCode)
			}

		default:
			t.Fatalf("unexpected pending type for villager: %v", pending.Type)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.game.Phase != PhaseEnded {
		t.Fatalf("game did not end, phase=%v", s.game.Phase)
	}
	if s.game.Winner != WinnerVillage && s.game.Winner != WinnerMafia {
		t.Fatalf("unexpected winner: %v", s.game.Winner)
	}
}

func TestWSNoRedirectWhenConnected(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	ts, s := newTestWSServer(t, 6, seed)

	// Drive to get a pending message
	s.mu.Lock()
	s.driveGameLocked()
	s.mu.Unlock()

	// Connect WS and wait for hub registration
	wc := dialWS(t, ts, s)

	// Drain initial messages
	wc.drain(time.Second)

	// Verify hub is still connected after draining
	if !s.hub.connected() {
		t.Fatal("hub disconnected after draining messages")
	}

	// Ensure we have a pending message
	s.mu.Lock()
	hasPendingMessage := s.game.Pending != nil && s.game.Pending.Type == PendingMessage
	s.mu.Unlock()

	if !hasPendingMessage {
		t.Skip("no pending message for this seed")
	}

	// Send message — should get 200, not 303 redirect
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{"message": {"testing no redirect"}}
	resp, err := client.Post(ts.URL+"/action/message", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWSFallbackRedirectWithoutConnection(t *testing.T) {
	seed := findSeedForHumanRole(t, 6, RoleVillager)
	ts, s := newTestWSServer(t, 6, seed)

	// Drive to get a pending message — no WS connected
	s.mu.Lock()
	s.driveGameLocked()
	s.mu.Unlock()

	s.mu.Lock()
	hasPendingMessage := s.game.Pending != nil && s.game.Pending.Type == PendingMessage
	s.mu.Unlock()

	if !hasPendingMessage {
		t.Skip("no pending message for this seed")
	}

	// Send message without WS — should get redirect (303)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	form := url.Values{"message": {"testing redirect"}}
	resp, err := client.Post(ts.URL+"/action/message", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
}

func TestHubSendNilConnIsNoop(t *testing.T) {
	h := &hub{}
	err := h.send(context.Background(), wsMessage{Target: "test", Action: "replace", HTML: "<p>hi</p>"})
	if err != nil {
		t.Fatalf("expected nil error for nil conn, got %v", err)
	}
}

func TestHubConnectedReportsCorrectly(t *testing.T) {
	h := &hub{}
	if h.connected() {
		t.Fatal("expected not connected initially")
	}
}
