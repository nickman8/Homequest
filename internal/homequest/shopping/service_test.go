package shopping

import (
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sync"
	"testing"
	"time"
)

func TestShoppingAtomicBatchClaimVersionsAndScope(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/shop.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	f, _ := family.New(db)
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	for _, u := range []int64{900001, 900002, 900003, 900004} {
		f.Start(u, "A")
	}
	h, _ := f.Create(900001, "Дом", "UTC")
	invite, _ := f.Invite(900001, now)
	f.Join(900002, invite, now)
	f.Join(900003, invite, now)
	f.Create(900004, "Чужой", "UTC")
	items := []Input{{Name: "Молоко", Quantity: "2 л", Note: "Без лактозы"}, {Name: "Корм", Quantity: "1 пачка"}}
	if e = s.CreateBatch(900002, h.ID, "request", items, now); e != nil {
		t.Fatal(e)
	}
	if e = s.CreateBatch(900002, h.ID, "request", items, now); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.List(900001, h.ID, false)
	if len(rows) != 2 {
		t.Fatal("replayed batch", rows)
	}
	if e = s.CreateBatch(900001, h.ID, "bad", []Input{{Name: "Хлеб"}, {Name: ""}}, now); e == nil {
		t.Fatal("partial invalid batch")
	}
	again, _ := s.List(900001, h.ID, false)
	if len(again) != 2 {
		t.Fatal("partial write")
	}
	r := rows[0]
	var wg sync.WaitGroup
	for _, u := range []int64{900002, 900003} {
		wg.Add(1)
		go func(u int64) { defer wg.Done(); s.Change(u, h.ID, r.ID, r.Version, "claim", nil, now) }(u)
	}
	wg.Wait()
	rows, _ = s.List(900001, h.ID, false)
	r = rows[0]
	if r.Status != "claimed" || r.Version != 2 {
		t.Fatal(r)
	}
	if _, e = s.List(900004, h.ID, false); e == nil {
		t.Fatal("foreign read")
	}
	if e = s.Change(900004, h.ID, r.ID, r.Version, "buy", nil, now); e == nil {
		t.Fatal("foreign write")
	}
	if e = s.Change(900001, h.ID, r.ID, r.Version, "release", nil, now); e == nil {
		t.Fatal("owner released another person's claim")
	}
	winner := int64(900002)
	u, _ := f.Authorize(winner, h.ID)
	if u.ID != r.ClaimedBy {
		winner = 900003
	}
	if e = s.Change(winner, h.ID, r.ID, r.Version, "buy", nil, now); e != nil {
		t.Fatal(e)
	}
	if e = s.Change(winner, h.ID, r.ID, r.Version, "buy", nil, now); e == nil {
		t.Fatal("stale version")
	}
	history, _ := s.List(900001, h.ID, true)
	if len(history) != 1 || history[0].Status != "bought" {
		t.Fatal(history)
	}
	if e = s.Change(winner, h.ID, r.ID, history[0].Version, "reopen", nil, now); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.List(900001, h.ID, false)
	if len(rows) != 2 {
		t.Fatal("reopen")
	}
	var events int64
	db.Model(&Event{}).Count(&events)
	if events != 5 {
		t.Fatal("audit duplicated", events)
	}
	New(db)
	rows, _ = s.List(900001, h.ID, false)
	if len(rows) != 2 {
		t.Fatal("restart")
	}
	// A departing buyer releases only pending claims, with one audit transition.
	r = rows[0]
	if e = s.Change(winner, h.ID, r.ID, r.Version, "claim", nil, now); e != nil {
		t.Fatal(e)
	}
	if e = f.Leave(winner); e != nil {
		t.Fatal(e)
	}
	if e = s.ReleaseDeparted(h.ID, now); e != nil {
		t.Fatal(e)
	}
	if e = s.ReleaseDeparted(h.ID, now); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.List(900001, h.ID, false)
	if rows[0].Status != "needed" || rows[0].ClaimedBy != 0 {
		t.Fatal("departed claim not freed", rows[0])
	}
	if e = s.Change(winner, h.ID, rows[0].ID, rows[0].Version, "buy", nil, now); e == nil {
		t.Fatal("departed buyer")
	}

}
