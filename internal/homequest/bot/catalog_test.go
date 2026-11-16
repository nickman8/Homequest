package bot

import (
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"strings"
	"testing"
)

func TestCatalogsAndPremiumThroughUpdates(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/flow.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<p>Дом</p>", "test")
	if e != nil {
		t.Fatal(e)
	}
	b.BillingMode = "manual_test"
	n := int64(0)
	say := func(uid int64, text string) {
		t.Helper()
		n++
		if e = b.Handle(Update{ID: n, Message: &Message{From: Person{ID: uid, Name: "Участник"}, Chat: Chat{ID: uid, Type: "private"}, Text: text}}); e != nil {
			t.Fatal(e)
		}
	}
	click := func(uid int64, kind string) {
		t.Helper()
		var a Action
		if e = db.Where("viewer = ? AND kind = ?", uid, kind).Order("expires DESC").First(&a).Error; e != nil {
			t.Fatalf("button %s: %v", kind, e)
		}
		n++
		if e = b.Handle(Update{ID: n, Query: &Query{ID: "q", From: Person{ID: uid}, Chat: Chat{ID: uid, Type: "private"}, Data: a.Token, MessageID: 1}}); e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
	}
	say(900001, "/create")
	say(900001, "Тёплый дом")
	say(900001, "UTC")
	click(900001, "settings")
	click(900001, "invite")
	var d Delivery
	db.Order("id DESC").First(&d)
	var p map[string]any
	_ = json.Unmarshal([]byte(d.Payload), &p)
	text := p["text"].(string)
	token := strings.Split(text, "join_")[1]
	say(900002, "/start join_"+token)
	click(900001, "wishes:0")
	click(900001, "wish_new")
	say(900001, "Книга | https://example.org | Бумажная | 1000")
	click(900002, "wishes:0")
	click(900002, "wish_claim:1")
	say(900001, "/start")
	click(900001, "wishes:0")
	db.Order("id DESC").First(&d)
	if strings.Contains(d.Payload, "бронь") || strings.Contains(d.Payload, "купили") {
		t.Fatal("gift recipient disclosure")
	}
	say(900001, "/start")
	click(900001, "points")
	click(900001, "premium")
	click(900001, "premium_on")
	click(900001, "premium_stats")
	say(900001, "/start")
	click(900001, "points")
	click(900001, "rewards:0")
	click(900001, "reward_new")
	say(900001, "Фильм | 10 | Выбрать вместе")
	say(900002, "/task")
	say(900002, "Вынести мусор")
	say(900002, "10")
	click(900002, "options:1")
	click(900002, "task_manage:1")
	click(900002, "reviewers:1:0")
	say(900002, "/start")
	click(900002, "tasks")
	click(900002, "done:1")
	say(900002, "/start")
	click(900002, "points")
	click(900002, "rewards:0")
	click(900002, "reward_request:1")
	say(900001, "/start")
	click(900001, "points")
	click(900001, "rewards:0")
	click(900001, "redemptions:0")
	var nReward int64
	click(900001, "redemption:1:approve")
	db.Table("hq_badges").Where("code = ?", "reward").Count(&nReward)
	if nReward != 0 {
		t.Fatal("reward badge before delivery")
	}
	click(900001, "redemption:1:deliver")
	db.Table("hq_badges").Where("code = ?", "reward").Count(&nReward)
	if nReward != 1 {
		t.Fatal("missing delivered reward badge", nReward)
	}
	h, _ := b.Family.Current(900002)
	balance, e := (&task.Service{DB: db}).Balance(900002, h.ID)
	if e != nil || balance != 0 {
		t.Fatal(balance, e)
	}
	say(900002, "/start")
	click(900002, "points")
	click(900002, "house")
	click(900002, "history:0")
	var badgeCount int64
	db.Table("hq_badges").Where("code = ?", "first").Count(&badgeCount)
	if badgeCount != 1 {
		t.Fatal("achievement not awarded automatically", badgeCount)
	}
	if path := os.Getenv("HQ_WIDGET_CARDS_OUT"); path != "" {
		var rows []Delivery
		if e = db.Order("id").Find(&rows).Error; e != nil {
			t.Fatal(e)
		}
		cards := map[string]Card{}
		for _, row := range rows {
			var payload map[string]any
			_ = json.Unmarshal([]byte(row.Payload), &payload)
			if raw, ok := payload["chat_data"].(string); ok {
				var c Card
				_ = json.Unmarshal([]byte(raw), &c)
				cards[c.Title] = c
			}
		}
		raw, e := json.Marshal(cards)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}

}
