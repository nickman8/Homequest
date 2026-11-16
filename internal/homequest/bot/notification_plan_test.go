package bot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFamilyReminderPlanThroughButtonsAndRestart(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	j.create(900004, "Другой", "UTC")
	j.send(900001, "/task", "")
	j.send(900001, "Мусор", "")
	j.send(900001, "10", "")
	j.click(900001, "Параметры")
	j.click(900001, "Повторение")
	j.send(900001, "каждый день 19:00", "")
	if e := j.b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	var old Delivery
	j.db.Where("notice=?", true).First(&old)
	j.send(900001, "/start", "")
	j.click(900001, "Настройки")
	j.click(900001, "Напоминания")
	j.click(900001, "Для всего дома")
	stale := j.token(900001, "Мягкий режим")
	j.click(900001, "Только в срок")
	h, _ := j.b.Family.Current(900001)
	for _, uid := range []int64{900002, 900004} {
		if e := j.b.SetReminderPlan(uid, h.ID, 1, []int{-30, 0}); e == nil {
			t.Fatal("non-owner/foreign changed family plan")
		}
	}
	if e := j.b.SetReminderPlan(900001, h.ID, 1, []int{0, 60, 120}); e == nil {
		t.Fatal("multiple overdue repeats")
	}
	j.send(900001, "", stale)
	j.restart()
	if e := j.b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	var active []Delivery
	j.db.Where("notice=? AND sent=?", true, false).Find(&active)
	if len(active) != 2 {
		t.Fatal("old jobs remain/new jobs duplicated", len(active))
	}
	for _, r := range active {
		if r.PolicyVersion != 1 || r.ScheduledFor.Hour() != 19 {
			t.Fatal(r)
		}
	}
	if e := j.b.Snooze(900001, old.ID, 30*time.Minute); e == nil {
		t.Fatal("stale snooze revived old plan")
	}
	api := &noticeTransport{}
	j.now = time.Date(2026, 3, 1, 18, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if e := j.b.Dispatch(context.Background(), api); e != nil {
			t.Fatal(e)
		}
	}
	if len(api.notices) != 0 {
		t.Fatal("old pre-due delivered")
	}
	j.now = j.now.Add(time.Hour)
	for i := 0; i < 5; i++ {
		_ = j.b.Dispatch(context.Background(), api)
	}
	if len(api.notices) != 2 {
		t.Fatal("new due not delivered", len(api.notices))
	}
	j.send(900001, "/start", "")
	j.click(900001, "Настройки")
	j.click(900001, "Напоминания")
	j.click(900001, "Для всего дома")
	j.click(900001, "Другие интервалы")
	j.send(900001, "за 30, в срок, через 90", "")
	if !strings.Contains(j.card(900001).Text, "за 30 мин.") {
		t.Fatal("custom plan not shown", j.card(900001))
	}
	j.send(900002, "/start", "")
	j.click(900002, "Настройки")
	j.click(900002, "Напоминания")
	for _, button := range j.card(900002).Buttons {
		if button.Label == "Для всего дома" {
			t.Fatal("non-owner editor shown")
		}
	}
}
