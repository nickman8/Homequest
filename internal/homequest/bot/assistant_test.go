package bot

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/task"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

type fixtureLLM struct {
	command assistant.Command
	text    string
	ids     []assistant.Candidate
}

func (f *fixtureLLM) Parse(_ context.Context, text string, ids []assistant.Candidate, _ string) (assistant.Command, error) {
	f.text = text
	f.ids = ids
	return f.command, nil
}

type fixtureSpeech struct{}

func (fixtureSpeech) Transcribe(context.Context, []byte) (string, error) {
	return "я выкинул мусор", nil
}
func (fixtureSpeech) Voice(context.Context, string) ([]byte, error) {
	return []byte("controlled audio fixture"), nil
}
func TestE08E09AIUpdatesVoiceDraftAndReplay(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/ai.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	b, e := New(db, "<h3>Дом</h3>", "test")
	if e != nil {
		t.Fatal(e)
	}
	llm := &fixtureLLM{}
	b.LLM = llm
	b.STT = fixtureSpeech{}
	b.VoiceSource = fixtureSpeech{}
	n := int64(0)
	say := func(text string) {
		t.Helper()
		n++
		if e = b.Handle(Update{ID: n, Message: &Message{From: Person{ID: 900001, Name: "A"}, Chat: Chat{ID: 900001, Type: "private"}, Text: text}}); e != nil {
			t.Fatal(e)
		}
	}
	click := func(kind string) {
		t.Helper()
		var a Action
		if e = db.Where("viewer = ? AND kind = ?", 900001, kind).First(&a).Error; e != nil {
			t.Fatal(kind, e)
		}
		n++
		if e = b.Handle(Update{ID: n, Query: &Query{ID: "q", From: Person{ID: 900001}, Chat: Chat{ID: 900001, Type: "private"}, Data: a.Token}}); e != nil {
			t.Fatal(e)
		}
	}
	say("/create")
	say("Тёплый дом")
	say("Europe/Berlin")
	say("/task")
	say("Вынести мусор")
	say("10")
	llm.command = assistant.Command{Action: "complete", IDs: []int{1}}
	n++
	u := Update{ID: n, Message: &Message{From: Person{ID: 900001, Name: "A"}, Chat: Chat{ID: 900001, Type: "private"}, Voice: &Voice{FileID: "fixture", Duration: 2}}}
	if e = b.Handle(u); e != nil {
		t.Fatal(e)
	}
	if e = b.Handle(u); e != nil {
		t.Fatal(e)
	}
	if llm.text != "" {
		t.Fatal("provider before consent")
	}
	click("ai_consent:1")
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	h, _ := b.Family.Current(900001)
	balance, _ := (&task.Service{DB: db}).Balance(900001, h.ID)
	if balance != 10 || llm.text != "я выкинул мусор" {
		t.Fatal(balance, llm.text)
	}
	// A draft must have no domain effect until its product confirmation button.
	llm.command = assistant.Command{Action: "draft", Draft: &assistant.Draft{Title: "Полить цветы", Points: 10, Schedule: task.Schedule{Mode: "weekdays", Days: []string{"tuesday"}, Hour: 19}}}
	say("Напоминай полить цветы каждый вторник в семь вечера, 10 баллов")
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	var count int64
	db.Model(&task.Instance{}).Count(&count)
	if count != 1 {
		t.Fatal("unconfirmed draft created instance")
	}
	click("ai_confirm:2")
	db.Model(&task.Instance{}).Count(&count)
	if count != 2 {
		t.Fatal(count)
	}
	click("ai_confirm:2")
	db.Model(&task.Instance{}).Count(&count)
	if count != 2 {
		t.Fatal("duplicate draft")
	}
	var job AIJob
	db.First(&job, 2)
	if job.Text != "" || job.VoiceID != "" || job.Command != "" {
		t.Fatal("input not cleared")
	}
	// Even a malicious adapter cannot authorize a foreign ID.
	llm.command = assistant.Command{Action: "complete", IDs: []int{999}}
	say("Я полил цветы")
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	balance, _ = (&task.Service{DB: db}).Balance(900001, h.ID)
	if balance != 10 {
		t.Fatal("foreign AI ID credited")
	}
	// Negative semantic fixture returns a completion, server still refuses it.
	llm.command = assistant.Command{Action: "complete", IDs: []int{2}}
	say("Я не полил цветы")
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	balance, _ = (&task.Service{DB: db}).Balance(900001, h.ID)
	if balance != 10 {
		t.Fatal("negation credited")
	}
	b.Now = func() time.Time { return time.Now().Add(time.Hour) }
	if e = b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestConsentIsBoundToProvider(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/provider-consent.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	b, _ := New(db, "<p>Дом</p>", "test")
	b.LLM = &fixtureLLM{}
	b.AIProvider = "https://new-gateway.example/v1"
	_ = b.Family.Start(900001, "A")
	_, _ = b.Family.Create(900001, "Дом", "UTC")
	db.Create(&AIConsent{UserID: 900001, Accepted: true, Provider: "https://previous-gateway.example/v1"})
	if e := b.enqueueAI(900001, "Я выкинул мусор", ""); e != nil {
		t.Fatal(e)
	}
	var job AIJob
	db.First(&job)
	if job.State != "consent" {
		t.Fatal("old provider consent reused")
	}
}
