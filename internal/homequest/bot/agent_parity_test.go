package bot

import (
	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/agent/agenttest"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestEntryPointsAgreeOnTaskCompletion(t *testing.T) {
	for _, entry := range []string{"buttons", "widgets", "agent"} {
		t.Run(entry, func(t *testing.T) {
			j := newJourney(t)
			const uid = 900001
			j.create(uid, "Дом", "Europe/Berlin")
			if entry == "buttons" {
				j.b.UIMode = "buttons"
			}
			j.send(uid, "/task", "")
			j.send(uid, "Полить цветы", "")
			j.send(uid, "13", "")
			if entry == "buttons" {
				var d Delivery
				if err := j.db.Where("viewer=? AND method=?", uid, "sendMessage").Last(&d).Error; err != nil {
					t.Fatal(err)
				}
				var p struct {
					Markup struct {
						Rows [][]struct {
							Label string `json:"text"`
							Token string `json:"callback_data"`
						} `json:"inline_keyboard"`
					} `json:"reply_markup"`
				}
				if err := json.Unmarshal([]byte(d.Payload), &p); err != nil {
					t.Fatal(err)
				}
				token := ""
				for _, row := range p.Markup.Rows {
					for _, b := range row {
						if b.Label == "Выполнено" {
							token = b.Token
						}
					}
				}
				if token == "" {
					t.Fatal("missing button")
				}
				for n := 0; n < 2; n++ {
					j.seq++
					if err := j.b.Handle(Update{ID: j.seq, Callback: &Callback{From: Person{ID: uid}, Message: Message{Chat: Chat{ID: uid, Type: "private"}}, Data: token}}); err != nil {
						t.Fatal(err)
					}
				}
			} else if entry == "widgets" {
				token := j.token(uid, "Выполнено")
				j.send(uid, "", token)
				j.send(uid, "", token)
			} else {
				j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "task_complete", Search: "Полить цветы"}}
				j.send(uid, "Я полил цветы", "")
				j.click(uid, "Понятно, продолжить")
				process(t, j)
				token := j.token(uid, "Подтвердить план")
				j.send(uid, "", token)
				j.send(uid, "", token)
			}
			h, e := j.b.Family.Current(uid)
			if e != nil {
				t.Fatal(e)
			}
			x, e := (&stats.Service{DB: j.db}).Summary(uid, h.ID, j.now)
			if e != nil || x.Balance != 13 || x.XP != 13 || x.Week != 13 {
				t.Fatal(x, e)
			}
			var count int64
			j.db.Model(&task.Attempt{}).Where("status=?", "accepted").Count(&count)
			if count != 1 {
				t.Fatal(count)
			}
		})
	}
}

func TestReplyContextRequiresFreshPersonalTaskButton(t *testing.T) {
	j, c, id := agentJourney(t)
	const uid = 900001
	j.send(uid, "/start", "")
	j.click(uid, "Сегодня")
	token := j.token(uid, "Выполнено")
	var msg Message
	raw := fmt.Sprintf(`{"from":{"id":900001},"chat":{"id":900001,"type":"private"},"text":"Перенеси это дело на завтра","reply_to_message":{"from":{"is_bot":true,"username":"test"},"reply_markup":{"inline_keyboard":[[{"callback_data":"%s"}]]}}}`, token)
	if e := json.Unmarshal([]byte(raw), &msg); e != nil {
		t.Fatal(e)
	}
	j.seq++
	if e := j.b.Handle(Update{ID: j.seq, Message: &msg}); e != nil {
		t.Fatal(e)
	}
	var job AIJob
	j.db.Last(&job)
	if job.CircleID != c || job.ReplyTask != id {
		t.Fatal("target missing")
	}
	if _, e := j.b.jobScope(&job); e != nil {
		t.Fatal(e)
	}
	if e := (&task.Service{DB: j.db}).SetDue(uid, c, id, "2026-11-18 12:00"); e != nil {
		t.Fatal(e)
	}
	if _, e := j.b.jobScope(&job); e == nil {
		t.Fatal("stale reply accepted")
	}
	j.b.reply = msg.Reply
	if _, _, e := j.b.replyTask(900002, c); e == nil {
		t.Fatal("another actor's button")
	}
}

func TestAgentFirstHomeRequiresExplicitConfirmedTimezone(t *testing.T) {
	j := newJourney(t)
	const uid = 900001
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Changes: []agent.Change{{Operation: "family_create", Name: "Тёплый дом", Timezone: "Europe/Berlin"}}}}
	j.send(uid, "/start", "")
	j.send(uid, "Создай Тёплый дом с берлинским временем", "")
	j.click(uid, "Понятно, продолжить")
	process(t, j)
	if _, e := j.b.Family.Current(uid); e == nil {
		t.Fatal("premature creation")
	}
	confirmAgent(t, j)
	h, e := j.b.Family.Current(uid)
	if e != nil || h.Timezone != "Europe/Berlin" || !h.Owner {
		t.Fatal(h, e)
	}
	j.now = j.now.Add(time.Minute)
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Changes: []agent.Change{{Operation: "family_leave"}}}}
	j.send(uid, "Хочу выйти из дома", "")
	process(t, j)
	if _, e := j.b.Family.Current(uid); e != nil {
		t.Fatal("sole owner lost home")
	}
}

func TestJoinLinkStaysLocal(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	j.send(900002, "/start", "")
	token, e := j.b.Family.Invite(900001, j.now)
	if e != nil {
		t.Fatal(e)
	}
	j.send(900002, "Хочу вступить https://t.me/test?start=join_"+token, "")
	var n int64
	if err := j.db.Model(&AIJob{}).Where("viewer=?", 900002).Count(&n).Error; err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, e = j.b.Family.Current(900002); e == nil {
		t.Fatal("joined before tapping link")
	}
	j.send(900002, "/start join_"+token, "")
	if _, e = j.b.Family.Current(900002); e != nil {
		t.Fatal(e)
	}
}
