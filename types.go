package main

import "math/rand"

type Role string

const (
	RoleMafia     Role = "mafia"
	RoleDoctor    Role = "doctor"
	RoleDetective Role = "detective"
	RoleVillager  Role = "villager"
)

type Phase string

const (
	PhaseSetup    Phase = "setup"
	PhaseNight    Phase = "night"
	PhaseDay      Phase = "day"
	PhaseVote     Phase = "vote"
	PhaseWinCheck Phase = "win_check"
	PhaseEnded    Phase = "ended"
)

type Winner string

const (
	WinnerNone    Winner = ""
	WinnerVillage Winner = "village"
	WinnerMafia   Winner = "mafia"
)

type EliminationCause string

const (
	CauseNightKill EliminationCause = "night_kill"
	CauseVote      EliminationCause = "vote"
)

type PendingActionType string

const (
	PendingNone        PendingActionType = ""
	PendingMessage     PendingActionType = "message"
	PendingVote        PendingActionType = "vote"
	PendingNightKill   PendingActionType = "night_kill"
	PendingNightSave   PendingActionType = "night_save"
	PendingNightInvest PendingActionType = "night_investigate"
)

// PendingAction represents an action the game is waiting for from the human.
type PendingAction struct {
	Type             PendingActionType
	ActorID          PlayerID
	AllowedTargetIDs []PlayerID // empty for message type
	Prompt           string     // displayed to the human
}

type PlayerID int

type Player struct {
	ID           PlayerID
	Name         string
	IsHuman      bool
	Role         Role
	Alive        bool
	RoleRevealed bool
	Agent        Agent // nil for human player
}

// NightState tracks the collected night actions for the current night.
type NightState struct {
	KillTarget    *PlayerID
	ProtectTarget *PlayerID
	InvestTarget  *PlayerID
	InvestResult  *bool // nil=not done, true=mafia, false=not mafia
	Step          int   // 0=mafia, 1=doctor, 2=detective, 3=resolve
}

// VoteState tracks the collected votes for the current vote phase.
type VoteState struct {
	Votes map[PlayerID]PlayerID // voter -> target
	Order []PlayerID            // fixed voting order (alive at phase start)
	Index int                   // next voter index
}

// DiscussionState tracks day discussion progression.
type DiscussionState struct {
	Order []PlayerID // fixed speaker order (alive at phase start)
	Index int        // next speaker index
}

type Game struct {
	Players        []Player
	Phase          Phase
	DayNumber      int
	Winner         Winner
	LastEliminated *PlayerID
	RNG            *rand.Rand

	// Interactive turn state (M3)
	Pending    *PendingAction
	Night      NightState
	Discussion DiscussionState
	Vote       VoteState

	// Doctor consecutive-protect tracking
	DoctorLastProtected *PlayerID

	// Detective accumulated knowledge (private to detective)
	Investigations map[PlayerID]bool // target -> true=mafia

	// EventLog stores public game events for AI agent context.
	// Managed by the server — same backing slice as server.eventLog.
	EventLog *[]string
}
