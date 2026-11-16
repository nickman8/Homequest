package stats

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestWeekBoundaryTiesAndSpending(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/stats.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	f, _ := family.New(db)
	tasks, _ := task.New(db)
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []int64{900001, 900002} {
		_ = f.Start(id, "A")
	}
	h, _ := f.Create(900001, "Дом", "Europe/Berlin")
	now := time.Date(2026, 11, 15, 22, 59, 0, 0, time.UTC)
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	item, _ := tasks.Create(900001, h.ID, "Задание", 10, nil)
	_, _ = tasks.Submit(900001, h.ID, item.ID, "", now)
	after := now.Add(2 * time.Minute)
	summary, e := s.Summary(900001, h.ID, after)
	if e != nil || summary.Week != 0 || summary.XP != 10 || summary.Balance != 10 {
		t.Fatal(summary, e)
	}
	for _, id := range []int64{900001, 900002} {
		item, _ := tasks.Create(900001, h.ID, "Новое", 5, nil)
		_, _ = tasks.Submit(id, h.ID, item.ID, "", after)
	}
	rank, e := s.Ranking(900001, h.ID, after)
	if e != nil || len(rank) != 2 || rank[0].Rank != 1 || rank[1].Rank != 1 {
		t.Fatal(rank, e)
	}
	badges, e := s.Badges(900001, h.ID, after)
	if e != nil || len(badges) != 1 {
		t.Fatal(badges, e)
	}
	_, _ = s.Badges(900001, h.ID, after)
	var n int64
	db.Model(&Badge{}).Count(&n)
	if n != 1 {
		t.Fatal("duplicate badge")
	}
}
