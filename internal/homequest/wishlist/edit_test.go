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

func TestOwnerEditsArchiveAndSecretClaims(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/edit.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	f, _ := family.New(db)
	s, _ := New(db)
	for _, id := range []int64{900001, 900002, 900003, 900004} {
		_ = f.Start(id, "Участник")
	}
	h, _ := f.Create(900001, "Дом", "UTC")
	_, _ = f.Create(900004, "Другой", "UTC")
	invite, _ := f.Invite(900001, time.Now())
	_ = f.Join(900002, invite, time.Now())
	_ = f.Join(900003, invite, time.Now())
	item, e := s.Create(900002, h.ID, "Книга", "https://example.org/book", "Бумажная", "1000")
	if e != nil {
		t.Fatal(e)
	}
	before, _ := s.List(900002, h.ID)
	rawBefore, _ := json.Marshal(before)
	if e = s.Claim(900003, h.ID, item.ID); e != nil {
		t.Fatal(e)
	}
	after, _ := s.List(900002, h.ID)
	rawAfter, _ := json.Marshal(after)
	if !bytes.Equal(rawBefore, rawAfter) {
		t.Fatal("owner learned reservation")
	}
	for _, id := range []int64{900001, 900003, 900004} {
		if e = s.Edit(id, h.ID, item.ID, item.Version, "Чужая правка", "", "", ""); e == nil {
			t.Fatal("non-author edit", id)
		}
		if e = s.Archive(id, h.ID, item.ID, item.Version); e == nil {
			t.Fatal("non-author archive", id)
		}
	}
	if e = s.Edit(900002, h.ID, item.ID, item.Version, "Новая книга", "javascript:alert(1)", "", ""); e == nil {
		t.Fatal("unsafe URL")
	}
	if e = s.Edit(900002, h.ID, item.ID, item.Version, "Другая книга", "https://example.org/new", "Другое издание", "1200"); e != nil {
		t.Fatal(e)
	}
	donor, _ := s.List(900003, h.ID)
	if donor[0].Claim == nil || !donor[0].Claim.Mine || !donor[0].Claim.Changed {
		t.Fatal("reservation lost or change not shown")
	}
	owner, _ := s.List(900002, h.ID)
	if owner[0].Claim != nil || owner[0].Version != item.Version+1 {
		t.Fatal("owner projection")
	}
	if e = s.Edit(900002, h.ID, item.ID, item.Version, "Устарело", "", "", ""); e == nil {
		t.Fatal("stale edit overwrote new wish")
	}
	if e = s.Archive(900002, h.ID, item.ID, item.Version); e == nil {
		t.Fatal("stale archive")
	}
	if e = s.Archive(900002, h.ID, item.ID, item.Version+1); e != nil {
		t.Fatal(e)
	}
	for _, id := range []int64{900001, 900002, 900003} {
		rows, e := s.List(id, h.ID)
		if e != nil || len(rows) != 0 {
			t.Fatal("archived wish still visible", e)
		}
	}
	if e = s.Claim(900001, h.ID, item.ID); e == nil {
		t.Fatal("archived claim")
	}
	if e = s.UpdateClaim(900003, h.ID, item.ID, "purchase"); e == nil {
		t.Fatal("archived purchase")
	}
	var n int64
	db.Model(&ItemClaim{}).Where("item_id=?", item.ID).Count(&n)
	if n != 1 {
		t.Fatal("claim audit deleted")
	}
}

func TestConcurrentEditUsesOneVersion(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/race.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	defer sql.Close()
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	h, _ := f.Create(900001, "Дом", "UTC")
	item, _ := s.Create(900001, h.ID, "Книга", "", "", "")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"Первая", "Вторая"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			results <- s.Edit(900001, h.ID, item.ID, item.Version, name, "", "", "")
		}(name)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
}

func TestLegacyWishlistMigration(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/legacy.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	for _, statement := range []string{
		"CREATE TABLE hq_wishlist_items (id integer PRIMARY KEY, circle_id integer, user_id integer, name text, url text, note text, price text)",
		"CREATE TABLE hq_wishlist_claims (item_id integer PRIMARY KEY, claimed_by_id integer, purchased numeric)",
		"INSERT INTO hq_wishlist_items VALUES (1,1,1,'Книга','','','')",
		"INSERT INTO hq_wishlist_claims VALUES (1,2,1)",
	} {
		if e = db.Exec(statement).Error; e != nil {
			t.Fatal(e)
		}
	}
	if _, e = New(db); e != nil {
		t.Fatal(e)
	}
	var item Item
	var claim ItemClaim
	db.First(&item, 1)
	db.First(&claim, "item_id=?", 1)
	if item.Version != 1 || item.Archived || claim.Version != 1 || !claim.Purchased {
		t.Fatal("legacy wish changed", item, claim)
	}
}
