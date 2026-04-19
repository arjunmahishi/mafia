package main

import (
	"fmt"
	"strings"
)

// systemPrompt builds the system prompt for an AI agent based on its role, identity, and game state.
func systemPrompt(player Player, identity AgentIdentity, g *Game) string {
	var b strings.Builder

	b.WriteString("You are playing a game of Mafia. ")
	b.WriteString(fmt.Sprintf("Your name is %s. ", identity.Name))
	b.WriteString(fmt.Sprintf("Your personality: %s. ", identity.Trait))
	b.WriteString("Stay in character at all times.\n")
	b.WriteString("Do not include actions, gestures, or stage directions (e.g. text in asterisks like *looks around*). Only output spoken dialogue.\n\n")

	b.WriteString("RULES SUMMARY:\n")
	b.WriteString("- Players: villagers, 1 doctor, 1 detective, and mafia.\n")
	b.WriteString("- Night: mafia kills, doctor protects, detective investigates.\n")
	b.WriteString("- Day: everyone discusses, then votes to eliminate someone.\n")
	b.WriteString("- Village team wins when all mafia are dead. Mafia wins when they equal or outnumber village team.\n")
	b.WriteString("- Ties in voting mean no one is eliminated.\n\n")

	b.WriteString(fmt.Sprintf("YOUR ROLE: %s\n", player.Role))

	switch player.Role {
	case RoleMafia:
		b.WriteString("You are Mafia. Your goal is to eliminate villagers while avoiding suspicion.\n")
		b.WriteString("Blend in, deflect suspicion, and steer votes toward villagers.\n")
		// Reveal fellow mafia members
		var allies []string
		for _, p := range g.Players {
			if p.ID != player.ID && p.Role == RoleMafia && p.Alive {
				allies = append(allies, p.Name)
			}
		}
		if len(allies) > 0 {
			b.WriteString(fmt.Sprintf("Your mafia allies: %s\n", strings.Join(allies, ", ")))
		}

	case RoleDoctor:
		b.WriteString("You are the Doctor. Each night you protect one player from being killed.\n")
		b.WriteString("You cannot protect the same player two nights in a row.\n")
		b.WriteString("Be strategic about who you protect. Hint at saves without revealing your role.\n")

	case RoleDetective:
		b.WriteString("You are the Detective. Each night you investigate one player to learn if they are Mafia or Not Mafia.\n")
		b.WriteString("Use your findings to guide discussion, but be careful — if mafia figure out you're the detective, they'll target you.\n")
		// Reveal investigation results
		if len(g.Investigations) > 0 {
			b.WriteString("Your investigation results so far:\n")
			for targetID, isMafia := range g.Investigations {
				target, err := g.FindPlayer(targetID)
				if err != nil {
					continue
				}
				result := "Not Mafia"
				if isMafia {
					result = "MAFIA"
				}
				b.WriteString(fmt.Sprintf("  - %s: %s\n", target.Name, result))
			}
		}

	case RoleVillager:
		b.WriteString("You are a Villager. You have no special abilities.\n")
		b.WriteString("Use discussion, observation, and voting to identify and eliminate mafia.\n")
	}

	return b.String()
}

// gameContext builds a text summary of the current public game state.
func gameContext(g *Game) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Current: Day %d\n", g.DayNumber))

	b.WriteString("\nPlayers:\n")
	for _, p := range g.Players {
		status := "alive"
		if !p.Alive {
			status = fmt.Sprintf("dead (was %s)", p.Role)
		}
		b.WriteString(fmt.Sprintf("  - %s: %s\n", p.Name, status))
	}

	return b.String()
}

// eventHistory returns the event log as a single string for LLM context.
func eventHistory(events []string) string {
	if len(events) == 0 {
		return "No events yet."
	}
	return strings.Join(events, "\n")
}

// nightActionPrompt builds the user message for a night action decision.
func nightActionPrompt(player Player, validTargets []Player) string {
	var b strings.Builder

	switch player.Role {
	case RoleMafia:
		b.WriteString("It's night. Choose a player to KILL.\n")
	case RoleDoctor:
		b.WriteString("It's night. Choose a player to PROTECT.\n")
	case RoleDetective:
		b.WriteString("It's night. Choose a player to INVESTIGATE.\n")
	}

	b.WriteString("\nValid targets:\n")
	for _, t := range validTargets {
		b.WriteString(fmt.Sprintf("  - ID %d: %s\n", t.ID, t.Name))
	}

	b.WriteString("\nRespond with ONLY a JSON object: {\"target_id\": <number>}")

	return b.String()
}

// discussionPrompt builds the user message for a day discussion turn.
func discussionPrompt(events []string) string {
	var b strings.Builder

	b.WriteString("It's your turn to speak during the day discussion.\n")
	b.WriteString("Share your thoughts, defend yourself, or build alliances.\n")
	b.WriteString("Don't accuse someone just for the sake of creating drama. Base your suspicions on actual observations — who's been quiet, who deflected, whose behavior changed after a kill. It's okay to share thoughts without pointing fingers.\n\n")
	b.WriteString("Game events so far:\n")
	b.WriteString(eventHistory(events))
	b.WriteString("\n\nRespond in character with 1-3 sentences. Do NOT use JSON. Do not include gestures or actions in asterisks. Speak only in dialogue.")

	return b.String()
}

// votePrompt builds the user message for a vote decision.
func votePrompt(events []string, validTargets []Player) string {
	var b strings.Builder

	b.WriteString("It's time to vote. Choose who to eliminate.\n\n")
	b.WriteString("Today's events:\n")
	b.WriteString(eventHistory(events))
	b.WriteString("\n\nValid targets:\n")
	for _, t := range validTargets {
		b.WriteString(fmt.Sprintf("  - ID %d: %s\n", t.ID, t.Name))
	}

	b.WriteString("\nRespond with ONLY a JSON object: {\"target_id\": <number>}")

	return b.String()
}
