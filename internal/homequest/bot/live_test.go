package bot

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
)

// Real gateway + product updates + fresh SQLite; Telegram delivery is controlled.
func TestLiveGatewayCompletionThroughBot(t *testing.T) {
	if os.Getenv("HOMEQUEST_LIVE_AI") != "1" {
		t.Skip("explicit live smoke only")
	}
	raw, e := os.ReadFile(os.Getenv("HOMEQUEST_SECRETS_FILE"))
	if e != nil {
		t.Fatal("private configuration unavailable")
	}
	var cfg map[string]string
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("invalid configuration")
	}
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/live.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<p>Дом</p>", "fixture_bot")
	if e != nil {
		t.Fatal(e)
	}
	b.AIProvider = cfg["NICKLLM_BASE_URL"]
	b.LLM = &assistant.NickLLM{BaseURL: b.AIProvider, Key: cfg["NICKLLM_API_KEY"], Model: cfg["NICKLLM_MODEL"]}
	n := int64(0)
	say := func(text string) {
		t.Helper()
		n++
		if e = b.Handle(Update{ID: n, Message: &Message{From: Person{ID: 900101, Name: "A"}, Chat: Chat{ID: 900101, Type: "private"}, Text: text}}); e != nil {
			t.Fatal(e)
		}
	}
	say("/create")
	say("Тёплый дом")
	say("UTC")
	say("/task")
	say("Вынести мусор")
	say("10")
	say("Я выкинул мусор")
	var button Action
	if e = db.Where("kind = ?", "ai_consent:1").First(&button).Error; e != nil {
		t.Fatal(e)
	}
	n++
	if e = b.Handle(Update{ID: n, Query: &Query{ID: "live_fixture", From: Person{ID: 900101}, Chat: Chat{ID: 900101, Type: "private"}, Data: button.Token}}); e != nil {
		t.Fatal(e)
	}
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	h, e := b.Family.Current(900101)
	if e != nil {
		t.Fatal(e)
	}
	balance, e := (&task.Service{DB: db}).Balance(900101, h.ID)
	if e != nil || balance != 10 {
		t.Fatal("real gateway did not complete task", balance, e)
	}
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	balance, _ = (&task.Service{DB: db}).Balance(900101, h.ID)
	if balance != 10 {
		t.Fatal("duplicate credit")
	}
}
