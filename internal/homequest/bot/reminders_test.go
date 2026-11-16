package bot

import (
	"context"
	"donetick.com/core/internal/homequest/task"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestReminderPlanningRestartSnoozeAndInvalidation(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/reminders.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<p>Дом</p>", "test")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	b.Now = func() time.Time { return now }
	_ = b.Family.Start(900001, "A")
	h, _ := b.Family.Create(900001, "Дом", "UTC")
	s := &task.Service{DB: db}
	item, _ := s.Create(900001, h.ID, "Мусор", 10, nil)
	if e = s.SetSchedule(900001, h.ID, item.ID, task.Schedule{Mode: "daily", Hour: 12}, now); e != nil {
		t.Fatal(e)
	}
	if e = b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	if e = b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	var n int64
	db.Model(&Delivery{}).Count(&n)
	if n != 3 {
		t.Fatal("duplicate schedule", n)
	}
	api := &fakeAPI{}
	now = now.Add(time.Hour)
	if e = b.Dispatch(context.Background(), api); e != nil {
		t.Fatal(e)
	}
	if api.calls != 1 {
		t.Fatal("pre due not sent")
	}
	var d Delivery
	db.Order("id").First(&d)
	if e = b.Snooze(900001, d.ID, 30*time.Minute); e != nil {
		t.Fatal(e)
	}
	var instance task.Instance
	db.First(&instance, item.ID)
	if instance.DueAt.Hour() != 12 {
		t.Fatal("snooze changed due date")
	}
	now = now.Add(30 * time.Minute)
	_ = b.Dispatch(context.Background(), api)
	if api.calls != 2 {
		t.Fatal("snooze missing")
	}
	_, _ = s.Submit(900001, h.ID, item.ID, "", now)
	now = now.Add(30 * time.Minute)
	_ = b.Dispatch(context.Background(), api)
	if api.calls != 2 {
		t.Fatal("closed reminder sent")
	}
	// A fresh process must not replay reminders missed by hours.
	now = now.Add(24 * time.Hour)
	_ = b.PlanReminders([]int{-60, 0, 120})
	now = now.Add(8 * time.Hour)
	_ = b.Dispatch(context.Background(), api)
	if api.calls != 2 {
		t.Fatal("restart avalanche")
	}
}
func TestQuietHoursAndVacation(t *testing.T) {
	now := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	pref := RestPreference{QuietStart: 22, QuietEnd: 8}
	next, drop := deliveryTime(now, "UTC", pref)
	if drop || next.Hour() != 8 || next.Day() != 17 {
		t.Fatal(next, drop)
	}
	pref.VacationUntil = now.Add(7 * 24 * time.Hour)
	_, drop = deliveryTime(now, "UTC", pref)
	if !drop {
		t.Fatal("vacation ignored")
	}
}
