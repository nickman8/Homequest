package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/agent/agenttest"
	"donetick.com/core/internal/homequest/task"
)

func agentJourney(t *testing.T) (*journey, int, int) {
	j := newJourney(t)
	j.create(900001, "Тёплый дом", "Europe/Berlin")
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "task_due", Due: "2026-03-02 19:00"}}
	j.b.STT = fixtureSpeech{}
	j.b.VoiceSource = fixtureSpeech{}
	j.click(900001, "Домашние планы")
	j.click(900001, "Комнаты")
	j.click(900001, "Добавить комнату")
	j.send(900001, "Кухня", "")
	j.click(900001, "Дела комнаты")
	j.send(900001, "/task", "")
	j.send(900001, "Раковина", "")
	j.send(900001, "10", "")
	j.click(900001, "Параметры")
	j.click(900001, "Комната и очередь")
	j.click(900001, "Выбрать комнату")
	j.click(900001, "Выбрать")
	h, _ := j.b.Family.Current(900001)
	rows, _ := (&task.Service{DB: j.db}).List(900001, h.ID)
	return j, h.ID, rows[0].ID
}
func process(t *testing.T, j *journey) {
	t.Helper()
	if e := j.b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func confirmAgent(t *testing.T, j *journey) {
	t.Helper()
	for n := 0; n < 100; n++ {
		c := j.card(900001)
		for _, b := range c.Buttons {
			if b.Label == "Подтвердить план" {
				j.click(900001, b.Label)
				return
			}
		}
		j.click(900001, "Следующая часть")
	}
	t.Fatal("preview pagination")
}
func TestAgentEinoRoomPlanRestartAndVoiceConfirmation(t *testing.T) {
	j, c, id := agentJourney(t)
	j.send(900001, "/start", "")
	j.send(900001, "Перенеси все дела здесь на завтра в 19:00", "")
	j.click(900001, "Понятно, продолжить")
	process(t, j)
	if j.card(900001).Title != "Проверьте изменения" || !strings.Contains(j.card(900001).Text, "Кухня") {
		t.Fatal(j.card(900001))
	}
	var i task.Instance
	j.db.First(&i, id)
	if i.DueAt != nil {
		t.Fatal("preview wrote task")
	}
	var sess agent.Session
	j.db.First(&sess, "viewer=?", 900001)
	if !strings.Contains(sess.History, "Перенеси") {
		t.Fatal("user context not saved")
	}
	// Real handlers after restart, no direct success writes.
	j.restart()
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "read"}}
	confirmAgent(t, j)
	j.db.First(&i, id)
	if i.DueAt == nil || i.DueAt.Hour() != 18 {
		t.Fatal("plan not applied", i)
	}
	version := i.Version
	var p agent.Plan
	j.db.First(&p)
	var job AIJob
	j.db.First(&job)
	if p.State != "applied" || job.State != "done" {
		t.Fatal("non-atomic job completion")
	}
	if _, e := (&agent.Service{DB: j.db}).Apply(900001, p.ID, j.now); e != nil {
		t.Fatal(e)
	}
	j.db.First(&i, id)
	if i.Version != version {
		t.Fatal("replay changed version")
	}
	_ = c
	_ = version
}
func TestAgentRejectsStaleFocusForeignToolsAndStop(t *testing.T) {
	j, _, id := agentJourney(t)
	j.send(900001, "/start", "")
	j.send(900001, "Перенеси здесь на завтра", "")
	j.click(900001, "Понятно, продолжить")
	if e := (&agent.Service{DB: j.db}).Reset(900001, j.now); e != nil {
		t.Fatal(e)
	}
	process(t, j)
	var i task.Instance
	j.db.First(&i, id)
	if i.DueAt != nil {
		t.Fatal("queued old focus changed task")
	}
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "task_due", Due: "2026-03-02 19:00", ForeignID: 999999}}
	j.send(900001, "Перенеси задание", "")
	process(t, j)
	var count int64
	j.db.Model(&agent.Plan{}).Count(&count)
	if count != 0 {
		t.Fatal("foreign ID made a plan")
	}
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "task_due", Due: "2026-03-02 19:00"}}
	j.send(900001, "Перенеси задание", "")
	j.send(900001, "/stop", "")
	process(t, j)
	j.db.Model(&agent.Plan{}).Count(&count)
	if count != 0 {
		t.Fatal("stopped job made a plan")
	}
}
func TestAgentPlanExpiryAndSupersededConfirmation(t *testing.T) {
	j, _, id := agentJourney(t)
	j.send(900001, "/start", "")
	j.send(900001, "Перенеси на завтра", "")
	j.click(900001, "Понятно, продолжить")
	process(t, j)
	var old AIJob
	j.db.Order("id DESC").First(&old)
	j.send(900001, "Перенеси на послезавтра", "")
	process(t, j)
	if e := j.b.applyAgent(&old); e != nil {
		t.Fatal(e)
	}
	var i task.Instance
	j.db.First(&i, id)
	if i.DueAt != nil {
		t.Fatal("superseded plan applied")
	}
	j.now = j.now.Add(16 * time.Minute)
	process(t, j)
	j.send(900001, "Подтверждаю", "")
	process(t, j)
	j.db.First(&i, id)
	if i.DueAt != nil {
		t.Fatal("expired voice confirmation applied")
	}
}

func TestAgentNegativeCompletionAndActualVoiceConfirmation(t *testing.T) {
	j, _, id := agentJourney(t)
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "task_complete"}}
	j.send(900001, "/start", "")
	for n, phrase := range []string{"Я не помыл раковину", "Я завтра помою раковину", "Мне кажется, раковину помыли", "Надо помыть раковину"} {
		j.send(900001, phrase, "")
		if n == 0 {
			j.click(900001, "Понятно, продолжить")
		}
		process(t, j)
		var count int64
		j.db.Model(&agent.Plan{}).Where("state=?", "pending").Count(&count)
		if count != 0 {
			t.Fatalf("unsafe phrase proposed completion: %s", phrase)
		}
	}
	j.send(900001, "Я помыл раковину", "")
	process(t, j)
	var i task.Instance
	j.db.First(&i, id)
	if i.Closed {
		t.Fatal("completion before confirmation")
	}
	j.b.STT = agentSpeech{text: "Подтверждаю"}
	j.b.VoiceSource = fixtureSpeech{}
	j.seq++
	if e := j.b.Handle(Update{ID: j.seq, Message: &Message{From: Person{ID: 900001}, Chat: Chat{ID: 900001, Type: "private"}, Voice: &Voice{FileID: "fixture", Duration: 2}}}); e != nil {
		t.Fatal(e)
	}
	process(t, j)
	j.db.First(&i, id)
	if !i.Closed {
		t.Fatal("voice did not confirm bound plan")
	}
}

type agentSpeech struct{ text string }

func (a agentSpeech) Transcribe(context.Context, []byte) (string, error) { return a.text, nil }

func TestAgentClarificationCannotConfirmAnOlderPlan(t *testing.T) {
	j, _, id := agentJourney(t)
	j.send(900001, "/start", "")
	j.send(900001, "Перенеси дела на завтра", "")
	j.click(900001, "Понятно, продолжить")
	process(t, j)
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "read"}}
	j.send(900001, "Какие дела здесь?", "")
	process(t, j)
	j.send(900001, "Да", "")
	process(t, j)
	var i task.Instance
	j.db.First(&i, id)
	if i.DueAt != nil {
		t.Fatal("yes after a different request applied the older plan")
	}
}

type countedAgent struct{ calls int }

func (a *countedAgent) Run(context.Context, agent.Context, string, agent.Access) (agent.Proposal, error) {
	a.calls++
	return agent.Proposal{Reply: "Ответ"}, nil
}
func TestAgentOldConsentCannotEnableExpandedTools(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Тёплый дом", "UTC")
	engine := &countedAgent{}
	j.b.Agent = engine
	j.send(900001, "Что купить?", "")
	var job AIJob
	j.db.Order("id DESC").First(&job)
	// Simulate a queued request persisted by the previous release, not a domain result.
	j.db.Model(&job).Updates(map[string]any{"consent_scope": "agent_v1", "state": "queued"})
	process(t, j)
	if engine.calls != 0 {
		t.Fatal("old consent permitted new external context")
	}
}
