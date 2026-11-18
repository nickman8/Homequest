package agent

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/task"
	"donetick.com/core/internal/homequest/wishlist"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
		t.Fatal("live proposal failed domain validation", e, "task version supplied", proposal.Changes[0].Version != 0)
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
		t.Fatal("live proposal failed domain validation", e, "task version supplied", proposal.Changes[0].Version != 0)
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

func TestLiveEinoWishPlan(t *testing.T) {
	engine := liveEngine(t)
	s, circle, now := setup(t)
	wishes, err := wishlist.New(s.DB)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := s.Context(alice, now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.Run(context.Background(), scope, "Добавь в мой список желаний подарок «Термос». Без ссылки, цены и заметки.", databaseAccess{s, scope, now})
	if err != nil {
		t.Fatal("live wish plan", err)
	}
	if len(p.Changes) != 1 || p.Changes[0].Operation != "wish_create" || p.Changes[0].Name != "Термос" {
		t.Fatal("unexpected wish proposal")
	}
	preview, err := s.Prepare(scope, "live-wish", p.Changes, now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := wishes.List(alice, circle)
	if err != nil || len(rows) != 0 {
		t.Fatal("preview wrote a gift")
	}
	if _, err = s.Apply(alice, preview.ID, now); err != nil {
		t.Fatal(err)
	}
	rows, err = wishes.List(alice, circle)
	if err != nil || len(rows) != 1 || rows[0].Name != "Термос" || rows[0].Claim != nil {
		t.Fatal("wish projection after confirmation")
	}
}

// Opt-in sample must be approved for the live provider and contain only the
// completion phrase. Audio stays local. The test family and balance are temporary.
func TestLiveEinoRussianAudio(t *testing.T) {
	path := os.Getenv("HQ_LIVE_RUSSIAN_AUDIO")
	if path == "" {
		t.Skip("provide approved audio of the completion phrase")
	}
	engine := liveEngine(t)
	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("audio unavailable")
	}
	stt := assistant.FasterWhisper{Python: os.Getenv("STT_PYTHON"), Script: filepath.Join("..", "..", "..", "scripts", "transcribe.py"), TempDir: t.TempDir()}
	transcript, err := stt.Transcribe(context.Background(), audio)
	if err != nil || !strings.Contains(strings.ToLower(transcript), "мусор") || !strings.Contains(strings.ToLower(transcript), "кинул") {
		t.Fatal("local STT did not recognize the requested completion")
	}
	s, circle, now := setup(t)
	ts := &task.Service{DB: s.DB}
	row, err := ts.Create(alice, circle, "Вынести мусор", 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := s.Context(alice, now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := engine.Run(context.Background(), ctx, transcript, databaseAccess{s, ctx, now})
	if err != nil || len(p.Changes) != 1 || p.Changes[0].Operation != "task_complete" || p.Changes[0].ID != row.ID || ValidateIntent(transcript, p.Changes) != nil {
		t.Fatal("recognized audio did not produce the authorized command", err)
	}
	preview, err := s.Prepare(ctx, "live-audio", p.Changes, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(alice, preview.ID, now); err != nil {
		t.Fatal(err)
	}
	if balance, err := ts.Balance(alice, circle); err != nil || balance != 10 {
		t.Fatal("missing or incorrect award")
	}
}
