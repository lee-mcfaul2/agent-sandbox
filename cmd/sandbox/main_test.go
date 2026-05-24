package main

import (
	"testing"
	"time"

	"github.com/lee-mcfaul2/agent-sandbox/internal/config"
)

func TestSystemPromptEmbedded(t *testing.T) {
	if len(systemPrompt) == 0 {
		t.Fatal("systemPrompt empty")
	}
	if len(systemPrompt) > 8192 {
		t.Errorf("systemPrompt unexpectedly large: %d bytes", len(systemPrompt))
	}
}

// The HTTP-client timeout MUST equal the loop's wallclock. A shorter client
// timeout cancels an in-flight LLM call with context.DeadlineExceeded, which
// the driver mis-categorises as FinishWallclockTimeout — making slow LLM
// backends (CPU-bound Ollama) indistinguishable from the wallclock actually
// elapsing. Single source of truth = the wallclock.
func TestNewLLMClientUsesWallclockTimeout(t *testing.T) {
	cfg := &config.Config{
		LitellmURL:          "http://example.invalid",
		WallclockTimeoutSec: 1200,
	}
	c := newLLMClient(cfg)
	if got, want := c.HTTP.Timeout, 1200*time.Second; got != want {
		t.Errorf("HTTP.Timeout = %v, want %v", got, want)
	}
}
