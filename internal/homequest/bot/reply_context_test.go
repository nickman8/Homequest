package bot

import (
	"context"
	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/agent/agenttest"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/wishlist"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"donetick.com/core/internal/homequest/task"
)

func attachReply(t *testing.T, b *Bot, tokens ...string) {
	t.Helper()
	rows := [][]map[string]string{{}}
	for _, token := range tokens {
		rows[0] = append(rows[0], map[string]string{"callback_data": token})
	}
	raw, e := json.Marshal(map[string]any{"reply_to_message": map[string]any{"from": map[string]any{"is_bot": true, "username": "test"}, "reply_markup": map[string]any{"inline_keyboard": rows}}})
	if e != nil {
		t.Fatal(e)
	}
	var m Message
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	b.reply = m.Reply
}

func TestReplyTaskListMustNotSilentlySelectFirst(t *testing.T) {
	j, c, id := agentJourney(t)
	second, e := (&task.Service{DB: j.db}).Create(900001, c, "Второе дело", 10, nil)
	if e != nil {
		t.Fatal(e)
	}
	firstButton, e := j.b.button(900001, c, "done:"+strconv.Itoa(id), "Готово")
	if e != nil {
		t.Fatal(e)
	}
	secondButton, e := j.b.button(900001, c, "done:"+strconv.Itoa(second.ID), "Готово")
	if e != nil {
		t.Fatal(e)
	}
	attachReply(t, j.b, firstButton.Action, secondButton.Action)
	if _, _, e = j.b.replyTask(900001, c); e == nil {
		t.Fatal("ambiguous two-task card silently selected first target")
	}
}

func sendReply(t *testing.T, j *journey, text string, tokens ...string) AIJob {
	t.Helper()
	attachReply(t, j.b, tokens...)
	raw, e := json.Marshal(map[string]any{"from": map[string]any{"id": 900001}, "chat": map[string]any{"id": 900001, "type": "private"}, "text": text})
	if e != nil {
		t.Fatal(e)
	}
	var m Message
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	m.Reply = j.b.reply
	j.seq++
	if e = j.b.Handle(Update{ID: j.seq, Message: &m}); e != nil {
		t.Fatal(e)
	}
	var job AIJob
	j.db.Last(&job)
	return job
}

func TestReplyResourceCardsPlanAndApplyAfterRestart(t *testing.T) {
	for _, kind := range []string{"wish", "shopping", "reward"} {
		t.Run(kind, func(t *testing.T) {
			j, c, _ := agentJourney(t)
			var id int
			var buttonKind, operation string
			switch kind {
			case "wish":
				r, e := (&wishlist.Service{DB: j.db}).Create(900001, c, "Атлас", "https://example.invalid/atlas", "заметка", "500")
				if e != nil {
					t.Fatal(e)
				}
				id, buttonKind, operation = r.ID, "wish_info:", "wish_archive"
			case "shopping":
				service := &shopping.Service{DB: j.db}
				if e := service.CreateBatch(900001, c, "reply-test", []shopping.Input{{Name: "Молоко"}}, j.now); e != nil {
					t.Fatal(e)
				}
				rows, e := service.List(900001, c, false)
				if e != nil || len(rows) != 1 {
					t.Fatal(rows, e)
				}
				id, buttonKind, operation = rows[0].ID, "shop_manage:", "shopping_buy"
			case "reward":
				r, e := (&reward.Service{DB: j.db}).Create(900001, c, "Кино", "Договорённость", 10)
				if e != nil {
					t.Fatal(e)
				}
				id, buttonKind, operation = r.ID, "reward_manage:", "reward_archive"
			}
			button, e := j.b.button(900001, c, buttonKind+strconv.Itoa(id), "Объект")
			if e != nil {
				t.Fatal(e)
			}
			j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: operation}}
			job := sendReply(t, j, "Сделай с этим объектом запрошенное действие", button.Action)
			if job.ReplyTarget.Kind != kind || job.ReplyTarget.ID != id {
				t.Fatal("reply target not persisted", job.ReplyTarget)
			}
			scope, e := j.b.jobScope(&job)
			if e != nil {
				t.Fatal(e)
			}
			if scope.ReplyResource == nil || scope.ReplyResource.ID != id || scope.ReplyResource.Snapshot != job.ReplyTarget.Snapshot {
				t.Fatal("missing scoped projection", scope.ReplyResource)
			}
			if job.State == "consent" {
				j.click(900001, "Понятно, продолжить")
			}
			process(t, j)
			if j.card(900001).Title != "Проверьте изменения" {
				t.Fatal(j.card(900001))
			}
			j.restart()
			confirmAgent(t, j)
			if e = j.db.First(&job, job.ID).Error; e != nil {
				t.Fatal(e)
			}
			if job.State != "done" {
				t.Fatal("not applied", job.State)
			}
			var p agent.Plan
			if e = j.db.First(&p, "id=?", job.PlanID).Error; e != nil {
				t.Fatal(e)
			}
			if p.ReplyTarget != job.ReplyTarget || p.State != "applied" {
				t.Fatal("unbound plan", p)
			}
		})
	}
}

func TestReplyResourceRejectsUsedExpiredForeignAndChangedButtons(t *testing.T) {
	for _, fault := range []string{"used", "expired", "foreign_viewer", "foreign_family", "forged_bot", "edited", "mixed", "navigation_only"} {
		t.Run(fault, func(t *testing.T) {
			j, c, _ := agentJourney(t)
			service := &reward.Service{DB: j.db}
			r, e := service.Create(900001, c, "Кино", "", 10)
			if e != nil {
				t.Fatal(e)
			}
			button, e := j.b.button(900001, c, "reward_manage:"+strconv.Itoa(r.ID), "Каталог")
			if e != nil {
				t.Fatal(e)
			}
			attachReply(t, j.b, button.Action)
			uid, circle := int64(900001), c
			switch fault {
			case "used":
				j.db.Model(&Action{}).Where("token=?", button.Action).Update("used", true)
			case "expired":
				j.now = j.now.Add(25 * time.Hour)
			case "foreign_viewer":
				uid = 900002
			case "foreign_family":
				circle = c + 1
			case "forged_bot":
				j.b.reply.From.Username = "other_bot"
			case "edited":
				if e = service.Edit(uid, c, r.ID, "Другое кино", 20, ""); e != nil {
					t.Fatal(e)
				}
			case "mixed":
				other, e := service.Create(uid, c, "Другая награда", "", 12)
				if e != nil {
					t.Fatal(e)
				}
				b, e := j.b.button(uid, c, "reward_manage:"+strconv.Itoa(other.ID), "Каталог")
				if e != nil {
					t.Fatal(e)
				}
				attachReply(t, j.b, button.Action, b.Action)
			case "navigation_only":
				b, e := j.b.button(uid, c, "rewards:1", "Следующая")
				if e != nil {
					t.Fatal(e)
				}
				attachReply(t, j.b, b.Action)
			}
			if target, e := j.b.replyTarget(uid, circle); e == nil {
				t.Fatal("invalid reply accepted", fault, target)
			}
		})
	}
}

func TestReplyDeduplicatesButtonsButNotDifferentKindsWithSameID(t *testing.T) {
	j, c, _ := agentJourney(t)
	r, e := (&reward.Service{DB: j.db}).Create(900001, c, "Кино", "", 10)
	if e != nil {
		t.Fatal(e)
	}
	w, e := (&wishlist.Service{DB: j.db}).Create(900001, c, "Книга", "", "", "")
	if e != nil {
		t.Fatal(e)
	}
	if r.ID != w.ID {
		t.Fatal("test requires overlapping resource IDs")
	}
	first, e := j.b.button(900001, c, "reward_manage:"+strconv.Itoa(r.ID), "Каталог")
	if e != nil {
		t.Fatal(e)
	}
	second, e := j.b.button(900001, c, "reward_request:"+strconv.Itoa(r.ID), "Запросить")
	if e != nil {
		t.Fatal(e)
	}
	attachReply(t, j.b, first.Action, second.Action)
	target, e := j.b.replyTarget(900001, c)
	if e != nil || target.Kind != "reward" || target.ID != r.ID {
		t.Fatal(target, e)
	}
	wish, e := j.b.button(900001, c, "wish_info:"+strconv.Itoa(w.ID), "Подробнее")
	if e != nil {
		t.Fatal(e)
	}
	attachReply(t, j.b, first.Action, wish.Action)
	if _, e = j.b.replyTarget(900001, c); e == nil {
		t.Fatal("same ID from different kinds treated as one target")
	}
}

func TestReplyBindsReportRedemptionRoomAndHandoffCards(t *testing.T) {
	j, c, id := agentJourney(t)
	const uid int64 = 900001
	ts := &task.Service{DB: j.db}
	attempt, e := ts.Submit(uid, c, id, "", j.now)
	if e != nil {
		t.Fatal(e)
	}
	rw := &reward.Service{DB: j.db}
	rewardItem, e := rw.Create(uid, c, "Чай", "", 5)
	if e != nil {
		t.Fatal(e)
	}
	request, e := rw.Request(uid, c, rewardItem.ID, "reply-redemption", j.now)
	if e != nil {
		t.Fatal(e)
	}
	rooms, e := ts.Rooms(uid, c)
	if e != nil || len(rooms) != 1 {
		t.Fatal(rooms, e)
	}
	invite, e := j.b.Family.Invite(uid, j.now)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.b.Family.Start(900002, "Другой участник"); e != nil {
		t.Fatal(e)
	}
	if e = j.b.Family.Join(900002, invite, j.now); e != nil {
		t.Fatal(e)
	}
	owner, e := j.b.Family.Authorize(uid, c)
	if e != nil {
		t.Fatal(e)
	}
	recipient, e := j.b.Family.Authorize(900002, c)
	if e != nil {
		t.Fatal(e)
	}
	assigned, e := ts.Create(uid, c, "Передать дело", 5, &owner.ID)
	if e != nil {
		t.Fatal(e)
	}
	handoff, e := ts.OfferHandoff(uid, c, assigned.ID, recipient.ID, j.now)
	if e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		kind, action string
		id           int
	}{
		{"report", "undo:" + strconv.Itoa(attempt.ID), attempt.ID},
		{"redemption", "redemption:" + strconv.Itoa(request.ID) + ":cancel", request.ID},
		{"room", "room_rename:" + strconv.Itoa(rooms[0].ID), rooms[0].ID},
		{"handoff", "handoff:" + strconv.Itoa(handoff.ID) + ":cancel", handoff.ID},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			button, e := j.b.button(uid, c, tc.action, "Объект")
			if e != nil {
				t.Fatal(e)
			}
			attachReply(t, j.b, button.Action)
			target, e := j.b.replyTarget(uid, c)
			if e != nil {
				t.Fatal(e)
			}
			if target.Kind != tc.kind || target.ID != tc.id || len(target.Snapshot) != 64 {
				t.Fatal(target)
			}
			scope, e := (&agent.Service{DB: j.db}).Context(uid, j.now)
			if e != nil {
				t.Fatal(e)
			}
			if e = (&agent.Service{DB: j.db}).BindReply(&scope, target, j.now); e != nil {
				t.Fatal(e)
			}
			if scope.ReplyResource == nil || scope.ReplyResource.Kind != tc.kind {
				t.Fatal(scope.ReplyResource)
			}
		})
	}
}

func TestReplyToEditedWidgetUsesDeliveredStateNotOriginalKeyboard(t *testing.T) {
	j, c, _ := agentJourney(t)
	const uid int64 = 900001
	rw := &reward.Service{DB: j.db}
	first, e := rw.Create(uid, c, "Первая награда", "", 5)
	if e != nil {
		t.Fatal(e)
	}
	second, e := rw.Create(uid, c, "Вторая награда", "", 7)
	if e != nil {
		t.Fatal(e)
	}
	api := &widgetMessageTransport{}
	drain := func() {
		t.Helper()
		for i := 0; i < 10; i++ {
			if e := j.b.Dispatch(context.Background(), api); e != nil {
				t.Fatal(e)
			}
		}
	}
	drain()
	if e = j.b.rewards(uid, 0, c, 0); e != nil {
		t.Fatal(e)
	}
	original := j.token(uid, "Каталог")
	drain()
	var view CardView
	if e = j.db.First(&view, "viewer=?", uid).Error; e != nil {
		t.Fatal(e)
	}
	messageID := view.MessageID
	attachReply(t, j.b, original)
	j.b.reply.MessageID = messageID
	target, e := j.b.replyTarget(uid, c)
	if e != nil || target.ID != first.ID {
		t.Fatal(target, e)
	}
	// Telegram's inline keyboard still names the first reward after chat_data edit.
	if e = j.b.rewards(uid, messageID, c, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = j.b.replyTarget(uid, c); e == nil {
		t.Fatal("pending widget edit must not select old or new target")
	}
	drain()
	target, e = j.b.replyTarget(uid, c)
	if e != nil || target.ID != second.ID {
		t.Fatal("obsolete original keyboard selected", target, e)
	}
	// A queued update with an uncertain/failed delivery must also refuse context.
	if e = j.b.rewards(uid, messageID, c, 0); e != nil {
		t.Fatal(e)
	}
	failed := &fakeAPI{fail: true}
	if e = j.b.Dispatch(context.Background(), failed); e != nil {
		t.Fatal(e)
	}
	if _, e = j.b.replyTarget(uid, c); e == nil {
		t.Fatal("failed widget edit supplied reply context")
	}
}
