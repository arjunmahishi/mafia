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
<div class="flex items-center gap-3 px-4 py-3 bg-neutral-900 border border-neutral-800 rounded-lg mb-4">
  <span class="font-display text-xl text-neutral-200 tracking-wide">{{if eq (printf "%s" .Phase) "night"}}Night{{else if eq (printf "%s" .Phase) "day"}}Day{{else if eq (printf "%s" .Phase) "vote"}}Vote{{else}}{{.Phase}}{{end}}{{if gt .DayNumber 0}} {{.DayNumber}}{{end}}</span>
  {{if .HumanRole}}
    <span class="role-badge role-{{.HumanRole}}">{{.HumanRole}}</span>
  {{end}}
</div>
{{end}}`

const tmplActionPanel = `{{define "action-panel"}}
{{if .Winner}}
  <div class="text-center py-8">
    <p class="font-display text-3xl text-neutral-200 mb-6">{{if eq (printf "%s" .Winner) "mafia"}}The Mafia wins.{{else}}The Village wins.{{end}}</p>
    <form method="post" action="/start">
      <input type="hidden" name="player_count" value="{{len .Players}}" />
      <input type="hidden" name="player_name" value="{{.HumanName}}" />
      <button type="submit" class="px-8 py-3 bg-red-600 text-white font-display text-lg tracking-wider rounded hover:bg-red-700 hover:shadow-[0_0_24px_rgba(220,38,38,0.3)] transition-all cursor-pointer">Play Again</button>
    </form>
  </div>
{{else if .Pending}}
  <div class="border border-red-600 rounded-lg p-5 bg-red-600/5">
    <h3 class="font-display text-lg text-red-500 mb-1">Your Turn</h3>
    <p class="text-sm text-neutral-400 mb-4">{{.Pending.Prompt}}</p>

    {{if eq .Pending.Type "message"}}
      <form method="post" action="/action/message">
        <textarea name="message" placeholder="Type your message..." required
          class="w-full min-h-[60px] bg-[#0a0a0a] border border-neutral-800 rounded-md text-neutral-200 font-body text-sm px-3 py-2 resize-y outline-none focus:border-red-600 mb-3"></textarea>
        <button type="submit" class="px-6 py-2 bg-red-600 text-white font-display tracking-wider rounded hover:bg-red-700 hover:shadow-[0_0_24px_rgba(220,38,38,0.3)] transition-all cursor-pointer">Send</button>
      </form>
    {{else if eq .Pending.Type "vote"}}
      <form method="post" action="/action/vote" class="flex items-center gap-3">
        <select name="target" required
          class="bg-[#0a0a0a] border border-neutral-800 rounded-md text-neutral-200 text-sm px-3 py-2 min-w-[180px] outline-none focus:border-red-600 min-h-[44px]">
          <option value="">Choose who to eliminate</option>
          {{range .AllowedTargets}}
            <option value="{{.ID}}">{{.Name}}</option>
          {{end}}
        </select>
        <button type="submit" class="px-6 py-2 bg-red-600 text-white font-display tracking-wider rounded hover:bg-red-700 hover:shadow-[0_0_24px_rgba(220,38,38,0.3)] transition-all cursor-pointer">Cast Vote</button>
      </form>
    {{else}}
      <form method="post" action="/action/night" class="flex items-center gap-3">
        <select name="target" required
          class="bg-[#0a0a0a] border border-neutral-800 rounded-md text-neutral-200 text-sm px-3 py-2 min-w-[180px] outline-none focus:border-red-600 min-h-[44px]">
          <option value="">Choose target</option>
          {{range .AllowedTargets}}
            <option value="{{.ID}}">{{.Name}}</option>
          {{end}}
        </select>
        <button type="submit" class="px-6 py-2 bg-red-600 text-white font-display tracking-wider rounded hover:bg-red-700 hover:shadow-[0_0_24px_rgba(220,38,38,0.3)] transition-all cursor-pointer">Confirm</button>
      </form>
    {{end}}
  </div>
{{else}}
  <p class="text-neutral-500 italic py-4 text-sm flex items-center gap-2"><span class="thinking-dot"></span>Waiting...</p>
{{end}}
{{end}}`

const tmplPlayerList = `{{define "player-list"}}
{{range $i, $p := .Players}}
  <div class="seat absolute flex flex-col items-center" data-seat="{{$i}}" data-player="{{$p.ID}}" data-total="{{len $.Players}}" data-human="{{$p.IsHuman}}">
    <div class="seat-icon flex flex-col items-center gap-1">
      {{if $p.IsHuman}}
      <div class="avatar w-12 h-12 rounded-full flex items-center justify-center border-2 transition-all duration-300 {{if not $p.Alive}}opacity-30 border-neutral-700{{else}}border-white bg-white/5{{end}}">
        <svg viewBox="0 0 48 48" class="w-8 h-8">
          <circle cx="24" cy="12" r="6" fill="none" stroke="white" stroke-width="2"/>
          <line x1="24" y1="18" x2="24" y2="34" stroke="white" stroke-width="2" stroke-linecap="round"/>
          <line x1="14" y1="24" x2="34" y2="24" stroke="white" stroke-width="2" stroke-linecap="round"/>
          <line x1="24" y1="34" x2="16" y2="44" stroke="white" stroke-width="2" stroke-linecap="round"/>
          <line x1="24" y1="34" x2="32" y2="44" stroke="white" stroke-width="2" stroke-linecap="round"/>
        </svg>
      </div>
      {{else}}
      <div class="avatar w-12 h-12 rounded-lg flex items-center justify-center border-2 transition-all duration-300 {{if not $p.Alive}}opacity-30 border-neutral-700{{else}}border-neutral-500 bg-neutral-800/50{{end}}">
        <svg viewBox="0 0 56 56" class="w-8 h-8 text-neutral-400">
          <line x1="20" y1="0" x2="14" y2="-10" stroke="currentColor" stroke-width="2" stroke-linecap="round" transform="translate(0,14)"/>
          <line x1="36" y1="0" x2="42" y2="-10" stroke="currentColor" stroke-width="2" stroke-linecap="round" transform="translate(0,14)"/>
          <rect x="6" y="6" width="44" height="44" rx="8" fill="none" stroke="currentColor" stroke-width="2"/>
          <rect x="14" y="18" width="10" height="12" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <rect x="32" y="18" width="10" height="12" rx="2" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <polygon points="28,36 32,42 28,48 24,42" fill="none" stroke="currentColor" stroke-width="1.5"/>
        </svg>
      </div>
      {{end}}
      <span class="text-xs font-medium max-w-[80px] truncate text-center {{if not $p.Alive}}line-through text-neutral-600{{else if $p.IsHuman}}text-white{{else}}text-neutral-300{{end}}">{{$p.Name}}</span>
      {{if not $p.Alive}}<span class="text-[10px] text-neutral-600">dead</span>{{end}}
      {{if or $p.RoleRevealed $.RevealAllRoles}}<span class="role-badge role-{{$p.Role}}">{{$p.Role}}</span>{{end}}
    </div>
  </div>
{{end}}
{{end}}`

const tmplEventItem = `{{define "event-item"}}<li>{{.}}</li>{{end}}`

const tmplLobby = `{{define "lobby"}}
<div class="flex flex-col items-center text-center">
  <h1 class="font-display text-6xl text-white tracking-wide mb-1"><span class="text-red-600">Mafia</span></h1>
  <p class="text-neutral-500 text-lg mb-10">Social deception with AI agents</p>
  <form method="post" action="/start" class="flex flex-col items-center gap-4">
    <div class="flex flex-col items-center gap-1">
      <label for="player_name" class="text-neutral-400 text-xs uppercase tracking-widest">Your Name</label>
      <input id="player_name" name="player_name" type="text" placeholder="Enter your name" required
        class="bg-neutral-900 border border-neutral-800 rounded-md text-neutral-200 text-base text-center px-4 py-2 w-56 outline-none focus:border-red-600 placeholder:text-neutral-600" />
    </div>
    <div class="flex flex-col items-center gap-1">
      <label for="player_count" class="text-neutral-400 text-xs uppercase tracking-widest">Players</label>
      <input id="player_count" name="player_count" type="number" min="5" max="10" value="8"
        class="bg-neutral-900 border border-neutral-800 rounded-md text-neutral-200 text-xl text-center px-4 py-2 w-20 outline-none focus:border-red-600" />
    </div>
    <button type="submit" class="mt-2 px-8 py-3 bg-red-600 text-white font-display text-lg tracking-wider rounded hover:bg-red-700 hover:shadow-[0_0_24px_rgba(220,38,38,0.3)] transition-all cursor-pointer">Start Game</button>
  </form>
</div>
{{end}}`

const indexTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Mafia — Social Deception with AI</title>
  <script src="https://cdn.tailwindcss.com"></script>
  <script>
    tailwind.config = {
      theme: {
        extend: {
          fontFamily: {
            display: ['"Special Elite"', 'cursive'],
            body: ['Inter', 'sans-serif'],
          },
        },
      },
    }
  </script>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Special+Elite&family=Inter:wght@300;400;500;600&display=swap" rel="stylesheet">
  <style>
    body {
      font-family: 'Inter', sans-serif;
      -webkit-font-smoothing: antialiased;
    }
    body::before {
      content: '';
      position: fixed;
      inset: 0;
      background: radial-gradient(ellipse at 50% 0%, rgba(220,38,38,0.04) 0%, transparent 60%);
      pointer-events: none;
      z-index: 0;
    }

    /* Role badges */
    .role-badge {
      display: inline-block;
      padding: 0.1rem 0.45rem;
      border-radius: 4px;
      font-size: 0.65rem;
      font-weight: 500;
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .role-mafia { background: rgba(220,38,38,0.15); color: #fca5a5; border: 1px solid rgba(220,38,38,0.3); }
    .role-doctor { background: rgba(34,197,94,0.12); color: #86efac; border: 1px solid rgba(34,197,94,0.25); }
    .role-detective { background: rgba(59,130,246,0.12); color: #93c5fd; border: 1px solid rgba(59,130,246,0.25); }
    .role-villager { background: rgba(163,163,163,0.1); color: #a3a3a3; border: 1px solid rgba(163,163,163,0.2); }

    /* Speaking highlight on seat avatar */
    .seat.speaking .avatar {
      border-color: #dc2626 !important;
      box-shadow: 0 0 16px rgba(220,38,38,0.5), 0 0 32px rgba(220,38,38,0.2);
    }

    /* Center bubble fade-in */
    #center-bubble {
      transition: opacity 0.3s ease;
    }

    /* Thinking dot animation */
    @keyframes pulse-dot {
      0%, 100% { opacity: 0.3; transform: scale(0.8); }
      50% { opacity: 1; transform: scale(1.2); }
    }
    .thinking-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #dc2626;
      animation: pulse-dot 1.2s ease-in-out infinite;
    }

    /* Scrollbar styling */
    #event-log::-webkit-scrollbar { width: 4px; }
    #event-log::-webkit-scrollbar-track { background: transparent; }
    #event-log::-webkit-scrollbar-thumb { background: #1e1e1e; border-radius: 2px; }

    /* Table surface */
    .table-surface {
      background: radial-gradient(ellipse at center, rgba(30,30,30,0.6) 0%, rgba(10,10,10,0.9) 70%);
      border: 2px solid #1e1e1e;
    }
  </style>
</head>
<body class="bg-[#0a0a0a] text-neutral-400 font-body font-light min-h-screen">

  <div id="lobby" class="relative z-10 flex items-center justify-center min-h-screen {{if .HasGame}}hidden{{end}}">
  {{if not .HasGame}}
    {{template "lobby" .}}
  {{end}}
  </div>

  <div id="game-area" class="relative z-10 max-w-4xl mx-auto px-4 py-6 {{if not .HasGame}}hidden{{end}}">
    <div class="flex items-center gap-2">
      <div id="phase-info" class="flex-1">{{if .HasGame}}{{template "phase-info" .}}{{end}}</div>
      <button id="tts-toggle" onclick="toggleTTS()" title="Toggle voice" class="w-10 h-10 flex items-center justify-center rounded-lg bg-neutral-900 border border-neutral-800 text-neutral-400 hover:text-white hover:border-neutral-600 transition-colors cursor-pointer">
        <svg id="tts-icon-on" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5">
          <path d="M11 5L6 9H2v6h4l5 4V5z" stroke-linecap="round" stroke-linejoin="round"/>
          <path d="M19.07 4.93a10 10 0 010 14.14M15.54 8.46a5 5 0 010 7.07" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <svg id="tts-icon-off" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-5 h-5 hidden">
          <path d="M11 5L6 9H2v6h4l5 4V5z" stroke-linecap="round" stroke-linejoin="round"/>
          <line x1="23" y1="9" x2="17" y2="15" stroke-linecap="round" stroke-linejoin="round"/>
          <line x1="17" y1="9" x2="23" y2="15" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </button>
    </div>

    <!-- Round table -->
    <div id="round-table" class="relative w-full mx-auto my-6" style="aspect-ratio: 1 / 1; max-width: 600px;">
      <!-- Oval table surface -->
      <div class="table-surface absolute rounded-full" style="top: 15%; left: 15%; width: 70%; height: 70%;"></div>
      <!-- Centered speech area (inside the table) -->
      <div id="center-bubble" class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-10 max-w-[280px] w-full text-center hidden">
        <span id="center-speaker" class="text-xs font-display text-red-400 uppercase tracking-wider"></span>
        <p id="center-text" class="text-sm text-neutral-300 mt-1 leading-relaxed break-words"></p>
      </div>
      <!-- Player seats positioned around the circle -->
      <div id="player-list">
        {{if .HasGame}}{{template "player-list" .}}{{end}}
      </div>
    </div>

    <div id="action-panel" class="mb-4">
      {{if .HasGame}}{{template "action-panel" .}}{{end}}
    </div>

    <div class="bg-neutral-900 border border-neutral-800 rounded-lg p-4">
      <h2 class="font-display text-sm text-neutral-500 uppercase tracking-widest mb-3">Event Log</h2>
      <ul id="event-log" class="list-none max-h-[300px] overflow-y-auto scroll-smooth space-y-0">
        {{range .EventLog}}
          <li class="py-1.5 text-sm leading-relaxed text-neutral-400 border-b border-neutral-800/50 last:border-b-0">{{.}}</li>
        {{end}}
      </ul>
    </div>
  </div>

  <script>
  // TTS toggle — global so onclick can call it
  var ttsEnabled = true;
  function toggleTTS() {
    ttsEnabled = !ttsEnabled;
    document.getElementById('tts-icon-on').classList.toggle('hidden', !ttsEnabled);
    document.getElementById('tts-icon-off').classList.toggle('hidden', ttsEnabled);
    if (!ttsEnabled) speechSynthesis.cancel();
  }

  (function() {
    var ws;
    var reconnectTimer;

    // --- Seat layout ---
    // Position seats around an ellipse. Human (index 0) always at bottom.
    function layoutSeats() {
      var seats = document.querySelectorAll('.seat');
      if (!seats.length) return;
      var total = seats.length;
      seats.forEach(function(seat, i) {
        var angle = (Math.PI / 2) + (2 * Math.PI * i / total);
        var radiusX = 46;
        var radiusY = 46;
        var cx = 50 + radiusX * Math.cos(angle);
        var cy = 50 + radiusY * Math.sin(angle);
        seat.style.left = cx + '%';
        seat.style.top = cy + '%';
        seat.style.transform = 'translate(-50%, -50%)';
      });
    }

    // --- Speaker highlight ---
    function highlightSpeaker(playerID) {
      document.querySelectorAll('.seat').forEach(function(s) {
        s.classList.remove('speaking');
      });
      if (playerID) {
        var seat = document.querySelector('.seat[data-player="' + playerID + '"]');
        if (seat) seat.classList.add('speaking');
      }
    }

    // --- Center bubble ---
    function showCenterBubble(speakerName, text) {
      var bubble = document.getElementById('center-bubble');
      var speaker = document.getElementById('center-speaker');
      var textEl = document.getElementById('center-text');
      if (!bubble) return;
      speaker.textContent = speakerName;
      textEl.textContent = text;
      bubble.classList.remove('hidden');
    }

    function hideCenterBubble() {
      var bubble = document.getElementById('center-bubble');
      if (bubble) bubble.classList.add('hidden');
    }

    // --- Text-to-Speech ---
    // Assign a slightly different voice/pitch per player for variety.
    var voiceMap = {};
    var availableVoices = [];

    function loadVoices() {
      availableVoices = speechSynthesis.getVoices().filter(function(v) {
        return v.lang.startsWith('en');
      });
    }
    if (typeof speechSynthesis !== 'undefined') {
      loadVoices();
      speechSynthesis.onvoiceschanged = loadVoices;
    }

    function speakMessage(playerID, text) {
      if (!ttsEnabled || typeof speechSynthesis === 'undefined' || !text) return;
      speechSynthesis.cancel(); // stop any current speech

      var utterance = new SpeechSynthesisUtterance(text);
      utterance.rate = 0.95;

      // Pick a consistent voice per player
      if (availableVoices.length > 0) {
        if (!voiceMap[playerID]) {
          // Hash player ID to pick a voice index
          var idx = (playerID * 7) % availableVoices.length;
          voiceMap[playerID] = availableVoices[idx];
        }
        utterance.voice = voiceMap[playerID];
      }

      // Vary pitch slightly per player (0.8 - 1.2 range)
      utterance.pitch = 0.8 + ((playerID * 3) % 5) * 0.1;
      speechSynthesis.speak(utterance);
    }

    // --- WebSocket ---
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
        } else if (msg.action === "stream") {
          el.textContent += msg.html;
          var log = document.getElementById("event-log");
          if (log) log.scrollTop = log.scrollHeight;
        } else if (msg.action === "replace") {
          el.innerHTML = msg.html;
          if (msg.target === "player-list") {
            layoutSeats();
            hideCenterBubble();
          }
        } else if (msg.action === "show") {
          el.classList.remove('hidden');
          if (msg.html) el.innerHTML = msg.html;
          if (msg.target === "game-area") layoutSeats();
        } else if (msg.action === "hide") {
          el.classList.add('hidden');
        } else if (msg.action === "speak") {
          // Show message in center bubble, highlight speaker, trigger TTS.
          // msg.target = "center-bubble", msg.html = "playerID|speakerName|text"
          var parts = msg.html.split('|');
          var pid = parts[0];
          var name = parts[1];
          var text = parts.slice(2).join('|'); // rejoin in case text has pipes
          highlightSpeaker(pid);
          showCenterBubble(name, text);
          speakMessage(parseInt(pid, 10), text);
        } else if (msg.action === "speak-start") {
          // Start streaming into center bubble. msg.html = "playerID|speakerName"
          var parts = msg.html.split('|');
          var pid = parts[0];
          var name = parts.slice(1).join('|');
          highlightSpeaker(pid);
          showCenterBubble(name, '');
        } else if (msg.action === "speak-stream") {
          // Append token to center bubble text (el = center-text)
          el.textContent += msg.html;
        } else if (msg.action === "speak-end") {
          // Streaming done. msg.html = "playerID|full text" — trigger TTS
          var parts = msg.html.split('|');
          var pid = parseInt(parts[0], 10);
          var text = parts.slice(1).join('|');
          speakMessage(pid, text);
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

    // Initial layout (script is at bottom of body, DOM is already parsed)
    layoutSeats();
    window.addEventListener("resize", layoutSeats);

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
	mu            sync.Mutex
	game          *Game
	eventLog      []string
	streamedUpTo  int           // eventLog entries up to this index were already sent via streaming
	tmpl          *template.Template
	hub           hub
	botDelay      time.Duration // delay between bot actions for pacing
	driving       bool          // true when driveGameAsync goroutine is active
	newAgent      NewAgentFunc  // factory for creating bot agents
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
	HumanName      string
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
		data.HumanName = human.Name
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

	humanName := strings.TrimSpace(r.FormValue("player_name"))

	g, err := NewGame(playerCount, humanName, rand.New(rand.NewSource(rand.Int63())), s.newAgent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := g.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	if s.driving {
		s.mu.Unlock()
		http.Error(w, "game in progress", http.StatusConflict)
		return
	}
	s.game = g
	s.eventLog = []string{"Game started!"}
	s.eventLog = append(s.eventLog, fmt.Sprintf("%s, you are the %s.", g.HumanPlayer().Name, g.HumanPlayer().Role))
	s.streamedUpTo = 0
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
		s.broadcastBubble(human.ID, human.Name, msg)
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

	// Show game area, hide lobby — add/remove 'hidden' class
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

// broadcastThinking sends a "{name} is thinking..." indicator to the action panel.
// Can be called with s.mu held (hub has its own lock).
func (s *server) broadcastThinking(name string) {
	ctx := context.Background()
	html := fmt.Sprintf(
		`<p class="text-neutral-500 italic py-4 text-sm flex items-center gap-2"><span class="thinking-dot"></span>%s is thinking...</p>`,
		template.HTMLEscapeString(name),
	)
	s.hub.send(ctx, wsMessage{Target: "action-panel", Action: "replace", HTML: html})
}

// broadcastBubble shows a player's message in the center bubble, highlights
// their seat, and triggers TTS on the client.
func (s *server) broadcastBubble(playerID PlayerID, playerName, text string) {
	ctx := context.Background()
	payload := fmt.Sprintf("%d|%s|%s",
		playerID,
		template.HTMLEscapeString(playerName),
		template.HTMLEscapeString(text),
	)
	s.hub.send(ctx, wsMessage{Target: "center-bubble", Action: "speak", HTML: payload})
}

// broadcastSpeakStart prepares the center bubble for streaming (shows speaker name, clears text).
func (s *server) broadcastSpeakStart(playerID PlayerID, playerName string) {
	ctx := context.Background()
	payload := fmt.Sprintf("%d|%s", playerID, template.HTMLEscapeString(playerName))
	s.hub.send(ctx, wsMessage{Target: "center-bubble", Action: "speak-start", HTML: payload})
}

// broadcastSpeakEnd signals that streaming is done and triggers TTS with the full message.
func (s *server) broadcastSpeakEnd(playerID PlayerID, text string) {
	ctx := context.Background()
	payload := fmt.Sprintf("%d|%s", playerID, template.HTMLEscapeString(text))
	s.hub.send(ctx, wsMessage{Target: "center-bubble", Action: "speak-end", HTML: payload})
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

			// Broadcast any new event log entries (skip those already sent via streaming)
			broadcastFrom := prevLogLen
			if s.streamedUpTo > broadcastFrom {
				broadcastFrom = s.streamedUpTo
			}
			for i := broadcastFrom; i < len(s.eventLog); i++ {
				s.broadcastEvent(ctx, s.eventLog[i])
			}
			s.streamedUpTo = 0 // reset so it doesn't affect future iterations

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

		// Check if agent supports streaming
		if sa, ok := speaker.Agent.(StreamingAgent); ok {
			s.stepDayStreamLocked(sa, speaker, disc)
			return // one streaming speaker per step; let driveGameAsync loop back
		}

		// Broadcast "thinking" indicator
		s.broadcastThinking(speaker.Name)

		// Bot speaks (non-streaming)
		msg, err := speaker.Agent.Discuss(g, *speaker, g.DayNumber)
		if err != nil {
			s.abortGameLocked(err)
			return
		}
		s.eventLog = append(s.eventLog, fmt.Sprintf("[%s] %s", speaker.Name, msg))
		s.broadcastBubble(speaker.ID, speaker.Name, msg)
		disc.Index++
	}

	s.finishDiscussionLocked()
}

// stepDayStreamLocked handles a single streaming agent's discussion turn.
// It sends a placeholder over WS, releases the lock during the streaming LLM
// call, then re-acquires the lock to update state.
func (s *server) stepDayStreamLocked(sa StreamingAgent, speaker *Player, disc *DiscussionState) {
	ctx := context.Background()

	// Send placeholder for streaming message in event log
	streamID := fmt.Sprintf("stream-%d-%d", speaker.ID, s.game.DayNumber)
	placeholder := fmt.Sprintf(
		`<li id="%s"><strong>[%s]</strong> <span id="%s-text"></span></li>`,
		streamID, template.HTMLEscapeString(speaker.Name), streamID,
	)
	s.hub.send(ctx, wsMessage{Target: "event-log", Action: "append", HTML: placeholder})

	// Prepare center bubble for streaming: highlight speaker, show empty bubble
	s.broadcastSpeakStart(speaker.ID, speaker.Name)

	// Broadcast "thinking" indicator
	s.broadcastThinking(speaker.Name)

	// Snapshot what we need, then release the lock for the LLM call.
	// Safe: s.driving is true (preventing re-entry) and handleStart rejects
	// new games while driving. The Game pointer is stable during our turn.
	gameCopy := s.game
	playerCopy := *speaker
	dayNumber := s.game.DayNumber

	s.mu.Unlock()

	// Stream tokens — onToken sends each chunk over WS (event log + center bubble)
	textTarget := streamID + "-text"
	msg, err := sa.DiscussStream(gameCopy, playerCopy, dayNumber, func(token string) {
		escaped := template.HTMLEscapeString(token)
		s.hub.send(ctx, wsMessage{
			Target: textTarget,
			Action: "stream",
			HTML:   escaped,
		})
		// Also stream to the center bubble
		s.hub.send(ctx, wsMessage{
			Target: "center-text",
			Action: "speak-stream",
			HTML:   escaped,
		})
	})

	// Re-acquire lock and update state
	s.mu.Lock()

	if err != nil {
		s.abortGameLocked(err)
		return
	}

	s.eventLog = append(s.eventLog, fmt.Sprintf("[%s] %s", speaker.Name, msg))
	s.streamedUpTo = len(s.eventLog) // mark as already broadcast via streaming
	// Send speak-end to trigger TTS with the full message
	s.broadcastSpeakEnd(speaker.ID, msg)
	disc.Index++

	s.finishDiscussionLocked()
}

// finishDiscussionLocked checks if discussion is done and advances to vote phase.
func (s *server) finishDiscussionLocked() {
	g := s.game
	disc := &g.Discussion

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

		// Broadcast "thinking" indicator
		s.broadcastThinking(voter.Name)

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
