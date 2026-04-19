# Game Mechanics - MVP (Turn-Based)

## Setup

- **Players**: Configurable (N total = 1 human + N-1 AI agents).
  - Minimum: 5, Maximum: 10. Default: 5.
- **Roles**: Mafia, Doctor, Detective, Villager. Randomly assigned across all
  players (human included — human can be any role).
- **Role distribution**:
  - Always exactly **1 Doctor** and **1 Detective**.
  - Mafia count = `floor(N/4)`, minimum 1.
  - Remaining players are Villagers.

| Total Players | Mafia | Doctor | Detective | Villagers |
| ------------- | ----- | ------ | --------- | --------- |
| 5             | 1     | 1      | 1         | 2         |
| 6             | 1     | 1      | 1         | 3         |
| 7             | 1     | 1      | 1         | 4         |
| 8             | 2     | 1      | 1         | 4         |
| 9             | 2     | 1      | 1         | 5         |
| 10            | 2     | 1      | 1         | 6         |

## Roles

- **Mafia**: Knows who the other mafia members are. Picks a target to kill each
  night. During the day, blends in and tries to steer votes toward villagers.
- **Doctor**: Each night, picks one player to protect. If that player is the
  mafia's target, the kill is prevented. **Cannot protect the same player two
  nights in a row** (including themselves).
- **Detective**: Each night, investigates one player. Learns whether they are
  **Mafia or Not Mafia** (binary). Must use this information carefully during
  day discussion without revealing themselves too obviously.
- **Villager**: No special ability. Relies on discussion, observation, and
  voting to identify mafia.

## Game Loop

```
Setup → [Night Phase → Day Phase → Vote Phase] → repeat until win condition
```

### Night Phase

Night actions happen in this order:

1. **Mafia picks a kill target.**
   - If the human is Mafia: human always picks the target (regardless of how
     many mafia exist).
   - If the human is not Mafia: first AI mafia agent picks the target.
   - No mafia-to-mafia discussion at night (MVP simplification).

2. **Doctor picks someone to protect.**
   - If the human is the Doctor: human picks via UI.
   - If the human is not the Doctor: AI doctor picks.
   - Cannot protect the same player as the previous night.

3. **Detective investigates one player.**
   - If the human is the Detective: human picks a player to investigate.
     Result ("Mafia" / "Not Mafia") is shown immediately during the night
     phase.
   - If the human is not the Detective: AI detective investigates.

4. **Dawn — resolve the night.**
   - If the Doctor protected the mafia's target → no one dies. Announce:
     "The doctor saved someone last night!"
   - Otherwise → target is eliminated, role revealed.

- **First night has a kill** — Day 1 always starts with information to discuss.
- If the human has no night action (they're a Villager), they see
  "Night falls... waiting."

### Day Phase (Discussion)

- **Round-robin**: each living player speaks once, in a fixed order (randomized
  at game start).
- AI agents generate their message based on full game history (deaths, past
  accusations, votes, their secret role, and any private information from their
  night actions).
- Human types their message when it's their turn.
- Everyone sees all messages in sequence.
- **1 round of discussion** before voting.

### Vote Phase

- **Sequential voting**: each player casts a vote to eliminate someone (or
  abstain).
- AI agents vote one by one, votes revealed as they come in.
- Human votes in their position in the order.
- **Majority rules**: player with most votes is eliminated. Ties = no
  elimination.

### Elimination

- When a player is eliminated (night kill or vote), their **role is revealed**.

## Win Conditions

- **Village team wins** (Villager + Doctor + Detective): all Mafia are
  eliminated.
- **Mafia wins**: Mafia count ≥ village team count (among living players).

## AI Agent Behavior

- **Villager agents**: Observe, accuse, defend. Try to identify mafia through
  discussion patterns and voting history.
- **Mafia agents**: Blend in, deflect suspicion, subtly steer votes toward
  villagers.
- **Doctor agents**: Protect strategically. May hint at saves without revealing
  role.
- **Detective agents**: Use investigation results to guide discussion. Must
  balance revealing info vs. self-preservation (mafia will target a known
  detective).
- Each agent receives full public game history as LLM context (messages, votes,
  eliminations). Mafia agents additionally know who the other mafia members are.
  Doctor knows their save history. Detective knows their investigation results.

### Identity

- Each agent has a **name and personality trait** drawn from a hardcoded pool
  (10-15 entries). Randomly assigned at game start.
- The human player is always displayed as **"You"** — no name input needed.

### Prompting

- Minimal system prompt per role for MVP. Trust the prompt — no output
  filtering or guardrails against role leaks.

## UX Flow (Single Page)

1. **Lobby**: Pick player count, start game.
2. **Game screen**: Phase indicator, player list (alive/dead), chat log, input
   area (active on human's turn). Night phase shows role-specific UI (target
   picker for Mafia, protect picker for Doctor, investigate picker for
   Detective, waiting screen for Villager).
3. **End screen**: Winner announcement + role reveal for all players.
