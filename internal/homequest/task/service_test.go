package task

import (
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sync"
	"testing"
	"time"
)

func TestCompletionRaceSnapshotAndAuthorization(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/task.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	f, e := family.New(db)
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []int64{900001, 900002, 900003, 900004} {
		_ = f.Start(id, "A")
	}
	h, e := f.Create(900001, "Дом", "UTC")
	if e != nil {
		t.Fatal(e)
	}
	token, _ := f.Invite(900001, time.Now())
	_ = f.Join(900002, token, time.Now())
	_ = f.Join(900003, token, time.Now())
	_, _ = f.Create(900004, "Чужой", "UTC")
	item, e := s.Create(900001, h.ID, "Вынести мусор", 10, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Complete(900004, h.ID, item.ID, time.Now()); e == nil {
		t.Fatal("cross family")
	}
	var wg sync.WaitGroup
	for _, uid := range []int64{900002, 900003} {
		wg.Add(1)
		go func(uid int64) { defer wg.Done(); _, _ = s.Complete(uid, h.ID, item.ID, time.Now()) }(uid)
	}
	wg.Wait()
	a, _ := s.Balance(900002, h.ID)
	b, _ := s.Balance(900003, h.ID)
	if a+b != 10 {
		t.Fatal("race duplicated points", a, b)
	}
	_, _ = s.Complete(900002, h.ID, item.ID, time.Now())
	a2, _ := s.Balance(900002, h.ID)
	b2, _ := s.Balance(900003, h.ID)
	if a2+b2 != 10 {
		t.Fatal("replay")
	}
	user, _ := f.Authorize(900002, h.ID)
	assigned, e := s.Create(900001, h.ID, "Посуда", 7, &user.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Complete(900003, h.ID, assigned.ID, time.Now()); e == nil {
		t.Fatal("wrong assignee")
	}
	if e = s.Edit(900003, h.ID, assigned.ID, "Изменено", 100); e == nil {
		t.Fatal("unauthorized edit")
	}
	if e = s.Edit(900001, h.ID, assigned.ID, "Посуда", 9); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Complete(900002, h.ID, assigned.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
}

func TestPauseCancelAndAssignment(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/manage.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	_ = f.Start(900002, "B")
	h, _ := f.Create(900001, "Дом", "UTC")
	token, _ := f.Invite(900001, time.Now())
	_ = f.Join(900002, token, time.Now())
	item, _ := s.Create(900001, h.ID, "Задание", 10, nil)
	b, _ := f.Authorize(900002, h.ID)
	if e := s.Manage(900002, h.ID, item.ID, "pause", 0); e == nil {
		t.Fatal("not author")
	}
	if e := s.Manage(900001, h.ID, item.ID, "pause", 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Submit(900002, h.ID, item.ID, "", time.Now()); e == nil {
		t.Fatal("paused completion")
	}
	_ = s.Manage(900001, h.ID, item.ID, "resume", 0)
	if e := s.Manage(900001, h.ID, item.ID, "assign", b.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Submit(900001, h.ID, item.ID, "", time.Now()); e == nil {
		t.Fatal("assigned to other")
	}
	if e := s.Manage(900001, h.ID, item.ID, "cancel", 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Submit(900002, h.ID, item.ID, "", time.Now()); e == nil {
		t.Fatal("cancelled reopened")
	}
}
