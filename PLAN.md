# AI Mafia - Social Deception with Agents

## Context

Hackathon project. 4 hour time limit.

## Core Idea

Mafia (the social deception game) but with AI agents. A single human player
plays with N agents (configurable). Same classic Mafia rules - but the twist is
that a single human can play with as many agents as they please.

The intent is to give the user an "AGI feel". Being deceived by an AI feels
creepy - and that is the point. That's what makes this interesting.

## Game Rules

- Roles: Mafia, Doctor, Detective, Villager
- Role assignment is random across all players (human + agents)
- Player count: min 5, max 10. Always 1 Doctor, 1 Detective, mafia =
  floor(N/4) min 1, rest Villagers
- Night phase: Mafia kills → Doctor saves → Detective investigates (sequential)
- Day phase: Round-robin discussion (1 round), then sequential vote to eliminate
- **Voting rules**:
  - Plurality: player with most votes is eliminated. Tie = no elimination.
  - Abstain is **not allowed** — every living player must vote.
  - Self-vote is **not allowed**.
- **Turn order**: Fixed player list order (no randomization). Same order used
  for discussion and voting every day.
- Win condition: Mafia wins if they equal/outnumber village team. Village team
  wins if all Mafia are eliminated
- The code design should accommodate role extension. Eventually, new characters
  should be AI-generatable

## Technical Decisions

- **Language**: Go 1.26
- **HTTP server/router**: `net/http` (stdlib)
- **Frontend**: Go `html/template` + HTMX 2.x (CDN)
- **Transport**:
  - **WebSocket (server → client)**: Single persistent connection for pushing all
    game events (agent messages, phase changes, kill announcements, vote reveals).
    Each WS message is an HTML fragment tagged with source (agent name/ID, system).
    Uses `github.com/coder/websocket` (successor to nhooyr.io/websocket, actively
    maintained, context-aware, concurrent-write safe).
  - **HTTP (client → server)**: Human inputs only — chat messages, vote
    selections, night kill target. Simple form submissions via HTMX.
- **Real-time pattern**: Server sends HTML fragments over WebSocket. HTMX swaps
  them into the DOM via `hx-swap-oob="beforeend"` for chat append. For LLM token
  streaming, create a placeholder div then stream tokens into it via OOB swaps.
- **LLM Provider**: OpenCode Zen (`https://opencode.ai/zen/v1/chat/completions`,
  OpenAI Chat Completions compatible)
- **LLM Client**: `github.com/openai/openai-go` (official, supports custom base
  URL via `option.WithBaseURL()`)
- **API key**: Read from `OPENCODE_ZEN_API_KEY` env variable
- **No database** - all game state in memory (hackathon scope)
- **Project structure**: Flat — all Go files in root or minimal packages.

## Build Order

1. Game state engine — state machine (setup → night → day → vote → check win →
   loop), player management, role assignment.
2. HTTP server + WebSocket hub — routes for human actions, WS connection for
   pushing events.
3. HTML templates — lobby, game screen (chat log, player list, input areas per
   phase).
4. LLM agent integration — call OpenCode Zen for each agent's turn, stream
   responses over WS.
5. Wire it all together — game loop drives the server, server drives the UI.
