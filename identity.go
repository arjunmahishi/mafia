package main

// AgentIdentity pairs a name with a personality trait for an AI agent.
type AgentIdentity struct {
	Name  string
	Trait string
}

// identityPool is the hardcoded pool of agent identities.
// At game start, N-1 are randomly sampled without replacement.
var identityPool = []AgentIdentity{
	{Name: "Vera", Trait: "paranoid and accusatory"},
	{Name: "Marcus", Trait: "calm and analytical"},
	{Name: "Dahlia", Trait: "charming but evasive"},
	{Name: "Soren", Trait: "blunt and confrontational"},
	{Name: "Ivy", Trait: "quiet and observant"},
	{Name: "Jasper", Trait: "nervously talkative"},
	{Name: "Luna", Trait: "cryptic and philosophical"},
	{Name: "Rook", Trait: "sardonic and skeptical"},
	{Name: "Ember", Trait: "fiercely loyal to early allies"},
	{Name: "Colt", Trait: "impulsive and emotional"},
	{Name: "Sage", Trait: "methodical and evidence-driven"},
	{Name: "Nyx", Trait: "playful and unpredictable"},
}

// pickIdentities randomly selects n identities from the pool without replacement.
func pickIdentities(n int, shuffle func(int, func(int, int))) []AgentIdentity {
	pool := make([]AgentIdentity, len(identityPool))
	copy(pool, identityPool)

	shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})

	if n > len(pool) {
		n = len(pool)
	}
	return pool[:n]
}
