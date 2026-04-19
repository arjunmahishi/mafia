package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

const (
	aiModel      = "claude-sonnet-4"
	aiTimeout    = 30 * time.Second
	aiMaxTokens  = 256
)

// AIAgent uses an LLM to make decisions.
type AIAgent struct {
	identity AgentIdentity
	client   *openai.Client
	history  []openai.ChatCompletionMessageParamUnion // per-agent context window
}

func NewAIAgent(identity AgentIdentity, client *openai.Client) *AIAgent {
	return &AIAgent{
		identity: identity,
		client:   client,
	}
}

func (a *AIAgent) NightAction(g *Game, player Player) (*PlayerID, error) {
	validTargets := g.NightActionTargets(player)
	if len(validTargets) == 0 {
		return nil, nil
	}

	var targetPlayers []Player
	for _, id := range validTargets {
		if p, err := g.FindPlayer(id); err == nil {
			targetPlayers = append(targetPlayers, *p)
		}
	}

	sysPrompt := systemPrompt(player, a.identity, g)
	gameState := gameContext(g)
	actionPrompt := nightActionPrompt(player, targetPlayers)

	messages := buildMessages(a.history, sysPrompt, gameState, actionPrompt)

	resp, err := a.complete(messages, true)
	if err != nil {
		return nil, fmt.Errorf("AI night action for %s: %w", player.Name, err)
	}

	targetID, err := parseTargetResponse(resp, validTargets)
	if err != nil {
		return nil, fmt.Errorf("AI night action parse for %s: %w", player.Name, err)
	}

	a.history = append(a.history, openai.UserMessage(actionPrompt))
	a.history = append(a.history, openai.AssistantMessage(resp))

	return &targetID, nil
}

func (a *AIAgent) Discuss(g *Game, player Player, dayNumber int) (string, error) {
	sysPrompt := systemPrompt(player, a.identity, g)
	gameState := gameContext(g)
	discPrompt := discussionPrompt(g.events())

	messages := buildMessages(a.history, sysPrompt, gameState, discPrompt)

	resp, err := a.complete(messages, false)
	if err != nil {
		return "", fmt.Errorf("AI discuss for %s: %w", player.Name, err)
	}

	a.history = append(a.history, openai.UserMessage(discPrompt))
	a.history = append(a.history, openai.AssistantMessage(resp))

	return resp, nil
}

func (a *AIAgent) Vote(g *Game, player Player) (PlayerID, bool, error) {
	validTargets := g.VoteTargets(player)
	if len(validTargets) == 0 {
		return 0, false, nil
	}

	var targetPlayers []Player
	for _, id := range validTargets {
		if p, err := g.FindPlayer(id); err == nil {
			targetPlayers = append(targetPlayers, *p)
		}
	}

	sysPrompt := systemPrompt(player, a.identity, g)
	gameState := gameContext(g)
	vPrompt := votePrompt(g.events(), targetPlayers)

	messages := buildMessages(a.history, sysPrompt, gameState, vPrompt)

	resp, err := a.complete(messages, true)
	if err != nil {
		return 0, false, fmt.Errorf("AI vote for %s: %w", player.Name, err)
	}

	targetID, err := parseTargetResponse(resp, validTargets)
	if err != nil {
		return 0, false, fmt.Errorf("AI vote parse for %s: %w", player.Name, err)
	}

	a.history = append(a.history, openai.UserMessage(vPrompt))
	a.history = append(a.history, openai.AssistantMessage(resp))

	return targetID, true, nil
}

// complete makes a non-streaming chat completion call.
func (a *AIAgent) complete(messages []openai.ChatCompletionMessageParamUnion, jsonMode bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	params := openai.ChatCompletionNewParams{
		Model:               aiModel,
		Messages:            messages,
		MaxCompletionTokens: openai.Int(int64(aiMaxTokens)),
		Temperature:         openai.Float(0.9),
	}

	if jsonMode {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "target_action",
					Strict: openai.Bool(true),
					Schema: targetResponseSchema,
				},
			},
		}
	}

	completion, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}

	if len(completion.Choices) == 0 {
		return "", errNoChoices
	}

	return completion.Choices[0].Message.Content, nil
}

// buildMessages constructs the full message list for a completion call.
func buildMessages(history []openai.ChatCompletionMessageParamUnion, sysPrompt, gameState, userPrompt string) []openai.ChatCompletionMessageParamUnion {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+3)
	msgs = append(msgs, openai.SystemMessage(sysPrompt))
	msgs = append(msgs, openai.UserMessage("GAME STATE:\n"+gameState))
	msgs = append(msgs, history...)
	msgs = append(msgs, openai.UserMessage(userPrompt))
	return msgs
}

// Target-filtering helpers (NightActionTargets, VoteTargets) live in engine.go.

// targetResponse is the JSON structure expected from the LLM for action decisions.
type targetResponse struct {
	TargetID int `json:"target_id"`
}

// targetResponseSchema is the JSON Schema enforced via Structured Outputs.
var targetResponseSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"target_id": map[string]any{"type": "integer"},
	},
	"required":             []string{"target_id"},
	"additionalProperties": false,
}

// parseTargetResponse extracts a valid PlayerID from the LLM's JSON response.
func parseTargetResponse(resp string, validTargets []PlayerID) (PlayerID, error) {
	var tr targetResponse
	if err := json.Unmarshal([]byte(resp), &tr); err != nil {
		return 0, err
	}

	id := PlayerID(tr.TargetID)
	for _, valid := range validTargets {
		if id == valid {
			return id, nil
		}
	}

	return 0, errInvalidTarget
}

type aiError string

func (e aiError) Error() string { return string(e) }

const (
	errNoChoices    aiError = "no choices in completion response"
	errInvalidTarget aiError = "target_id not in valid targets"
)
