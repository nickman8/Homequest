package assistant

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// Opt-in only; CI must never spend provider quota or use real credentials.
func TestLiveVyceAISemantics(t *testing.T) {
	if os.Getenv("HOMEQUEST_LIVE_AI") != "1" {
		t.Skip("explicit live smoke only")
	}
	raw, e := os.ReadFile(os.Getenv("HOMEQUEST_SECRETS_FILE"))
	if e != nil {
		t.Fatal("private config unavailable")
	}
	var cfg map[string]string
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("invalid config")
	}
	n := &NickLLM{BaseURL: cfg["NICKLLM_BASE_URL"], Key: cfg["NICKLLM_API_KEY"], Model: cfg["NICKLLM_MODEL"]}
	candidates := []Candidate{{ID: 1, Title: "Вынести мусор", Version: 1}, {ID: 2, Title: "Помыть посуду", Version: 1}}
	for _, tc := range []struct{ name, text, want string }{
		{"completion", "Я выкинул мусор", "complete"},
		{"negation", "Я не выкинул мусор", "no_effect"},
		{"future", "Я завтра выкину мусор", "no_effect"},
		{"intention", "Надо выкинуть мусор", "no_effect"},
		{"uncertain", "Мне кажется, мусор уже вынесли", "no_effect"},
		{"past", "Я выкинул мусор вчера", "no_effect"},
		{"multiple", "Выкинул мусор и помыл посуду", "confirm_multiple"},
		{"draft", "Напоминай выносить мусор каждый вторник в семь вечера, 10 баллов", "draft"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, e := n.Parse(context.Background(), tc.text, candidates, "Europe/Berlin")
			if e != nil {
				t.Fatalf("live parse: %v", e)
			}
			if tc.want == "no_effect" {
				if c.Action != "none" && c.Action != "clarify" {
					t.Fatal("unsafe live interpretation", c.Action)
				}
			} else if c.Action != tc.want {
				t.Fatal("unexpected action", c.Action)
			}
			if tc.want == "complete" && (len(c.IDs) != 1 || c.IDs[0] != 1) {
				t.Fatal("wrong task")
			}
			if tc.want == "draft" {
				d := c.Draft
				if d == nil || d.Points != 10 || d.Schedule.Mode != "weekdays" || d.Schedule.Hour != 19 || d.Schedule.Minute != 0 || len(d.Schedule.Days) != 1 || d.Schedule.Days[0] != "tuesday" {
					t.Fatal("incorrect live schedule")
				}
			}
		})
	}
}
