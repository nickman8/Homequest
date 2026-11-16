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
