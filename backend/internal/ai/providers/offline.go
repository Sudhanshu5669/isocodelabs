package providers

import (
	"context"
	"errors"

	"leadflow/internal/ai"
)

// Offline is the "provider" used when no model is configured
// (AI_PROVIDER=offline, or the API key is missing). Every call fails, so the
// assistant answers with its fixed fallback reply and hands the buyer to a
// person. The chat keeps working; it just is not clever.
type Offline struct{}

// ErrOffline is returned by every Offline call.
var ErrOffline = errors.New("ai: no language model configured")

// Complete always fails.
func (Offline) Complete(context.Context, ai.Request) (ai.Turn, error) { return ai.Turn{}, ErrOffline }

// Provider returns "offline".
func (Offline) Provider() string { return "offline" }

// Model returns "none".
func (Offline) Model() string { return "none" }
