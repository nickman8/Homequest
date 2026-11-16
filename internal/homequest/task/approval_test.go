package task

import (
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestPhotoApprovalSnapshotRejectAndUndo(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/approval.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	f, _ := family.New(db)
	s, _ := New(db)
	for _, id := range []int64{900001, 900002, 900003} {
		_ = f.Start(id, "A")
	}
	h, _ := f.Create(900001, "Дом", "UTC")
	token, _ := f.Invite(900001, time.Now())
	_ = f.Join(900002, token, time.Now())
	_, _ = f.Create(900003, "Другой", "UTC")
	item, _ := s.Create(900001, h.ID, "Мусор", 10, nil)
	if e = s.Configure(900001, h.ID, item.ID, true, true); e != nil {
		t.Fatal(e)
	}
	a, e := s.Submit(900002, h.ID, item.ID, "", time.Now())
	if e != nil || a.Status != "photo" {
		t.Fatal(a, e)
	}
	if e = s.Review(900001, h.ID, a.ID, true, time.Now()); e == nil {
		t.Fatal("accepted without photo")
	}
	if e = s.Attach(900003, h.ID, a.ID, "photo-file", time.Now()); e == nil {
		t.Fatal("foreign photo")
	}
	if e = s.Attach(900002, h.ID, a.ID, "photo-file", time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900002, h.ID, a.ID, true, time.Now()); e == nil {
		t.Fatal("self approval")
	}
	if balance, _ := s.Balance(900002, h.ID); balance != 0 {
		t.Fatal("early points")
	}
	if e = s.Edit(900001, h.ID, item.ID, "Мусор", 99); e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900001, h.ID, a.ID, false, time.Now()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Submit(900002, h.ID, item.ID, "new-photo", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Edit(900001, h.ID, item.ID, "Мусор", 1000); e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900001, h.ID, a.ID, true, time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900001, h.ID, a.ID, true, time.Now()); e != nil {
		t.Fatal(e)
	}
	if balance, _ := s.Balance(900002, h.ID); balance != 99 {
		t.Fatal("snapshot", balance)
	}
	if _, e = s.Photo(900003, h.ID, a.ID); e == nil {
		t.Fatal("foreign viewer")
	}
	plain, _ := s.Create(900001, h.ID, "Посуда", 5, nil)
	done, e := s.Submit(900002, h.ID, plain.ID, "", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Undo(900002, h.ID, done.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	if e = s.Undo(900002, h.ID, done.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	if balance, _ := s.Balance(900002, h.ID); balance != 99 {
		t.Fatal("double undo", balance)
	}
}

func TestLegacyAttemptMigration(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/legacy.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	h, _ := f.Create(900001, "Дом", "UTC")
	item, _ := s.Create(900001, h.ID, "Мусор", 10, nil)
	a, e := s.Submit(900001, h.ID, item.ID, "", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	db.Model(&a).Updates(map[string]any{"status": "", "user_id": 0})
	db.Exec("DROP INDEX idx_hq_attempts_instance_id")
	db.Exec("CREATE UNIQUE INDEX idx_hq_attempts_instance_id ON hq_attempts(instance_id)")
	s, e = New(db)
	if e != nil {
		t.Fatal(e)
	}
	var migrated Attempt
	db.First(&migrated, a.ID)
	if migrated.Status != "accepted" || migrated.UserID == 0 {
		t.Fatal("legacy attempt not migrated")
	}
	if e = s.Undo(900001, h.ID, a.ID, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Submit(900001, h.ID, item.ID, "", time.Now()); e != nil {
		t.Fatal("legacy unique index survived", e)
	}
}

func TestDesignatedReviewerAndDeparture(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/reviewer.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	f, _ := family.New(db)
	s, _ := New(db)
	for _, uid := range []int64{900001, 900002, 900003} {
		_ = f.Start(uid, "Тест")
	}
	h, _ := f.Create(900001, "Дом", "UTC")
	now := time.Now()
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	_ = f.Join(900003, token, now)
	reviewer, _ := f.Authorize(900003, h.ID)
	item, _ := s.Create(900001, h.ID, "Пыль", 10, nil)
	_ = s.Configure(900001, h.ID, item.ID, true, false)
	if e := s.SetReviewer(900001, h.ID, item.ID, reviewer.ID); e != nil {
		t.Fatal(e)
	}
	a, e := s.Submit(900002, h.ID, item.ID, "", now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900001, h.ID, a.ID, true, now); e == nil {
		t.Fatal("ignored designated reviewer")
	}
	if e = f.Leave(900003); e != nil {
		t.Fatal(e)
	}
	if e = s.Review(900003, h.ID, a.ID, true, now); e == nil {
		t.Fatal("departed reviewer")
	}
	if e = s.Review(900002, h.ID, a.ID, true, now); e == nil {
		t.Fatal("self approval fallback")
	}
	if e = s.Review(900001, h.ID, a.ID, true, now); e != nil {
		t.Fatal(e)
	}
	db.First(&a, a.ID)
	if a.ReviewedBy == nil || a.ReviewedAt == nil {
		t.Fatal("missing review audit")
	}
}

func TestConcurrentApproveRejectIsSingleDecision(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/race.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	_ = f.Start(900002, "B")
	h, _ := f.Create(900001, "Дом", "UTC")
	now := time.Now()
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	item, _ := s.Create(900001, h.ID, "Посуда", 10, nil)
	_ = s.Configure(900001, h.ID, item.ID, true, false)
	a, _ := s.Submit(900002, h.ID, item.ID, "", now)
	results := make(chan error, 2)
	for _, approve := range []bool{true, false} {
		go func(v bool) { results <- s.Review(900001, h.ID, a.ID, v, now) }(approve)
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("conflicting decisions", successes)
	}
	db.First(&a, a.ID)
	balance, _ := s.Balance(900002, h.ID)
	if (a.Status == "accepted" && balance != 10) || (a.Status == "rejected" && balance != 0) {
		t.Fatal(a.Status, balance)
	}
}
