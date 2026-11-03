package providers_test

import (
	"testing"

	"leadflow/internal/ai/providers"
	"leadflow/internal/config"
)

// One config value picks the provider; AI_MODEL overrides its default model.
func TestNewPicksProviderFromConfig(t *testing.T) {
	cases := []struct {
		cfg          config.Config
		wantProvider string
		wantModel    string
	}{
		{config.Config{AIProvider: "gemini", GeminiAPIKey: "k"}, "gemini", providers.DefaultGeminiModel},
		{config.Config{AIProvider: "gemini", GeminiAPIKey: "k", AIModel: "gemini-3.1-flash-lite"}, "gemini", "gemini-3.1-flash-lite"},
		{config.Config{AIProvider: "openai", OpenAIAPIKey: "k"}, "openai", providers.DefaultOpenAIModel},
		{config.Config{AIProvider: "openai", OpenAIAPIKey: "k", AIModel: "gpt-4.1-nano"}, "openai", "gpt-4.1-nano"},
		{config.Config{AIProvider: "offline"}, "offline", "none"},
	}
	for _, c := range cases {
		m, err := providers.New(c.cfg)
		if err != nil {
			t.Fatalf("%s: %v", c.wantProvider, err)
		}
		if m.Provider() != c.wantProvider || m.Model() != c.wantModel {
			t.Errorf("got %s/%s, want %s/%s", m.Provider(), m.Model(), c.wantProvider, c.wantModel)
		}
	}
}

func TestNewRefusesMissingKeyAndUnknownProvider(t *testing.T) {
	for _, cfg := range []config.Config{
		{AIProvider: "gemini"},
		{AIProvider: "openai"},
		{AIProvider: "claude-but-misspelt"},
	} {
		if _, err := providers.New(cfg); err == nil {
			t.Errorf("%q: want an error", cfg.AIProvider)
		}
	}
}
