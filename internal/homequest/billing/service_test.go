package billing

import (
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestManualEntitlementAuthorizationModeAndPersistence(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/billing.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	f, _ := family.New(db)
	s, e := New(db, "manual_test")
	if e != nil {
		t.Fatal(e)
	}
	for _, u := range []int64{900001, 900002, 900004} {
		_ = f.Start(u, "Тест")
	}
	h, _ := f.Create(900001, "Дом", "UTC")
	other, _ := f.Create(900004, "Другой", "UTC")
	now := time.Now().UTC()
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	if e = s.Manual(900002, h.ID, true, now); e == nil {
		t.Fatal("member granted Premium")
	}
	if e = s.Manual(900004, h.ID, true, now); e == nil {
		t.Fatal("foreign grant")
	}
	if e = s.Manual(900001, h.ID, true, now); e != nil {
		t.Fatal(e)
	}
	if e = s.Manual(900001, h.ID, true, now); e != nil {
		t.Fatal(e)
	}
	restarted, e := New(db, "manual_test")
	if e != nil {
		t.Fatal(e)
	}
	if yes, e := restarted.Active(900002, h.ID, now); e != nil || !yes {
		t.Fatal("not persisted", e)
	}
	if yes, _ := s.Active(900004, other.ID, now); yes {
		t.Fatal("cross-family entitlement")
	}
	live, _ := New(db, "stars")
	if e = live.Manual(900001, h.ID, true, now); e == nil {
		t.Fatal("manual bypass in live mode")
	}
	end := now.Add(-time.Second)
	db.Model(&Entitlement{}).Where("circle_id = ?", h.ID).Update("expires_at", end)
	if yes, _ := s.Active(900002, h.ID, now); yes {
		t.Fatal("expired entitlement")
	}
	if e = s.Manual(900001, h.ID, false, now); e != nil {
		t.Fatal(e)
	}
	if yes, _ := s.Active(900002, h.ID, now); yes {
		t.Fatal("revocation")
	}
}
