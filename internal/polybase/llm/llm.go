// Package llm defines the LLM provider interface for AI-assisted queries (Phase 4 stub).
package llm

import "context"

// Provider is the interface for an LLM provider.
type Provider interface {
	// Complete sends a prompt and returns the completion.
	Complete(ctx context.Context, prompt string) (string, error)

	// Name returns the provider name.
	Name() string
}

// Config holds configuration for an LLM provider.
type Config struct {
	Provider string `json:"provider"` // "openai", "anthropic", ...
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
}

// NopProvider is a no-op provider used as a placeholder until Phase 4.
type NopProvider struct{}

func (NopProvider) Complete(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (NopProvider) Name() string { return "nop" }
