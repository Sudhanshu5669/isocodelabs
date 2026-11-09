package providers

// OpenAI: OpenAI's models, reached over the Chat Completions REST API.
//
// Many other companies copy this API exactly, so setting OPENAI_BASE_URL to
// their address makes this struct work with them too.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"leadflow/internal/ai"
)

// DefaultOpenAIModel is used when AI_MODEL is empty and AI_PROVIDER=openai.
const DefaultOpenAIModel = "gpt-6-luna"

// DefaultOpenAIBaseURL is OpenAI's own API.
const DefaultOpenAIBaseURL = "https://api.openai.com/v1"

// DefaultOpenAIReasoningEffort is used when OPENAI_REASONING_EFFORT is empty.
// "none" means the model answers straight away without thinking first: fast
// and cheap, and the fact-sheet chat does not need more. It is also required:
// newer models (gpt-6-luna, for example) refuse function tools on this API
// unless the effort is "none".
const DefaultOpenAIReasoningEffort = "none"

// OpenAI implements ai.ChatCompletion for OpenAI (and compatible) models.
type OpenAI struct {
	apiKey  string
	model   string
	effort  string // reasoning_effort to send; "" means leave it out
	baseURL string
	http    *http.Client
}

// NewOpenAI creates an OpenAI client. model, baseURL and effort may be "" for
// the defaults. effort "off" leaves reasoning_effort out of the request, for
// models that do not accept it at all (gpt-4.1-mini, for example).
func NewOpenAI(apiKey, model, baseURL, effort string) (*OpenAI, error) {
	if apiKey == "" {
		return nil, errors.New("openai: OPENAI_API_KEY is not set")
	}
	if model == "" {
		model = DefaultOpenAIModel
	}
	if baseURL == "" {
		baseURL = DefaultOpenAIBaseURL
	}
	switch effort {
	case "":
		effort = DefaultOpenAIReasoningEffort
	case "off":
		effort = ""
	}
	return &OpenAI{
		apiKey:  apiKey,
		model:   model,
		effort:  effort,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: callTimeout},
	}, nil
}

// Provider returns "openai".
func (o *OpenAI) Provider() string { return "openai" }

// Model returns the OpenAI model in use.
func (o *OpenAI) Model() string { return o.model }

// Complete sends one request and returns the model's turn.
func (o *OpenAI) Complete(ctx context.Context, req ai.Request) (ai.Turn, error) {
	body, err := EncodeOpenAIRequest(o.model, o.effort, req)
	if err != nil {
		return ai.Turn{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ai.Turn{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	var out openaiResponse
	status, err := postJSON(o.http, httpReq, &out)
	if err != nil {
		return ai.Turn{}, fmt.Errorf("openai: %w", err)
	}
	if status != http.StatusOK {
		return ai.Turn{}, fmt.Errorf("openai: status %d: %s", status, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return ai.Turn{}, errors.New("openai: no answer")
	}
	return fromOpenAI(out.Choices[0].Message)
}

// -----------------------------------------------------------------------------
// The JSON shapes of the Chat Completions API (only the fields we need).
// -----------------------------------------------------------------------------

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Tools    []openaiTool    `json:"tools,omitempty"`
	// Covers the reply AND any hidden reasoning the model does first, so it
	// is more generous than Gemini's limit.
	MaxCompletionTokens int `json:"max_completion_tokens"`
	// How much the model thinks before answering ("none", "minimal", "low"...).
	// Left out when empty.
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// Temperature is not sent: newer OpenAI models only accept the default.
}

type openaiMessage struct {
	Role       string           `json:"role"`    // system, user, assistant or tool
	Content    *string          `json:"content"` // null on an assistant message that only calls tools
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"` // on a tool message: which call it answers
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // a JSON object, sent as a string
	} `json:"function"`
}

type openaiTool struct {
	Type     string             `json:"type"` // always "function"
	Function openaiFunctionDecl `json:"function"`
}

type openaiFunctionDecl struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  openaiSchema `json:"parameters"`
}

type openaiSchema struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
	Properties  map[string]openaiSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
}

type openaiResponse struct {
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// -----------------------------------------------------------------------------
// Translating between our types and OpenAI's
// -----------------------------------------------------------------------------

func toOpenAI(model, effort string, req ai.Request) openaiRequest {
	w := openaiRequest{Model: model, MaxCompletionTokens: 4096, ReasoningEffort: effort}
	if req.System != "" {
		w.Messages = append(w.Messages, openaiMessage{Role: "system", Content: &req.System})
	}

	for _, t := range req.History {
		if t.Role == ai.RoleModel {
			w.Messages = append(w.Messages, assistantMessage(t))
			continue
		}
		// A user turn may hold tool results and/or text. OpenAI wants each tool
		// result as its own "tool" message, right after the assistant message
		// that asked for it, so results go first.
		var text strings.Builder
		for _, p := range t.Parts {
			if p.Result != nil {
				raw, _ := json.Marshal(p.Result.Response) // a map of plain values always marshals
				content := string(raw)
				w.Messages = append(w.Messages, openaiMessage{Role: "tool", ToolCallID: p.Result.ID, Content: &content})
			}
			text.WriteString(p.Text)
		}
		if text.Len() > 0 {
			s := text.String()
			w.Messages = append(w.Messages, openaiMessage{Role: "user", Content: &s})
		}
	}

	for _, s := range req.Tools {
		schema := openaiSchema{Type: "object", Properties: map[string]openaiSchema{}}
		for _, p := range s.Params {
			schema.Properties[p.Name] = openaiSchema{Type: p.Type, Description: p.Description, Enum: p.Enum}
			if p.Required {
				schema.Required = append(schema.Required, p.Name)
			}
		}
		w.Tools = append(w.Tools, openaiTool{Type: "function",
			Function: openaiFunctionDecl{Name: s.Name, Description: s.Description, Parameters: schema}})
	}
	return w
}

// assistantMessage converts one of the model's earlier turns back to OpenAI's shape.
func assistantMessage(t ai.Turn) openaiMessage {
	m := openaiMessage{Role: "assistant"}
	if text := t.Text(); text != "" {
		m.Content = &text
	}
	for _, c := range t.Calls() {
		args, _ := json.Marshal(c.Args)
		tc := openaiToolCall{ID: c.ID, Type: "function"}
		tc.Function.Name = c.Name
		tc.Function.Arguments = string(args)
		m.ToolCalls = append(m.ToolCalls, tc)
	}
	return m
}

// fromOpenAI converts OpenAI's reply to our Turn.
func fromOpenAI(m openaiMessage) (ai.Turn, error) {
	t := ai.Turn{Role: ai.RoleModel}
	if m.Content != nil && *m.Content != "" {
		t.Parts = append(t.Parts, ai.Part{Text: *m.Content})
	}
	for _, tc := range m.ToolCalls {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return ai.Turn{}, fmt.Errorf("openai: tool %s sent unreadable arguments", tc.Function.Name)
			}
		}
		t.Parts = append(t.Parts, ai.Part{Call: &ai.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args}})
	}
	return t, nil
}

// EncodeOpenAIRequest turns a request into the JSON body the Chat Completions
// API expects. effort "" leaves reasoning_effort out.
func EncodeOpenAIRequest(model, effort string, req ai.Request) ([]byte, error) {
	return json.Marshal(toOpenAI(model, effort, req))
}

// DecodeOpenAITurn reads a successful Chat Completions response body and
// returns the model's turn.
func DecodeOpenAITurn(raw []byte) (ai.Turn, error) {
	var out openaiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return ai.Turn{}, fmt.Errorf("openai: unreadable response: %w", err)
	}
	if len(out.Choices) == 0 {
		return ai.Turn{}, errors.New("openai: no answer")
	}
	return fromOpenAI(out.Choices[0].Message)
}
