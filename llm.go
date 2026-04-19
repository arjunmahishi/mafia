package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const zenBaseURL = "https://opencode.ai/zen/v1"

// NewLLMClient creates an OpenAI client pointed at OpenCode Zen and validates
// connectivity with a lightweight completion call.
// Returns an error if OPENCODE_ZEN_API_KEY is not set or the API is unreachable.
func NewLLMClient() (*openai.Client, error) {
	key := os.Getenv("OPENCODE_ZEN_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENCODE_ZEN_API_KEY is not set")
	}

	client := openai.NewClient(
		option.WithBaseURL(zenBaseURL),
		option.WithAPIKey(key),
	)

	if err := validateLLM(&client); err != nil {
		return nil, fmt.Errorf("LLM validation failed: %w", err)
	}

	return &client, nil
}

// validateLLM makes a minimal completion call to verify the API is reachable.
func validateLLM(client *openai.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:               aiModel,
		Messages:            []openai.ChatCompletionMessageParamUnion{openai.UserMessage("ping")},
		MaxCompletionTokens: openai.Int(1),
	})
	return err
}
