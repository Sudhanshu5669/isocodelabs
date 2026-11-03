package assistant_test

// THE EVAL SET: the assistant against the REAL model.
//
// Unit tests (agent_test.go) prove the code around the model is right. This
// file checks the model's behaviour with our prompt: does it stick to the
// fact sheet, hand off when a fact is missing, refuse price traps and rule
// changes, and answer in the buyer's language. Cases live in
// testdata/evals.json.
//
// It calls the real model picked by AI_PROVIDER and AI_MODEL (see
// internal/ai/providers), so it costs a little and needs that provider's key. It only runs when
// asked:
//
//	LEADFLOW_EVAL=1 go test ./tests/ai/assistant -run TestEvals -v
//
// Free-tier keys allow only a few requests a minute, so calls are paced
// (EVAL_RPM, default 4 per minute) and a rate-limited call is retried once.
//
// "model_safe" means the model's OWN reply had to pass the guardrails. The
// buyer is protected either way (a blocked reply is replaced), but a model
// that keeps needing the guard is a prompt problem worth knowing about.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/joho/godotenv"
	"golang.org/x/time/rate"

	"leadflow/internal/ai"
	"leadflow/internal/ai/assistant"
	"leadflow/internal/ai/providers"
	"leadflow/internal/config"
)

type evalCase struct {
	ID           string   `json:"id"`
	Lang         string   `json:"lang"`
	Category     string   `json:"category"`
	Message      string   `json:"message"`
	ContainsAny  []string `json:"contains_any"`
	NotContains  []string `json:"not_contains"`
	RequireTools []string `json:"require_tools"`
	ForbidTools  []string `json:"forbid_tools"`
	Script       string   `json:"script"` // latin, devanagari or gujarati
	ModelSafe    bool     `json:"model_safe"`
}

// The seeded fact sheet (see internal/seed), so evals match what buyers see.
var evalFacts = []assistant.Fact{
	{Topic: "amenities", Label: "Amenities", Value: "Clubhouse, gym, children's play area, landscaped garden and 24x7 security."},
	{Topic: "area", Label: "Apartment area", Value: "2 BHK apartments are about 1,085 to 1,150 sq ft. 3 BHK apartments are about 1,590 to 1,710 sq ft."},
	{Topic: "configurations", Label: "Apartment types", Value: "2 BHK and 3 BHK apartments."},
	{Topic: "location", Label: "Location", Value: "Located in Ahmedabad. The sales executive shares the exact location pin when you book a visit."},
	{Topic: "parking", Label: "Parking", Value: "One covered parking space with every 2 BHK and two with every 3 BHK."},
	{Topic: "possession", Label: "Possession", Value: "Expected possession: December 2027."},
	{Topic: "price_range", Label: "Indicative price range", Value: "Indicative prices across our projects run from ₹65 lakh to ₹1.2 crore. The final price, and any offer, is confirmed by the sales team."},
	{Topic: "visits", Label: "Site visits", Value: "Site visits are by appointment, in the free slots shown while booking. Each visit lasts about one hour."},
}

// recorder is a fake Actions that records which tools the model used.
type recorder struct{ used map[string]bool }

func (r *recorder) FreeSlots(_ context.Context, from time.Time, _ int) ([]assistant.Slot, error) {
	r.used[assistant.ToolFreeSlots] = true
	day := time.Date(from.Year(), from.Month(), from.Day()+2, 4, 30, 0, 0, time.UTC) // 10:00 IST
	return []assistant.Slot{
		{ID: "11111111-1111-4111-8111-111111111111", StartsAt: day, EndsAt: day.Add(time.Hour), ExecutiveName: "Rohan Mehta"},
		{ID: "22222222-2222-4222-8222-222222222222", StartsAt: day.Add(26 * time.Hour), EndsAt: day.Add(27 * time.Hour), ExecutiveName: "Aisha Khan"},
	}, nil
}
func (r *recorder) Book(context.Context, string) (assistant.Booking, error) {
	r.used[assistant.ToolBookSlot] = true
	return assistant.Booking{VisitCode: "x", StartsAt: time.Now(), ExecutiveName: "Rohan Mehta"}, nil
}
func (r *recorder) SaveProfile(context.Context, assistant.Profile) error {
	r.used[assistant.ToolSaveProfile] = true
	return nil
}
func (r *recorder) Handoff(context.Context, string) (string, error) {
	r.used[assistant.ToolHandoff] = true
	return "Rohan Mehta", nil
}
func (r *recorder) Flag(context.Context, string) error {
	r.used[assistant.ToolFlag] = true
	return nil
}

func TestEvals(t *testing.T) {
	if os.Getenv("LEADFLOW_EVAL") == "" {
		t.Skip("set LEADFLOW_EVAL=1 to run the eval set against the real model")
	}
	_ = godotenv.Load("../../../.env")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	llm, err := providers.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("model: %s / %s", llm.Provider(), llm.Model())

	raw, err := os.ReadFile("testdata/evals.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []evalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}

	rpm := 4.0
	if v, err := strconv.ParseFloat(os.Getenv("EVAL_RPM"), 64); err == nil && v > 0 {
		rpm = v
	}
	agent := &assistant.Agent{Model: &paced{llm: llm, limit: rate.NewLimiter(rate.Limit(rpm/60), 1)}}
	passed := 0
	var report strings.Builder
	for _, c := range cases {
		rec := &recorder{used: map[string]bool{}}
		start := time.Now()
		res := agent.Reply(context.Background(), assistant.Input{
			Chat: assistant.ChatContext{
				Lang: c.Lang, ProjectName: "Project 1", ReraNumber: "RERA-PLACEHOLDER-P1", BuyerName: "Asha Sharma",
				Facts: evalFacts, Now: time.Now(),
			},
			Message: c.Message,
		}, rec)

		problems := check(c, res, rec)
		status := "PASS"
		if len(problems) == 0 {
			passed++
		} else {
			status = "FAIL"
			t.Errorf("%s: %s\n  reply: %s", c.ID, strings.Join(problems, "; "), res.Reply)
		}
		fmt.Fprintf(&report, "%-4s %-22s %-12s %5.1fs  tools=%v blocked=%v\n", status, c.ID, c.Category,
			time.Since(start).Seconds(), keys(rec.used), res.Blocked)
	}
	t.Logf("\n%s\n%d of %d passed", report.String(), passed, len(cases))
}

// paced spaces out model calls and retries once after a rate-limit error.
type paced struct {
	llm   ai.ChatCompletion
	limit *rate.Limiter
}

func (p *paced) Provider() string { return p.llm.Provider() }
func (p *paced) Model() string    { return p.llm.Model() }

func (p *paced) Complete(ctx context.Context, req ai.Request) (ai.Turn, error) {
	for attempt := 0; ; attempt++ {
		if err := p.limit.Wait(ctx); err != nil {
			return ai.Turn{}, err
		}
		turn, err := p.llm.Complete(ctx, req)
		if err == nil || attempt == 1 || !strings.Contains(err.Error(), "status 429") {
			return turn, err
		}
		time.Sleep(40 * time.Second)
	}
}

func check(c evalCase, res assistant.Result, rec *recorder) []string {
	var p []string
	if res.Fallback {
		p = append(p, "model failed (fallback reply)")
	}
	if c.ModelSafe && res.Blocked {
		p = append(p, "model's own reply broke a guardrail ("+strings.Join(res.Flags, ", ")+")")
	}
	if len(c.ContainsAny) > 0 {
		found := false
		for _, s := range c.ContainsAny {
			found = found || strings.Contains(res.Reply, s)
		}
		if !found {
			p = append(p, fmt.Sprintf("reply mentions none of %v", c.ContainsAny))
		}
	}
	for _, s := range c.NotContains {
		if strings.Contains(res.Reply, s) {
			p = append(p, fmt.Sprintf("reply contains %q", s))
		}
	}
	for _, tool := range c.RequireTools {
		if !rec.used[tool] {
			p = append(p, "did not call "+tool)
		}
	}
	for _, tool := range c.ForbidTools {
		if rec.used[tool] {
			p = append(p, "called "+tool)
		}
	}
	if c.Script != "" && mainScript(res.Reply) != c.Script {
		p = append(p, fmt.Sprintf("reply is in %s, want %s", mainScript(res.Reply), c.Script))
	}
	return p
}

// mainScript says which writing system most letters of s use.
func mainScript(s string) string {
	counts := map[string]int{}
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Devanagari, r):
			counts["devanagari"]++
		case unicode.Is(unicode.Gujarati, r):
			counts["gujarati"]++
		case unicode.IsLetter(r) && r < 0x250:
			counts["latin"]++
		}
	}
	best, n := "latin", 0
	for k, v := range counts {
		if v > n {
			best, n = k, v
		}
	}
	return best
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
