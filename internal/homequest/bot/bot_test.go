package bot

import (
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
)

func TestE01ConversationAndReplay(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/bot.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<h3>Дом</h3>", "test_homequest_bot")
	if e != nil {
		t.Fatal(e)
	}
	n := int64(0)
	say := func(uid int64, text string) {
		t.Helper()
		n++
		u := Update{ID: n, Message: &Message{From: Person{ID: uid, Name: "A"}, Chat: Chat{ID: uid, Type: "private"}, Text: text}}
		if e = b.Handle(u); e != nil {
			t.Fatal(e)
		}
		if e = b.Handle(u); e != nil {
			t.Fatal(e)
		}
	}
	say(900001, "/start")
	say(900001, "/create")
	say(900001, "Тёплый дом")
	say(900001, "Europe/Berlin")
	home, e := b.Family.Current(900001)
	if e != nil || home.Name != "Тёплый дом" {
		t.Fatal(home, e)
	}
	token, e := b.Family.Invite(900001, b.Now())
	if e != nil {
		t.Fatal(e)
	}
	say(900002, "/start join_"+token)
	say(900003, "/start join_"+token)
	home, e = b.Family.Current(900002)
	if e != nil || len(home.Members) != 3 {
		t.Fatal(home, e)
	}
	say(900004, "/start")
	say(900004, "/create")
	say(900004, "Другой дом")
	say(900004, "UTC")
	var actions []Action
	db.Where("viewer = ? AND circle_id = ?", 900001, home.ID).Find(&actions)
	if len(actions) == 0 {
		t.Fatal("no bound actions")
	}
	n++
	q := Update{ID: n, Query: &Query{ID: "query", From: Person{ID: 900004}, Chat: Chat{ID: 900004, Type: "private"}, MessageID: 1, Data: actions[0].Token}}
	if e = b.Handle(q); e != nil {
		t.Fatal(e)
	}
	var rows []Delivery
	db.Order("id DESC").Limit(1).Find(&rows)
	raw, _ := json.Marshal(rows)
	if string(raw) == "[]" {
		t.Fatal("no denial delivery")
	}
	var count int64
	db.Table("circles").Count(&count)
	if count != 2 {
		t.Fatal("duplicate update side effect", count)
	}
}

func TestOldTaskActionAfterRuleChangeIsRejected(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/stale.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := New(db, "<p>Дом</p>", "test")
	_ = b.Family.Start(900001, "A")
	h, _ := b.Family.Create(900001, "Дом", "UTC")
	s := &task.Service{DB: db}
	item, _ := s.Create(900001, h.ID, "Посуда", 10, nil)
	button, e := b.button(900001, h.ID, fmt.Sprintf("done:%d", item.ID), "Выполнено")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Edit(900001, h.ID, item.ID, "Посуда", 100); e != nil {
		t.Fatal(e)
	}
	u := Update{ID: 1, Query: &Query{ID: "q", From: Person{ID: 900001}, Chat: Chat{ID: 900001, Type: "private"}, MessageID: 1, Data: button.Action}}
	if e = b.Handle(u); e != nil {
		t.Fatal(e)
	}
	balance, _ := s.Balance(900001, h.ID)
	if balance != 0 {
		t.Fatal("stale price action accepted")
	}
}

func TestDialogueBackAndRestartState(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/dialogue.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	b, _ := New(db, "<p>Дом</p>", "test")
	_ = b.Family.Start(900001, "A")
	_ = b.message(900001, "/create")
	_ = b.message(900001, "Дом")
	restarted, _ := New(db, "<p>Дом</p>", "test")
	state, _ := restarted.Family.Step(900001)
	if state.Step != "timezone" {
		t.Fatal("dialogue not durable")
	}
	if e := restarted.message(900001, "/back"); e != nil {
		t.Fatal(e)
	}
	state, _ = restarted.Family.Step(900001)
	if state.Step != "name" {
		t.Fatal("back should return to name", state.Step)
	}
	_ = restarted.message(900001, "/start")
	state, _ = restarted.Family.Step(900001)
	if state.Step != "" {
		t.Fatal("home retained hidden input mode")
	}
}
