# AI Mafia - Implementation Milestones

## M1: Core domain + state engine (completed)

- [x] Define types: `Game`, `Player`, `Role`, `Phase`
- [x] Role assignment: 1 Doctor, 1 Detective, floor(N/4) Mafia (min 1), rest
      Villagers
- [x] Player management: alive/dead tracking, role reveal on elimination
- [x] State machine transitions: setup -> night -> day -> vote -> win-check -> loop
- [x] Win condition check: mafia >= village team -> mafia wins; all mafia dead ->
      village wins
- [x] Verify: in-memory simulation can run a full game loop without UI

## M2: Vertical slice #1 - runnable deterministic game (completed)

- [x] Add `main.go` so `go run .` starts the app
- [x] Add minimal HTTP server (`net/http`) and in-memory game session
- [x] Add minimal single-page UI with: start game, phase indicator, player list,
      event log
- [x] Implement full end-to-end loop with deterministic bot actions (no LLM yet)
- [x] Show eliminations and winner in UI
- [x] Ship criteria: start game from browser and complete one full game to winner

## M3: Vertical slice #2 - full human interaction loop (completed)

- [x] Add human input endpoints for message, vote, and night action
- [x] Implement server pause/resume waiting for human turn
- [x] Add role-specific night controls (mafia kill, doctor protect, detective
      investigate, villager wait)
- [x] Implement day discussion turn order and sequential voting in fixed player
      order
- [x] Keep bots deterministic/rule-based for now
- [x] Ship criteria: human choices materially affect outcome in a full playable
      game

## M4: Vertical slice #3 - realtime event delivery (completed)

- [x] Add single WebSocket connection per client (`github.com/coder/websocket`)
- [x] Broadcast phase changes, chat messages, votes, eliminations, and win events
- [x] Push HTML fragments over WS with plain JS DOM insertion (no HTMX)
- [x] Keep gameplay fully playable end-to-end with live updates
- [x] Ship criteria: play full game without manual page refreshes

## M5: Vertical slice #4 - AI agents integrated (completed)

- [x] Add OpenCode Zen client setup (`openai-go`, custom base URL,
      `OPENCODE_ZEN_API_KEY`)
- [x] Implement role-based prompts for Mafia, Doctor, Detective, Villager
- [x] Add agent identity pool (name + personality trait)
- [x] Replace deterministic bot decisions with AI for discussion, voting, and
      night actions
- [x] Validate LLM reachability at startup; require API key to run
- [x] Enforce structured output (JSON Schema) for night action and vote decisions
- [x] Ship criteria: full game playable via `go run .` with AI-driven agents

## M6: Vertical slice #5 - streaming AI + UX polish

- [ ] Stream AI token output over WebSocket into the existing UI
- [ ] Add placeholder + incremental updates for in-progress agent responses
- [ ] Improve game screen clarity: current turn, required action, disabled
      controls when waiting
- [ ] Ensure mobile and desktop usability for full game flow
- [ ] Ship criteria: smooth full game demo with streaming AI responses

## M7: Stabilization + demo readiness

- [ ] Add comprehensive tests for transitions, role distribution, night
      resolution, vote ties, win conditions
- [ ] Add structured logging for key game events
- [ ] Cover edge cases: last-mafia elimination, doctor self-save rules,
      detective investigations
- [ ] Create a reliable demo script with expected milestone checkpoints
- [ ] Ship criteria: repeatable full-game demo with low failure risk
