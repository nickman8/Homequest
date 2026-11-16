package family

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func setup(t *testing.T) *Service {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/family.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestFamiliesInvitesAndOldMembership(t *testing.T) {
	s := setup(t)
	now := time.Now().UTC()
	for _, id := range []int64{900001, 900002, 900003, 900004} {
		if e := s.Start(id, "Участник"); e != nil {
			t.Fatal(e)
		}
	}
	a, e := s.Create(900001, "Тёплый дом", "Europe/Berlin")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Create(900004, "Другой дом", "UTC"); e != nil {
		t.Fatal(e)
	}
	token, e := s.Invite(900001, now)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = s.Join(900002, token, now); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.View(900004, a.ID); e == nil {
		t.Fatal("cross family read")
	}
	if _, e = s.Invite(900002, now); e == nil {
		t.Fatal("non owner invite")
	}
	if e = s.Leave(900001); e == nil {
		t.Fatal("sole owner left")
	}
	if e = s.Leave(900002); e != nil {
		t.Fatal(e)
	}
	if _, e = s.View(900002, a.ID); e == nil {
		t.Fatal("stale membership")
	}
	if e = s.Join(900003, token, now.Add(8*24*time.Hour)); e == nil {
		t.Fatal("expired invite")
	}
	if e = s.Revoke(900001); e != nil {
		t.Fatal(e)
	}
	if e = s.Join(900003, token, now); e == nil {
		t.Fatal("revoked invite")
	}
}
func TestInvalidZoneAndExplicitContext(t *testing.T) {
	s := setup(t)
	_ = s.Start(900001, "A")
	_ = s.Start(900002, "B")
	if _, e := s.Create(900001, "Дом", "guess"); e == nil {
		t.Fatal("invalid timezone")
	}
	_, e := s.Create(900001, "Дом", "UTC")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Create(900002, "Второй", "UTC")
	if e != nil {
		t.Fatal(e)
	}
	token, e := s.Invite(900001, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Join(900002, token, time.Now()); e == nil {
		t.Fatal("implicit context change")
	}
}

func TestOwnerTransferRemovalAndTimezone(t *testing.T) {
	s := setup(t)
	for _, id := range []int64{900001, 900002, 900003} {
		_ = s.Start(id, "A")
	}
	h, _ := s.Create(900001, "Дом", "UTC")
	token, _ := s.Invite(900001, time.Now())
	_ = s.Join(900002, token, time.Now())
	_, _ = s.Create(900003, "Другой", "UTC")
	b, _ := s.Authorize(900002, h.ID)
	a, _ := s.Authorize(900001, h.ID)
	if e := s.Transfer(900003, b.ID); e == nil {
		t.Fatal("foreign transfer")
	}
	if e := s.Transfer(900001, b.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Invite(900001, time.Now()); e == nil {
		t.Fatal("old owner")
	}
	if e := s.SetTimezone(900002, "Europe/Berlin"); e != nil {
		t.Fatal(e)
	}
	if e := s.Remove(900002, b.ID); e == nil {
		t.Fatal("remove sole owner")
	}
	if e := s.Remove(900002, a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.View(900001, h.ID); e == nil {
		t.Fatal("removed member")
	}
	home, e := s.Current(900002)
	if e != nil || home.Timezone != "Europe/Berlin" {
		t.Fatal(home, e)
	}
}
