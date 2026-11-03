// Package ai is everything to do with AI in this app.
//
// Folder map:
//
//	ai/completion.go   ChatCompletion: the interface every provider implements,
//	                   plus the message types they all share (this file)
//	ai/providers/      one struct per provider (Gemini, OpenAI, Offline) and
//	                   New(), which picks one from AI_PROVIDER in the config
//	ai/assistant/      the sales assistant built on top: the agent loop, its
//	                   tools, the prompt, the guardrails and fixed replies
//
// Go lesson: Go has no classes or inheritance. The "base class" here is an
// INTERFACE: a list of methods. Any struct that has all those methods counts
// as a ChatCompletion automatically, with no "extends" keyword. Code that
// takes a ChatCompletion works with every provider, and switching provider is
// just a different struct behind the same interface.
package ai

import "context"

// ChatCompletion is a chat model from some provider. Each provider in
// ai/providers implements all three methods.
type ChatCompletion interface {
	// Complete sends the conversation so far and returns the model's next
	// turn: text, tool calls, or both.
	Complete(ctx context.Context, req Request) (Turn, error)

	// Provider names the company behind the model, e.g. "gemini" or "openai".
	Provider() string

	// Model names the exact model, e.g. "gemini-3.5-flash-lite".
	Model() string
}

// -----------------------------------------------------------------------------
// The message types. They are provider-neutral: each provider translates them
// to and from its own JSON format, so nothing else in the app knows which
// company is behind the model.
// -----------------------------------------------------------------------------

// Who wrote a turn of the conversation, as the model sees it.
const (
	RoleUser  = "user"  // the buyer, or tool results we send back
	RoleModel = "model" // the assistant
)

// Request is everything the model needs for one step.
type Request struct {
	System  string // the rules and the fact sheet
	History []Turn
	Tools   []ToolSpec
}

// Turn is one message in the conversation.
type Turn struct {
	Role  string
	Parts []Part
}

// Part is one piece of a turn: plain text, a tool call the model asked for,
// or the result of a tool we ran.
type Part struct {
	Text   string
	Call   *ToolCall
	Result *ToolResult

	// Signature is an opaque token some models (Gemini) attach to their
	// reasoning. It must be sent back unchanged with the same part on the next
	// request. Providers that do not use it leave it empty.
	Signature []byte
}

// ToolCall is the model asking us to run one of our tools.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is what we send back after running a tool.
type ToolResult struct {
	ID       string
	Name     string
	Response map[string]any
}

// ToolSpec describes a tool to the model: its name, when to use it, and its
// parameters (all parameters are strings or numbers, which keeps it simple).
type ToolSpec struct {
	Name        string
	Description string
	Params      []Param
}

// Param is one named argument of a tool.
type Param struct {
	Name        string
	Type        string // "string" or "number"
	Description string
	Enum        []string // allowed values, for strings
	Required    bool
}

// Text joins the plain-text parts of a turn.
func (t Turn) Text() string {
	out := ""
	for _, p := range t.Parts {
		out += p.Text
	}
	return out
}

// Calls lists the tool calls in a turn.
func (t Turn) Calls() []ToolCall {
	var out []ToolCall
	for _, p := range t.Parts {
		if p.Call != nil {
			out = append(out, *p.Call)
		}
	}
	return out
}
