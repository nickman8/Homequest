package agent

import (
	"context"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"os"
	"testing"
	"time"
)

type databaseAccess struct {
	s   *Service
	c   Context
	now time.Time
}

func (a databaseAccess) Tasks(room int) ([]Task, error) { return a.s.Tasks(a.c, room, a.now) }
func TestLiveEinoRoomPlan(t *testing.T) {
	if os.Getenv("HQ_AGENT_LIVE") != "1" {
		t.Skip("opt-in live gateway; synthetic family database")
	}
	engine := liveEngine(t)
	s, c, now := setup(t)
	ts := &task.Service{DB: s.DB}
	kitchen, _ := ts.CreateRoom(alice, c, "Кухня")
	bath, _ := ts.CreateRoom(alice, c, "Ванная")
	first, _ := ts.Create(alice, c, "Помыть раковину", 10, nil)
	second, _ := ts.Create(alice, c, "Помыть раковину", 10, nil)
	if e := ts.SetRoom(alice, c, first.ID, kitchen.ID); e != nil {
		t.Fatal(e)
	}
	if e := ts.SetRoom(alice, c, second.ID, bath.ID); e != nil {
		t.Fatal(e)
	}
	scope, e := s.SelectRoom(alice, kitchen.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	proposal, e := engine.Run(context.Background(), scope, "Перенеси все задания здесь на завтра в 19:00", databaseAccess{s, scope, now})
	if e != nil {
		t.Fatal("live agent request failed; safe stage:", e.Error())
	}
	if len(proposal.Changes) != 1 || proposal.Changes[0].ID != first.ID || proposal.Changes[0].Operation != "task_due" || proposal.Changes[0].Due != "2026-11-17 19:00" {
		t.Fatal("live proposal did not match selected room/date")
	}
	p, e := s.Prepare(scope, "live-fixture", proposal.Changes, now)
	if e != nil {
		t.Fatal("live proposal failed domain validation")
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	var untouched task.Instance
	s.DB.First(&untouched, second.ID)
	if untouched.DueAt != nil {
		t.Fatal("other room changed")
	}
	t.Log("PASS real Eino/VyceAI -> room-scoped reads -> preview -> synthetic SQLite apply; no production writes; no STT")
}

func liveEngine(t *testing.T) *Eino {
	t.Helper()
	if os.Getenv("HQ_AGENT_LIVE") != "1" {
		t.Skip("opt-in live gateway; synthetic database")
	}
	cfg := map[string]string{}
	raw, e := os.ReadFile(os.Getenv("HOMEQUEST_SECRETS_FILE"))
	if e != nil || json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("private config unavailable")
	}
	get := func(k string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return cfg[k]
	}
	if get("NICKLLM_BASE_URL") != "https://vyceai.com/v1" || get("NICKLLM_MODEL") != "deepseek-4.1-flash-1017" {
		t.Fatal("verified gateway required")
	}

	engine, e := NewEino(context.Background(), get("NICKLLM_BASE_URL"), get("NICKLLM_API_KEY"), get("NICKLLM_MODEL"))
	if e != nil {
		t.Fatal("model init failed")
	}
	return engine
}
func (a databaseAccess) Resources(kind string) ([]Resource, error) {
	return a.s.Resources(a.c, kind, a.now)
}
func (a databaseAccess) Account() (Account, error) { return a.s.Account(a.c, a.now) }

func TestLiveEinoRewardPlan(t *testing.T) {
	engine := liveEngine(t)
	s, c, now := setup(t)
	rw, e := reward.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	ts := &task.Service{DB: s.DB}
	item, e := ts.Create(alice, c, "Работа", 50, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ts.Submit(alice, c, item.ID, "", now); e != nil {
		t.Fatal(e)
	}
	r, e := rw.Create(alice, c, "Кино", "Выбрать фильм", 30)
	if e != nil {
		t.Fatal(e)
	}
	scope, e := s.Context(alice, now)
	if e != nil {
		t.Fatal(e)
	}
	proposal, e := engine.Run(context.Background(), scope, "Запроси для меня награду Кино. Каталог не меняй.", databaseAccess{s, scope, now})
	if e != nil {
		t.Fatal("live reward request failed; safe stage:", e.Error())
	}
	if len(proposal.Changes) != 1 || proposal.Changes[0].Operation != "reward_request" || proposal.Changes[0].ID != r.ID || len(proposal.Changes[0].Snapshot) != 64 {
		t.Fatal("live proposal did not match request")
	}
	p, e := s.Prepare(scope, "live-reward", proposal.Changes, now)
	if e != nil {
		t.Fatal("live proposal failed domain validation")
	}
	before, _ := ts.Balance(alice, c)
	if before != 50 {
		t.Fatal("preview spent points")
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	after, _ := ts.Balance(alice, c)
	if after != 20 {
		t.Fatal("wrong spend")
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	after, _ = ts.Balance(alice, c)
	if after != 20 {
		t.Fatal("duplicate spend")
	}
	t.Log("PASS real Eino/VyceAI -> scoped reward read -> snapshot preview -> synthetic SQLite spend once; no production writes or STT")
}
