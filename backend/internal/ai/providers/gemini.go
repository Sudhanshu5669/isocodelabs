package providers

// Gemini: Google's models, reached over their REST API.
//
// It uses only Go's standard library (net/http and encoding/json). Google's
// SDK would add a very large dependency for what is a single POST request.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"leadflow/internal/ai"
)

// DefaultGeminiModel is used when AI_MODEL is empty and AI_PROVIDER=gemini.
const DefaultGeminiModel = "gemini-3.5-flash-lite"

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/"

// Gemini implements ai.ChatCompletion for Google's Gemini models.
type Gemini struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewGemini creates a Gemini client. model may be "" for DefaultGeminiModel.
func NewGemini(apiKey, model string) (*Gemini, error) {
	if apiKey == "" {
		return nil, errors.New("gemini: GEMINI_API_KEY is not set")
	}
	if model == "" {
		model = DefaultGeminiModel
	}
	return &Gemini{apiKey: apiKey, model: model, http: &http.Client{Timeout: callTimeout}}, nil
}

// Provider returns "gemini".
func (g *Gemini) Provider() string { return "gemini" }

// Model returns the Gemini model in use.
func (g *Gemini) Model() string { return g.model }

// Complete sends one request and returns the model's turn.
func (g *Gemini) Complete(ctx context.Context, req ai.Request) (ai.Turn, error) {
	body, err := EncodeGeminiRequest(req)
	if err != nil {
		return ai.Turn{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		geminiEndpoint+url.PathEscape(g.model)+":generateContent", bytes.NewReader(body))
	if err != nil {
		return ai.Turn{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey) // in a header, so the key never appears in URLs or logs

	var out geminiResponse
	status, err := postJSON(g.http, httpReq, &out)
	if err != nil {
		return ai.Turn{}, fmt.Errorf("gemini: %w", err)
	}
	if status != http.StatusOK {
		return ai.Turn{}, fmt.Errorf("gemini: status %d: %s", status, out.Error.Message)
	}
	if len(out.Candidates) == 0 {
		return ai.Turn{}, errors.New("gemini: no answer (possibly blocked by safety filters)")
	}
	return fromGemini(out.Candidates[0].Content), nil
}

// -----------------------------------------------------------------------------
// The JSON shapes Gemini's REST API uses (only the fields we need).
// -----------------------------------------------------------------------------

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	GenerationConfig  geminiGenConfig `json:"generationConfig"`
}

type geminiGenConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	Thought          bool                `json:"thought,omitempty"`
	ThoughtSignature []byte              `json:"thoughtSignature,omitempty"` // []byte is sent as base64, as Gemini expects
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  geminiSchema `json:"parameters"`
}

type geminiSchema struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
	Properties  map[string]geminiSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// -----------------------------------------------------------------------------
// Translating between our types and Gemini's
// -----------------------------------------------------------------------------

func toGemini(req ai.Request) geminiRequest {
	w := geminiRequest{
		// Low temperature: we want consistent, factual answers, not creativity.
		GenerationConfig: geminiGenConfig{Temperature: 0.3, MaxOutputTokens: 2048},
	}
	if req.System != "" {
		w.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: req.System}}}
	}

	for _, t := range req.History {
		role := "user"
		if t.Role == ai.RoleModel {
			role = "model"
		}
		c := geminiContent{Role: role}
		for _, p := range t.Parts {
			wp := geminiPart{Text: p.Text, ThoughtSignature: p.Signature}
			if p.Call != nil {
				wp.FunctionCall = &geminiFunctionCall{ID: p.Call.ID, Name: p.Call.Name, Args: p.Call.Args}
			}
			if p.Result != nil {
				wp.FunctionResponse = &geminiFunctionResp{ID: p.Result.ID, Name: p.Result.Name, Response: p.Result.Response}
			}
			c.Parts = append(c.Parts, wp)
		}
		w.Contents = append(w.Contents, c)
	}

	if len(req.Tools) > 0 {
		var decls []geminiFunctionDecl
		for _, s := range req.Tools {
			schema := geminiSchema{Type: "OBJECT", Properties: map[string]geminiSchema{}}
			for _, p := range s.Params {
				typ := "STRING"
				if p.Type == "number" {
					typ = "NUMBER"
				}
				schema.Properties[p.Name] = geminiSchema{Type: typ, Description: p.Description, Enum: p.Enum}
				if p.Required {
					schema.Required = append(schema.Required, p.Name)
				}
			}
			decls = append(decls, geminiFunctionDecl{Name: s.Name, Description: s.Description, Parameters: schema})
		}
		w.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	return w
}

// fromGemini converts Gemini's reply to our Turn. The model's private reasoning
// ("thought" parts) is dropped, but signatures stay on the part they came
// with, because Gemini requires them back on the next call.
func fromGemini(c geminiContent) ai.Turn {
	t := ai.Turn{Role: ai.RoleModel}
	for _, p := range c.Parts {
		if p.Thought && p.FunctionCall == nil {
			continue
		}
		part := ai.Part{Text: p.Text, Signature: p.ThoughtSignature}
		if fc := p.FunctionCall; fc != nil {
			part.Call = &ai.ToolCall{ID: fc.ID, Name: fc.Name, Args: fc.Args}
		}
		t.Parts = append(t.Parts, part)
	}
	return t
}

// EncodeGeminiRequest turns a request into the JSON body Gemini expects.
func EncodeGeminiRequest(req ai.Request) ([]byte, error) {
	return json.Marshal(toGemini(req))
}

// DecodeGeminiTurn reads a successful Gemini response body and returns the
// model's turn.
func DecodeGeminiTurn(raw []byte) (ai.Turn, error) {
	var out geminiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return ai.Turn{}, fmt.Errorf("gemini: unreadable response: %w", err)
	}
	if len(out.Candidates) == 0 {
		return ai.Turn{}, errors.New("gemini: no answer (possibly blocked by safety filters)")
	}
	return fromGemini(out.Candidates[0].Content), nil
}
