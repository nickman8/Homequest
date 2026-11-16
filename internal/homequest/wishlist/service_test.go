package wishlist

import (
	"bytes"
	"donetick.com/core/internal/homequest/family"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sync"
	"testing"
	"time"
)

func TestSecretClaimsRaceIsolationAndDeparture(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/wish.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	f, _ := family.New(db)
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	for i := int64(900001); i <= 900004; i++ {
		_ = f.Start(i, "Участник")
	}
	h, _ := f.Create(900001, "Тёплый дом", "UTC")
	_, _ = f.Create(900004, "Другой", "UTC")
	token, _ := f.Invite(900001, time.Now())
	_ = f.Join(900002, token, time.Now())
	_ = f.Join(900003, token, time.Now())
	item, e := s.Create(900001, h.ID, "Книга", "https://example.org/book", "Заметка", "1000 ₽")
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.List(900001, h.ID)
	if e != nil {
		t.Fatal(e)
	}
	rawBefore, _ := json.Marshal(before)
	results := make(chan int64, 2)
	var wg sync.WaitGroup
	for _, uid := range []int64{900002, 900003} {
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			if s.Claim(uid, h.ID, item.ID) == nil {
				results <- uid
			}
		}(uid)
	}
	wg.Wait()
	close(results)
	var winners []int64
	for uid := range results {
		winners = append(winners, uid)
	}
	if len(winners) != 1 {
		t.Fatal(winners)
	}
	winner := winners[0]
	if e = s.UpdateClaim(winner, h.ID, item.ID, "purchase"); e != nil {
		t.Fatal(e)
	}
	after, _ := s.List(900001, h.ID)
	rawAfter, _ := json.Marshal(after)
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Fatalf("owner payload leaks claim: %s", rawAfter)
	}
	if e = s.Claim(900001, h.ID, item.ID); e == nil {
		t.Fatal("self claim")
	}
	if e = s.UpdateClaim(900001, h.ID, item.ID, "release"); e == nil {
		t.Fatal("owner discovered/removed claim")
	}
	if _, e = s.List(900004, h.ID); e == nil {
		t.Fatal("foreign family")
	}
	if e = s.Claim(900004, h.ID, item.ID); e == nil {
		t.Fatal("foreign mutation")
	}
	if e = f.Leave(winner); e != nil {
		t.Fatal(e)
	}
	if e = s.UpdateClaim(winner, h.ID, item.ID, "release"); e == nil {
		t.Fatal("departed member")
	}
	// Family owner can release orphaned claims without an item-specific disclosure.
	if e = s.ReleaseDeparted(900001, h.ID); e != nil {
		t.Fatal(e)
	}
	other := int64(900002)
	if other == winner {
		other = 900003
	}
	if e = s.Claim(other, h.ID, item.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Create(900001, h.ID, "Bad", "javascript:alert(1)", "", ""); e == nil {
		t.Fatal("unsafe URL")
	}
}
