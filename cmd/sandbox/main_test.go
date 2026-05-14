package main

import "testing"

func TestSystemPromptEmbedded(t *testing.T) {
	if len(systemPrompt) == 0 {
		t.Fatal("systemPrompt empty")
	}
	if len(systemPrompt) > 8192 {
		t.Errorf("systemPrompt unexpectedly large: %d bytes", len(systemPrompt))
	}
}
