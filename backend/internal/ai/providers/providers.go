// Package providers holds one struct per AI provider. Each one implements
// every method of ai.ChatCompletion, so the rest of the app can use any of
// them without knowing which it has.
//
//	providers.go  New(): reads AI_PROVIDER and AI_MODEL and builds the right one
//	gemini.go     Gemini  (Google)
//	openai.go     OpenAI  (also works with any OpenAI-compatible API)
//	offline.go    Offline (no model: every call fails, the chat uses fixed replies)
//
// Switching provider is a config change only, for example:
//
//	AI_PROVIDER=openai
//	AI_MODEL=gpt-6-luna
//	OPENAI_API_KEY=...
//
// Adding a provider means one new file with a struct that has Complete,
// Provider and Model, plus one line in New.
package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"leadflow/internal/ai"
	"leadflow/internal/config"
)

// callTimeout bounds one model call. A slow model must not keep the buyer
// waiting: after this the assistant sends its fixed fallback reply instead.
const callTimeout = 25 * time.Second

// The compiler checks that every provider really implements ChatCompletion.
// If one is missing a method, the build fails on these lines.
var (
	_ ai.ChatCompletion = (*Gemini)(nil)
	_ ai.ChatCompletion = (*OpenAI)(nil)
	_ ai.ChatCompletion = Offline{}
)

// New builds the provider named in cfg.AIProvider, using cfg.AIModel (or that
// provider's default model when it is empty).
func New(cfg config.Config) (ai.ChatCompletion, error) {
	switch cfg.AIProvider {
	case "gemini":
		return NewGemini(cfg.GeminiAPIKey, cfg.AIModel)
	case "openai":
		return NewOpenAI(cfg.OpenAIAPIKey, cfg.AIModel, cfg.OpenAIBaseURL, cfg.OpenAIReasoningEffort)
	case "offline":
		return Offline{}, nil
	default:
		return nil, fmt.Errorf("providers: unknown AI_PROVIDER %q (use gemini, openai or offline)", cfg.AIProvider)
	}
}

// postJSON sends an already-built request and decodes the JSON answer into
// out, whatever the status code (providers put error details in the body).
// It returns the HTTP status for the caller to check.
func postJSON(client *http.Client, req *http.Request, out any) (int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return 0, fmt.Errorf("status %d, unreadable response", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
