package reward

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sync"
	"testing"
	"time"
)

func TestRedemptionReservationRefundSnapshotAndUndo(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/rewards.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	f, _ := family.New(db)
	tasks, _ := task.New(db)
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	_ = f.Start(900001, "A")
	_ = f.Start(900002, "B")
	h, _ := f.Create(900001, "Дом", "UTC")
	now := time.Now().UTC()
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	item, _ := tasks.Create(900001, h.ID, "Дело", 10, nil)
	attempt, _ := tasks.Submit(900002, h.ID, item.ID, "", now)
	r, e := s.Create(900001, h.ID, "Выбрать фильм", "Вечером", 10)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	ids := make(chan int, 2)
	for _, key := range []string{"request-a", "request-b"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			v, e := s.Request(900002, h.ID, r.ID, k, now)
			if e == nil {
				ids <- v.ID
			}
		}(key)
	}
	wg.Wait()
	close(ids)
	var requests []int
	for id := range ids {
		requests = append(requests, id)
	}
	if len(requests) != 1 {
		t.Fatal("overspent", requests)
	}
	if e = tasks.Undo(900002, h.ID, attempt.ID, now); e == nil {
		t.Fatal("undo ignored reserved funds")
	}
	if e = s.Edit(900001, h.ID, r.ID, "Новое название", 100); e != nil {
		t.Fatal(e)
	}
	if e = s.Archive(900001, h.ID, r.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Request(900002, h.ID, r.ID, "archived", now); e == nil {
		t.Fatal("archived reward requested")
	}
	if e = s.Resolve(900001, h.ID, requests[0], "reject", now); e != nil {
		t.Fatal(e)
	}
	if e = s.Resolve(900001, h.ID, requests[0], "reject", now); e != nil {
		t.Fatal(e)
	}
	if balance, _ := tasks.Balance(900002, h.ID); balance != 10 {
		t.Fatal("refund changed snapshot", balance)
	}
	summary, _ := (&stats.Service{DB: db}).Summary(900002, h.ID, now)
	if summary.XP != 10 || summary.Week != 10 {
		t.Fatal(summary)
	}
	r, e = s.Create(900001, h.ID, "Фильм", "Вечером", 10)
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.Request(900002, h.ID, r.ID, "last", now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Resolve(900002, h.ID, v.ID, "approve", now); e == nil {
		t.Fatal("self approve")
	}
	if e = s.Resolve(900001, h.ID, v.ID, "approve", now); e != nil {
		t.Fatal(e)
	}
	if e = s.Resolve(900001, h.ID, v.ID, "deliver", now); e != nil {
		t.Fatal(e)
	}
	if e = s.Resolve(900002, h.ID, v.ID, "cancel", now); e == nil {
		t.Fatal("cancel after delivery")
	}
}
