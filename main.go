package main

import (
	"log"
	"net/http"

	"github.com/openai/openai-go"
)

func main() {
	client, err := NewLLMClient()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("LLM client validated (OpenCode Zen)")

	srv := newServer()
	srv.newAgent = makeAgentFactory(client)

	addr := ":8080"
	log.Printf("mafia server listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.routes()); err != nil {
		log.Fatal(err)
	}
}

// makeAgentFactory returns a NewAgentFunc that creates AI agents.
// Returns nil if client is nil (used by tests — production always has a client).
func makeAgentFactory(client *openai.Client) NewAgentFunc {
	if client == nil {
		return nil // NewGame defaults to deterministic
	}
	return func(role Role, identity AgentIdentity) Agent {
		return NewAIAgent(identity, client)
	}
}
