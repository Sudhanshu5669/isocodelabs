package providers_test

import (
	"strings"
	"testing"

	"leadflow/internal/ai"
	"leadflow/internal/ai/providers"
)

// Like gemini_test.go: the JSON translation is checked without calling OpenAI.

func TestOpenAIRequestShape(t *testing.T) {
	req := ai.Request{
		System: "rules",
		History: []ai.Turn{
			{Role: ai.RoleUser, Parts: []ai.Part{{Text: "hi"}}},
			{Role: ai.RoleModel, Parts: []ai.Part{{Call: &ai.ToolCall{ID: "c1", Name: "get_free_slots", Args: map[string]any{}}}}},
			{Role: ai.RoleUser, Parts: []ai.Part{{Result: &ai.ToolResult{ID: "c1", Name: "get_free_slots", Response: map[string]any{"ok": true}}}}},
		},
		Tools: []ai.ToolSpec{{Name: "book_slot", Description: "d", Params: []ai.Param{
			{Name: "slot_id", Type: "string", Required: true},
			{Name: "budget", Type: "number"},
		}}},
	}
	raw, err := providers.EncodeOpenAIRequest("gpt-5-nano", req)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		`"model":"gpt-5-nano"`,
		`{"role":"system","content":"rules"}`,
		`{"role":"user","content":"hi"}`,
		`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"get_free_slots","arguments":"{}"}}]}`,
		`{"role":"tool","content":"{\"ok\":true}","tool_call_id":"c1"}`,
		`"required":["slot_id"]`,
		`"budget":{"type":"number"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request JSON missing %s\n%s", want, got)
		}
	}
}

func TestOpenAIResponseToTurn(t *testing.T) {
	turn, err := providers.DecodeOpenAITurn([]byte(`{"choices":[{"message":{"role":"assistant","content":"Booking now.",
		"tool_calls":[{"id":"call_1","type":"function","function":{"name":"book_slot","arguments":"{\"slot_id\":\"abc\"}"}}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if turn.Text() != "Booking now." {
		t.Errorf("text lost: %+v", turn)
	}
	calls := turn.Calls()
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "book_slot" || calls[0].Args["slot_id"] != "abc" {
		t.Errorf("tool call lost: %+v", calls)
	}
}

func TestOpenAIBadArgumentsIsAnError(t *testing.T) {
	_, err := providers.DecodeOpenAITurn([]byte(`{"choices":[{"message":{"role":"assistant",
		"tool_calls":[{"id":"x","type":"function","function":{"name":"book_slot","arguments":"{not json"}}]}}]}`))
	if err == nil {
		t.Error("unreadable arguments must fail the call (the assistant then falls back safely)")
	}
}
