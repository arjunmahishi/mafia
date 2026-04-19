package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Template fragments — each named template can be rendered independently for
// WebSocket pushes, or composed together for the full page render.

const tmplPhaseInfo = `{{define "phase-info"}}
<strong>Phase:</strong> {{.Phase}} {{if gt .DayNumber 0}}(Day {{.DayNumber}}){{end}}
{{if .HumanRole}}
  <span class="role-badge role-{{.HumanRole}}">Your role: {{.HumanRole}}</span>
{{end}}
{{end}}`

const tmplActionPanel = `{{define "action-panel"}}
{{if .Winner}}
  <p><strong>Winner: {{.Winner}}</strong></p>
  <form method="post" action="/start">
    <input type="hidden" name="player_count" value="{{len .Players}}" />
    <button type="submit">Play again</button>
  </form>
{{else if .Pending}}
  <div class="action-panel">
    <h3>Your turn</h3>
    <p>{{.Pending.Prompt}}</p>

    {{if eq .Pending.Type "message"}}
      <form method="post" action="/action/message">
        <textarea name="message" placeholder="Type your message..." required></textarea>
        <br/><button type="submit">Send message</button>
      </form>
    {{else if eq .Pending.Type "vote"}}
      <form method="post" action="/action/vote">
        <select name="target" required>
          <option value="">— Choose who to eliminate —</option>
          {{range .AllowedTargets}}
            <option value="{{.ID}}">{{.Name}}</option>
          {{end}}
        </select>
        <button type="submit">Cast vote</button>
      </form>
    {{else}}
      <form method="post" action="/action/night">
        <select name="target" required>
          <option value="">— Choose target —</option>
          {{range .AllowedTargets}}
            <option value="{{.ID}}">{{.Name}}</option>
          {{end}}
        </select>
        <button type="submit">Confirm</button>
      </form>
    {{end}}
  </div>
{{else}}
  <p class="waiting">Waiting...</p>
{{end}}
{{end}}`

const tmplPlayerList = `{{define "player-list"}}
<ul>
  {{range .Players}}
    <li class="{{if not .Alive}}dead{{end}} {{if .IsHuman}}you{{end}}">
      {{.Name}} — {{if .Alive}}alive{{else}}dead{{end}}
      {{if or .RoleRevealed $.RevealAllRoles}} ({{.Role}}){{end}}
    </li>
  {{end}}
</ul>
{{end}}`

const tmplEventItem = `{{define "event-item"}}<li>{{.}}</li>{{end}}`

const tmplLobby = `{{define "lobby"}}
<form method="post" action="/start">
  <label for="player_count">Players:</label>
  <input id="player_count" name="player_count" type="number" min="5" max="10" value="8" />
  <button type="submit">Start game</button>
</form>
{{end}}`

const indexTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>AI Mafia</title>
  <style>
    body { font-family: sans-serif; margin: 2rem; background: #fafafa; }
    h1 { margin-bottom: 0.5rem; }
    .row { display: flex; gap: 2rem; align-items: flex-start; flex-wrap: wrap; }
    .panel { border: 1px solid #ddd; border-radius: 8px; padding: 1rem; min-width: 280px; background: #fff; }
    .dead { color: #999; text-decoration: line-through; }
    .you { font-weight: bold; }
    ul { padding-left: 1.2rem; }
    .action-panel { border: 2px solid #4a90d9; border-radius: 8px; padding: 1rem; margin: 1rem 0; background: #f0f7ff; }
    .action-panel h3 { margin-top: 0; color: #4a90d9; }
    .waiting { color: #888; font-style: italic; padding: 1rem; }
    button[type=submit] { padding: 0.4rem 1rem; cursor: pointer; }
    select { padding: 0.3rem; min-width: 150px; }
    textarea { width: 100%; min-height: 60px; }
    .role-badge { display: inline-block; padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.85em; margin-left: 0.5rem; }
    .role-mafia { background: #f8d7da; color: #721c24; }
    .role-doctor { background: #d4edda; color: #155724; }
    .role-detective { background: #cce5ff; color: #004085; }
    .role-villager { background: #e2e3e5; color: #383d41; }
    #event-log { max-height: 400px; overflow-y: auto; }
  </style>
</head>
<body>
  <h1>AI Mafia</h1>

  <div id="lobby">
  {{if not .HasGame}}
    {{template "lobby" .}}
  {{end}}
  </div>

  <div id="game-area" {{if not .HasGame}}style="display:none"{{end}}>
    <p id="phase-info">{{if .HasGame}}{{template "phase-info" .}}{{end}}</p>

    <div id="action-panel">
      {{if .HasGame}}{{template "action-panel" .}}{{end}}
    </div>

    <div class="row">
      <div class="panel">
        <h2>Players</h2>
        <div id="player-list">
          {{if .HasGame}}{{template "player-list" .}}{{end}}
        </div>
      </div>

      <div class="panel">
        <h2>Event log</h2>
        <ul id="event-log">
          {{range .EventLog}}
            <li>{{.}}</li>
          {{end}}
        </ul>
      </div>
    </div>
  </div>

  <script>
  (function() {
    var ws;
    var reconnectTimer;

    function connect() {
      var proto = location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(proto + "//" + location.host + "/ws");

      ws.onmessage = function(e) {
        var msg = JSON.parse(e.data);
        var el = document.getElementById(msg.target);
        if (!el) return;

        if (msg.action === "append") {
          el.insertAdjacentHTML("beforeend", msg.html);
          el.scrollTop = el.scrollHeight;
        } else if (msg.action === "replace") {
          el.innerHTML = msg.html;
        } else if (msg.action === "show") {
          el.style.display = "";
          if (msg.html) el.innerHTML = msg.html;
        } else if (msg.action === "hide") {
          el.style.display = "none";
        }
      };

      ws.onclose = function() {
        ws = null;
        clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(connect, 2000);
      };

      ws.onerror = function() {
        if (ws) ws.close();
      };
    }

    connect();

    // Intercept form submissions and send via fetch when WS is active.
    document.addEventListener("submit", function(e) {
      var form = e.target;
      var action = form.getAttribute("action") || "";
      if (!action.match(/^\/(start|action\/)/)) return;
      if (!ws || ws.readyState !== WebSocket.OPEN) return;

      e.preventDefault();
      fetch(action, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
      }).then(function(res) {
        if (res.ok) {
          form.reset();
        } else {
          res.text().then(function(t) { alert(t); });
        }
      });
    });
  })();
  </script>
</body>
</html>
`

type server struct {
	mu       sync.Mutex
	game     *Game
	eventLog []string
	tmpl     *template.Template
	hub      hub
	botDelay time.Duration // delay between bot actions for pacing
	driving  bool          // true when driveGameAsync goroutine is active
	newAgent NewAgentFunc  // factory for creating bot agents
}

type indexData struct {
	HasGame        bool
	Phase          Phase
	DayNumber      int
	Winner         Winner
	Players        []Player
	EventLog       []string
	RevealAllRoles bool
	HumanRole      Role
	Pending        *PendingAction
	AllowedTargets []Player // player objects for the allowed target IDs
}

func newServer() *server {
	tmpl := template.Must(template.New("index").Parse(
		tmplPhaseInfo + tmplActionPanel + tmplPlayerList + tmplEventItem + tmplLobby + indexTemplate,
	))
	return &server{
		tmpl:     tmpl,
		botDelay: 500 * time.Millisecond,
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/start", s.handleStart)
	mux.HandleFunc("/action/message", s.handleMessage)
	mux.HandleFunc("/action/vote", s.handleVote)
	mux.HandleFunc("/action/night", s.handleNightAction)
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	data := s.buildIndexDataLocked()
	s.mu.Unlock()

	if err := s.tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) buildIndexDataLocked() indexData {
	if s.game == nil {
		return indexData{}
	}

	players := make([]Player, len(s.game.Players))
	copy(players, s.game.Players)

	logLines := make([]string, len(s.eventLog))
	copy(logLines, s.eventLog)

	data := indexData{
		HasGame:        true,
		Phase:          s.game.Phase,
		DayNumber:      s.game.DayNumber,
		Winner:         s.game.Winner,
		Players:        players,
		EventLog:       logLines,
		RevealAllRoles: s.game.Phase == PhaseEnded,
		Pending:        s.game.Pending,
	}

	human := s.game.HumanPlayer()
	if human != nil {
		data.HumanRole = human.Role
	}

	// Build allowed targets as player objects for the template
	if s.game.Pending != nil {
		for _, tid := range s.game.Pending.AllowedTargetIDs {
			p, err := s.game.FindPlayer(tid)
			if err == nil {
				data.AllowedTargets = append(data.AllowedTargets, *p)
			}
		}
	}

	return data
}

func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	playerCount := 8
	if raw := r.FormValue("player_count"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "invalid player count", http.StatusBadRequest)
			return
		}
		playerCount = parsed
	}

	g, err := NewGame(playerCount, rand.New(rand.NewSource(rand.Int63())), s.newAgent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := g.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.game = g
	s.eventLog = []string{"Game started!"}
	s.eventLog = append(s.eventLog, fmt.Sprintf("You are the %s.", g.HumanPlayer().Role))
	s.game.EventLog = &s.eventLog

	if s.hub.connected() {
		// Send initial events and full state over WS, then drive async.
		s.sendFullStateLocked(r.Context())
		s.mu.Unlock()
		s.driveGameAsync()
		w.WriteHeader(http.StatusOK)
	} else {
		s.driveGameLocked()
		s.mu.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	msg := strings.TrimSpace(r.FormValue("message"))
	if msg == "" {
		http.Error(w, "message cannot be empty", http.StatusBadRequest)
		return
	}

	s.mu.Lock()

	if err := s.validatePendingLocked(PendingMessage); err != nil {
		s.mu.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	human := s.game.HumanPlayer()
	eventText := fmt.Sprintf("[%s] %s", human.Name, msg)
	s.eventLog = append(s.eventLog, eventText)
	s.game.Discussion.Index++
	s.game.Pending = nil

	if s.hub.connected() {
		s.broadcastEvent(r.Context(), eventText)
		s.mu.Unlock()
		s.driveGameAsync()
		w.WriteHeader(http.StatusOK)
	} else {
		s.driveGameLocked()
		s.mu.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) handleVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetRaw := r.FormValue("target")
	if targetRaw == "" {
		http.Error(w, "must select a target", http.StatusBadRequest)
		return
	}
	targetInt, err := strconv.Atoi(targetRaw)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	targetID := PlayerID(targetInt)

	s.mu.Lock()

	if err := s.validatePendingLocked(PendingVote); err != nil {
		s.mu.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !s.isAllowedTargetLocked(targetID) {
		s.mu.Unlock()
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	human := s.game.HumanPlayer()
	// Safe: targetID was validated by isAllowedTargetLocked above.
	target, _ := s.game.FindPlayer(targetID)
	s.game.Vote.Votes[human.ID] = targetID
	eventText := fmt.Sprintf("%s votes for %s.", human.Name, target.Name)
	s.eventLog = append(s.eventLog, eventText)
	s.game.Vote.Index++
	s.game.Pending = nil

	if s.hub.connected() {
		s.broadcastEvent(r.Context(), eventText)
		s.mu.Unlock()
		s.driveGameAsync()
		w.WriteHeader(http.StatusOK)
	} else {
		s.driveGameLocked()
		s.mu.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) handleNightAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetRaw := r.FormValue("target")
	if targetRaw == "" {
		http.Error(w, "must select a target", http.StatusBadRequest)
		return
	}
	targetInt, err := strconv.Atoi(targetRaw)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}
	targetID := PlayerID(targetInt)

	s.mu.Lock()

	pending := s.game.Pending
	if pending == nil {
		s.mu.Unlock()
		http.Error(w, "no pending action", http.StatusBadRequest)
		return
	}

	isNightAction := pending.Type == PendingNightKill ||
		pending.Type == PendingNightSave ||
		pending.Type == PendingNightInvest
	if !isNightAction {
		s.mu.Unlock()
		http.Error(w, "not a night action", http.StatusBadRequest)
		return
	}

	if !s.isAllowedTargetLocked(targetID) {
		s.mu.Unlock()
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	var investEvent string
	switch pending.Type {
	case PendingNightKill:
		s.game.Night.KillTarget = &targetID
	case PendingNightSave:
		s.game.Night.ProtectTarget = &targetID
	case PendingNightInvest:
		s.game.Night.InvestTarget = &targetID
		// Safe: targetID was validated by isAllowedTargetLocked above.
		target, _ := s.game.FindPlayer(targetID)
		isMafia := target.Role == RoleMafia
		s.game.Night.InvestResult = &isMafia
		if isMafia {
			investEvent = fmt.Sprintf("Investigation result: %s is Mafia!", target.Name)
		} else {
			investEvent = fmt.Sprintf("Investigation result: %s is Not Mafia.", target.Name)
		}
		s.eventLog = append(s.eventLog, investEvent)
		if s.game.Investigations == nil {
			s.game.Investigations = make(map[PlayerID]bool)
		}
		s.game.Investigations[targetID] = isMafia
	}

	s.game.Night.Step++
	s.game.Pending = nil

	if s.hub.connected() {
		if investEvent != "" {
			s.broadcastEvent(r.Context(), investEvent)
		}
		s.mu.Unlock()
		s.driveGameAsync()
		w.WriteHeader(http.StatusOK)
	} else {
		s.driveGameLocked()
		s.mu.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *server) validatePendingLocked(expected PendingActionType) error {
	if s.game == nil {
		return fmt.Errorf("no game in progress")
	}
	if s.game.Phase == PhaseEnded {
		return fmt.Errorf("game already ended")
	}
	if s.game.Pending == nil {
		return fmt.Errorf("no pending action")
	}
	if s.game.Pending.Type != expected {
		return fmt.Errorf("expected %s action, got %s", expected, s.game.Pending.Type)
	}
	return nil
}

func (s *server) isAllowedTargetLocked(target PlayerID) bool {
	if s.game.Pending == nil {
		return false
	}
	for _, id := range s.game.Pending.AllowedTargetIDs {
		if id == target {
			return true
		}
	}
	return false
}

// --- WebSocket handler and broadcast helpers ---

func (s *server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("ws accept error: %v", err)
		return
	}

	s.hub.setConn(c)
	log.Println("ws: client connected")

	// Send initial state snapshot so a newly connected client is up to date.
	s.mu.Lock()
	s.sendFullStateLocked(r.Context())
	s.mu.Unlock()

	// Read loop — we don't expect inbound messages, but must read to detect close.
	for {
		_, _, err := c.Read(r.Context())
		if err != nil {
			break
		}
	}
	s.hub.clearConn(c)
	log.Println("ws: client disconnected")
}

// sendFullStateLocked sends the complete current state as WS messages.
// Must be called with s.mu held.
func (s *server) sendFullStateLocked(ctx context.Context) {
	if !s.hub.connected() {
		return
	}

	if s.game == nil {
		return
	}

	data := s.buildIndexDataLocked()

	// Phase info
	if html, err := s.renderFragment("phase-info", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "phase-info", Action: "replace", HTML: html})
	}

	// Player list
	if html, err := s.renderFragment("player-list", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "player-list", Action: "replace", HTML: html})
	}

	// Event log — send all entries
	var eventHTML strings.Builder
	for _, line := range data.EventLog {
		if html, err := s.renderFragment("event-item", line); err == nil {
			eventHTML.WriteString(html)
		}
	}
	s.hub.send(ctx, wsMessage{Target: "event-log", Action: "replace", HTML: eventHTML.String()})

	// Action panel
	if html, err := s.renderFragment("action-panel", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "action-panel", Action: "replace", HTML: html})
	}

	// Show game area, hide lobby
	s.hub.send(ctx, wsMessage{Target: "lobby", Action: "hide"})
	s.hub.send(ctx, wsMessage{Target: "game-area", Action: "show", HTML: ""})
}

// renderFragment executes a named template and returns the HTML string.
func (s *server) renderFragment(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("renderFragment %s: %v", name, err)
		return "", err
	}
	return buf.String(), nil
}

// broadcastEvent sends a single event log entry to the WS client.
func (s *server) broadcastEvent(ctx context.Context, text string) {
	if html, err := s.renderFragment("event-item", text); err == nil {
		s.hub.send(ctx, wsMessage{Target: "event-log", Action: "append", HTML: html})
	}
}

// broadcastPlayersLocked sends the updated player list. Must be called with s.mu held.
func (s *server) broadcastPlayersLocked(ctx context.Context) {
	data := s.buildIndexDataLocked()
	if html, err := s.renderFragment("player-list", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "player-list", Action: "replace", HTML: html})
	}
}

// broadcastActionPanelLocked sends the updated action panel. Must be called with s.mu held.
func (s *server) broadcastActionPanelLocked(ctx context.Context) {
	data := s.buildIndexDataLocked()
	if html, err := s.renderFragment("action-panel", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "action-panel", Action: "replace", HTML: html})
	}
}

// broadcastPhaseLocked sends the updated phase info. Must be called with s.mu held.
func (s *server) broadcastPhaseLocked(ctx context.Context) {
	data := s.buildIndexDataLocked()
	if html, err := s.renderFragment("phase-info", data); err == nil {
		s.hub.send(ctx, wsMessage{Target: "phase-info", Action: "replace", HTML: html})
	}
}

// --- Game driver ---

// abortGameLocked terminates the game due to an unrecoverable error (e.g. AI
// agent failure). Must be called with s.mu held.
func (s *server) abortGameLocked(err error) {
	log.Printf("game aborted: %v", err)
	s.game.Phase = PhaseEnded
	s.eventLog = append(s.eventLog, fmt.Sprintf("Game aborted: %v", err))
}

// driveGameLocked runs the game forward from the current state until it either
// needs human input (sets g.Pending) or the game ends.
// Must be called with s.mu held. Used by tests and as fallback when no WS is connected.
func (s *server) driveGameLocked() {
	g := s.game
	if g == nil {
		return
	}

	// Safety limit to prevent infinite loops
	for steps := 0; steps < 500; steps++ {
		if g.Phase == PhaseEnded || g.Pending != nil {
			return
		}

		switch g.Phase {
		case PhaseNight:
			s.stepNightLocked()
		case PhaseDay:
			s.stepDayLocked()
		case PhaseVote:
			s.stepVoteLocked()
		case PhaseWinCheck:
			s.stepWinCheckLocked()
		default:
			log.Printf("driveGameLocked: unexpected phase %v", g.Phase)
			return
		}
	}
}

// driveGameAsync runs the game forward in a background goroutine, broadcasting
// events over WebSocket after each bot action with pacing delays.
// Must be called WITHOUT s.mu held. Safe to call multiple times — only one
// driver goroutine runs at a time.
func (s *server) driveGameAsync() {
	s.mu.Lock()
	if s.driving || s.game == nil {
		s.mu.Unlock()
		return
	}
	s.driving = true
	s.mu.Unlock()

	ctx := context.Background()

	go func() {
		defer func() {
			s.mu.Lock()
			s.driving = false
			s.mu.Unlock()
		}()

		for steps := 0; steps < 500; steps++ {
			s.mu.Lock()
			g := s.game
			if g == nil || g.Phase == PhaseEnded || g.Pending != nil {
				// Broadcast final state before exiting
				s.broadcastActionPanelLocked(ctx)
				if g != nil && g.Phase == PhaseEnded {
					s.broadcastPhaseLocked(ctx)
					s.broadcastPlayersLocked(ctx)
				}
				s.mu.Unlock()
				return
			}

			prevPhase := g.Phase
			prevLogLen := len(s.eventLog)

			switch g.Phase {
			case PhaseNight:
				s.stepNightLocked()
			case PhaseDay:
				s.stepDayLocked()
			case PhaseVote:
				s.stepVoteLocked()
			case PhaseWinCheck:
				s.stepWinCheckLocked()
			default:
				log.Printf("driveGameAsync: unexpected phase %v", g.Phase)
				s.mu.Unlock()
				return
			}

			// Broadcast any new event log entries
			for i := prevLogLen; i < len(s.eventLog); i++ {
				s.broadcastEvent(ctx, s.eventLog[i])
			}

			// Broadcast phase change if it occurred
			phaseChanged := g.Phase != prevPhase
			if phaseChanged {
				s.broadcastPhaseLocked(ctx)
				s.broadcastPlayersLocked(ctx)
			}

			// If game ended or waiting for human, broadcast and exit
			if g.Phase == PhaseEnded || g.Pending != nil {
				s.broadcastActionPanelLocked(ctx)
				// On game end, send players if not already sent by phase change
				if g.Phase == PhaseEnded && !phaseChanged {
					s.broadcastPlayersLocked(ctx)
				}
				s.mu.Unlock()
				return
			}
			s.mu.Unlock()

			// Pace bot actions so events stream in visibly
			if s.botDelay > 0 {
				time.Sleep(s.botDelay)
			}
		}
	}()
}

// stepNightLocked processes night actions one role at a time.
// Night.Step: 0=mafia kill, 1=doctor protect, 2=detective investigate, 3=resolve.
func (s *server) stepNightLocked() {
	g := s.game

	for g.Night.Step <= 3 && g.Pending == nil {
		switch g.Night.Step {
		case 0: // Mafia kill
			s.stepNightMafiaLocked()
		case 1: // Doctor protect
			s.stepNightDoctorLocked()
		case 2: // Detective investigate
			s.stepNightDetectiveLocked()
		case 3: // Resolve dawn
			s.resolveNightLocked()
			return
		}
	}
}

func (s *server) stepNightMafiaLocked() {
	g := s.game

	// Find the acting mafia member: human mafia acts if alive, else first bot mafia.
	var actor *Player
	human := g.HumanPlayer()
	if human.Alive && human.Role == RoleMafia {
		actor = human
	} else {
		for i := range g.Players {
			p := &g.Players[i]
			if p.Alive && p.Role == RoleMafia && !p.IsHuman {
				actor = p
				break
			}
		}
	}

	if actor == nil {
		// No alive mafia — skip
		g.Night.Step++
		return
	}

	if actor.IsHuman {
		g.Pending = &PendingAction{
			Type:             PendingNightKill,
			ActorID:          actor.ID,
			AllowedTargetIDs: g.NightActionTargets(*actor),
			Prompt:           "Choose a player to eliminate tonight.",
		}
		return
	}

	// Bot mafia
	target, err := actor.Agent.NightAction(g, *actor)
	if err != nil {
		s.abortGameLocked(err)
		return
	}
	g.Night.KillTarget = target
	g.Night.Step++
}

func (s *server) stepNightDoctorLocked() {
	g := s.game

	doctor := g.FindByRole(RoleDoctor)
	if doctor == nil {
		g.Night.Step++
		return
	}

	if doctor.IsHuman {
		g.Pending = &PendingAction{
			Type:             PendingNightSave,
			ActorID:          doctor.ID,
			AllowedTargetIDs: g.NightActionTargets(*doctor),
			Prompt:           "Choose a player to protect tonight.",
		}
		return
	}

	// Bot doctor
	target, err := doctor.Agent.NightAction(g, *doctor)
	if err != nil {
		s.abortGameLocked(err)
		return
	}
	g.Night.ProtectTarget = target
	g.Night.Step++
}

func (s *server) stepNightDetectiveLocked() {
	g := s.game

	detective := g.FindByRole(RoleDetective)
	if detective == nil {
		g.Night.Step++
		return
	}

	if detective.IsHuman {
		g.Pending = &PendingAction{
			Type:             PendingNightInvest,
			ActorID:          detective.ID,
			AllowedTargetIDs: g.NightActionTargets(*detective),
			Prompt:           "Choose a player to investigate.",
		}
		return
	}

	// Bot detective
	target, err := detective.Agent.NightAction(g, *detective)
	if err != nil {
		s.abortGameLocked(err)
		return
	}
	if target != nil {
		g.Night.InvestTarget = target
		// Safe: Agent.NightAction only returns alive player IDs from g.Players.
		investP, _ := g.FindPlayer(*target)
		isMafia := investP.Role == RoleMafia
		g.Night.InvestResult = &isMafia
		if g.Investigations == nil {
			g.Investigations = make(map[PlayerID]bool)
		}
		g.Investigations[*target] = isMafia
	}
	g.Night.Step++
}

func (s *server) resolveNightLocked() {
	g := s.game

	if g.Night.KillTarget != nil {
		saved := g.Night.ProtectTarget != nil && *g.Night.ProtectTarget == *g.Night.KillTarget
		if saved {
			s.eventLog = append(s.eventLog, fmt.Sprintf("Night %d: The doctor saved someone!", g.DayNumber))
		} else {
			// Safe: KillTarget is set by mafia step from valid alive player IDs.
			target, _ := g.FindPlayer(*g.Night.KillTarget)
			if err := g.Eliminate(*g.Night.KillTarget, CauseNightKill); err == nil {
				s.eventLog = append(s.eventLog, fmt.Sprintf("Night %d: %s was killed! (was %s)", g.DayNumber, target.Name, target.Role))
			}
		}
	} else {
		s.eventLog = append(s.eventLog, fmt.Sprintf("Night %d: No one was killed.", g.DayNumber))
	}

	// Update doctor tracking
	g.DoctorLastProtected = g.Night.ProtectTarget

	// Check for win after night kill
	if winner, won := g.CheckWin(); won {
		g.Winner = winner
		g.Phase = PhaseEnded
		s.eventLog = append(s.eventLog, fmt.Sprintf("Game over! %s wins!", g.Winner))
		return
	}

	// Reset night state and advance to day
	g.Night = NightState{}
	if err := g.AdvancePhase(); err != nil {
		log.Printf("resolveNightLocked: AdvancePhase error: %v", err)
		return
	}

	// Initialize discussion state
	alive := g.AlivePlayerIDs()
	g.Discussion = DiscussionState{Order: alive, Index: 0}
	s.eventLog = append(s.eventLog, fmt.Sprintf("--- Day %d Discussion ---", g.DayNumber))
}

func (s *server) stepDayLocked() {
	g := s.game
	disc := &g.Discussion

	for disc.Index < len(disc.Order) && g.Pending == nil {
		speakerID := disc.Order[disc.Index]
		speaker, err := g.FindPlayer(speakerID)
		if err != nil || !speaker.Alive {
			disc.Index++
			continue
		}

		if speaker.IsHuman {
			g.Pending = &PendingAction{
				Type:    PendingMessage,
				ActorID: speaker.ID,
				Prompt:  "It's your turn to speak. Share your thoughts with the group.",
			}
			return
		}

		// Bot speaks
		msg, err := speaker.Agent.Discuss(g, *speaker, g.DayNumber)
		if err != nil {
			s.abortGameLocked(err)
			return
		}
		s.eventLog = append(s.eventLog, fmt.Sprintf("[%s] %s", speaker.Name, msg))
		disc.Index++
	}

	if disc.Index >= len(disc.Order) {
		// Discussion done, advance to vote
		if err := g.AdvancePhase(); err != nil {
			log.Printf("stepDayLocked: AdvancePhase error: %v", err)
			return
		}

		// Initialize vote state
		alive := g.AlivePlayerIDs()
		g.Vote = VoteState{
			Votes: make(map[PlayerID]PlayerID),
			Order: alive,
			Index: 0,
		}
		s.eventLog = append(s.eventLog, fmt.Sprintf("--- Day %d Vote ---", g.DayNumber))
	}
}

func (s *server) stepVoteLocked() {
	g := s.game
	vote := &g.Vote

	for vote.Index < len(vote.Order) && g.Pending == nil {
		voterID := vote.Order[vote.Index]
		voter, err := g.FindPlayer(voterID)
		if err != nil || !voter.Alive {
			vote.Index++
			continue
		}

		if voter.IsHuman {
			g.Pending = &PendingAction{
				Type:             PendingVote,
				ActorID:          voter.ID,
				AllowedTargetIDs: g.VoteTargets(*voter),
				Prompt:           "Cast your vote. Who should be eliminated?",
			}
			return
		}

		// Bot votes
		target, ok, err := voter.Agent.Vote(g, *voter)
		if err != nil {
			s.abortGameLocked(err)
			return
		}
		if ok {
			vote.Votes[voter.ID] = target
			// Safe: Agent.Vote only returns alive player IDs.
			targetP, _ := g.FindPlayer(target)
			s.eventLog = append(s.eventLog, fmt.Sprintf("%s votes for %s.", voter.Name, targetP.Name))
		}
		vote.Index++
	}

	if vote.Index >= len(vote.Order) {
		// Tally votes
		eliminated := TallyVotes(g, vote.Votes)
		if eliminated == nil {
			s.eventLog = append(s.eventLog, fmt.Sprintf("Day %d vote: Tie! No one is eliminated.", g.DayNumber))
		} else {
			// Safe: TallyVotes returns IDs from the vote map, which contains valid player IDs.
			target, _ := g.FindPlayer(*eliminated)
			if err := g.Eliminate(*eliminated, CauseVote); err == nil {
				s.eventLog = append(s.eventLog, fmt.Sprintf("Day %d vote: %s was eliminated! (was %s)", g.DayNumber, target.Name, target.Role))
			}
		}

		// Advance to win check
		if err := g.AdvancePhase(); err != nil {
			log.Printf("stepVoteLocked: AdvancePhase error: %v", err)
		}
	}
}

func (s *server) stepWinCheckLocked() {
	g := s.game

	if err := g.AdvancePhase(); err != nil {
		log.Printf("stepWinCheckLocked: AdvancePhase error: %v", err)
		return
	}

	if g.Phase == PhaseEnded {
		s.eventLog = append(s.eventLog, fmt.Sprintf("Game over! %s wins!", g.Winner))
		return
	}

	// New night — reset night state
	g.Night = NightState{}
	s.eventLog = append(s.eventLog, fmt.Sprintf("--- Night %d ---", g.DayNumber))
}

// runDeterministicGame is kept for M2 compatibility / non-interactive test usage.
func runDeterministicGame(g *Game, resolver RoundResolver, maxCycles int) ([]string, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}

	logs := []string{"Game created."}

	if g.Phase == PhaseSetup {
		if err := g.Start(); err != nil {
			return logs, err
		}
		logs = append(logs, fmt.Sprintf("Game started. Phase is now %s (Day %d).", g.Phase, g.DayNumber))
	}

	for cycle := 0; cycle < maxCycles; cycle++ {
		if g.Phase == PhaseEnded {
			logs = append(logs, fmt.Sprintf("Winner: %s.", g.Winner))
			return logs, nil
		}

		switch g.Phase {
		case PhaseNight:
			targetID, err := resolver.ResolveNight(g)
			if err != nil {
				return logs, err
			}
			if targetID != nil {
				player, err := g.FindPlayer(*targetID)
				if err != nil {
					return logs, err
				}
				if err := g.Eliminate(*targetID, CauseNightKill); err != nil {
					return logs, err
				}
				logs = append(logs, fmt.Sprintf("Night %d elimination: %s (%s).", g.DayNumber, player.Name, player.Role))

				if winner, won := g.CheckWin(); won {
					g.Winner = winner
					g.Phase = PhaseEnded
					logs = append(logs, fmt.Sprintf("Winner: %s.", g.Winner))
					return logs, nil
				}
			}
		case PhaseDay:
			logs = append(logs, fmt.Sprintf("Day %d discussion in fixed player order.", g.DayNumber))
		case PhaseVote:
			targetID, err := resolver.ResolveVote(g)
			if err != nil {
				return logs, err
			}

			if targetID == nil {
				logs = append(logs, fmt.Sprintf("Day %d vote result: tie, no elimination.", g.DayNumber))
			} else {
				player, err := g.FindPlayer(*targetID)
				if err != nil {
					return logs, err
				}
				if err := g.Eliminate(*targetID, CauseVote); err != nil {
					return logs, err
				}
				logs = append(logs, fmt.Sprintf("Day %d vote elimination: %s (%s).", g.DayNumber, player.Name, player.Role))

				if winner, won := g.CheckWin(); won {
					g.Winner = winner
					g.Phase = PhaseEnded
					logs = append(logs, fmt.Sprintf("Winner: %s.", g.Winner))
					return logs, nil
				}
			}
		}

		prev := g.Phase
		if err := g.AdvancePhase(); err != nil {
			return logs, err
		}
		if prev != g.Phase {
			logs = append(logs, fmt.Sprintf("Phase transition: %s -> %s.", prev, g.Phase))
		}
	}

	return logs, fmt.Errorf("maxCycles exceeded")
}
