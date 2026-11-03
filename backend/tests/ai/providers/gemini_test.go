package providers_test

import (
	"strings"
	"testing"

	"leadflow/internal/ai"
	"leadflow/internal/ai/providers"
)

// The translation to and from Gemini's JSON is where a mistake would quietly
// break tool calling, so it is tested without calling Google.

func TestGeminiRequestShape(t *testing.T) {
	req := ai.Request{
		System: "rules",
		History: []ai.Turn{
			{Role: ai.RoleUser, Parts: []ai.Part{{Text: "hi"}}},
			{Role: ai.RoleModel, Parts: []ai.Part{{
				Call:      &ai.ToolCall{ID: "c1", Name: "get_free_slots", Args: map[string]any{}},
				Signature: []byte("sig"),
			}}},
			{Role: ai.RoleUser, Parts: []ai.Part{{
				Result: &ai.ToolResult{ID: "c1", Name: "get_free_slots", Response: map[string]any{"ok": true}},
			}}},
		},
		Tools: []ai.ToolSpec{{Name: "book_slot", Description: "d", Params: []ai.Param{
			{Name: "slot_id", Type: "string", Required: true},
			{Name: "budget", Type: "number"},
		}}},
	}
	raw, err := providers.EncodeGeminiRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		`"systemInstruction":{"parts":[{"text":"rules"}]}`,
		`"role":"model"`,
		`"functionCall":{"id":"c1","name":"get_free_slots"}`,
		`"thoughtSignature":"c2ln"`, // base64 of "sig", sent back unchanged
		`"functionResponse":{"id":"c1","name":"get_free_slots","response":{"ok":true}}`,
		`"required":["slot_id"]`,
		`"budget":{"type":"NUMBER"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request JSON missing %s\n%s", want, got)
		}
	}
}

func TestGeminiResponseDropsThoughtsKeepsSignatures(t *testing.T) {
	turn, err := providers.DecodeGeminiTurn([]byte(`{"candidates":[{"content":{"role":"model","parts":[
		{"text":"private reasoning","thought":true},
		{"functionCall":{"name":"book_slot","args":{"slot_id":"abc"}},"thoughtSignature":"c2ln"},
		{"text":"Booking now."}
	]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.Parts) != 2 {
		t.Fatalf("want the thought dropped, got %d parts", len(turn.Parts))
	}
	call := turn.Parts[0].Call
	if call == nil || call.Name != "book_slot" || call.Args["slot_id"] != "abc" || string(turn.Parts[0].Signature) != "sig" {
		t.Errorf("tool call or signature lost: %+v", turn.Parts[0])
	}
	if turn.Parts[1].Text != "Booking now." {
		t.Errorf("text lost: %+v", turn.Parts[1])
	}
}

func TestGeminiEmptyAnswerIsAnError(t *testing.T) {
	if _, err := providers.DecodeGeminiTurn([]byte(`{"candidates":[]}`)); err == nil {
		t.Error("no candidates (e.g. safety-blocked) must be an error so the assistant falls back")
	}
}
