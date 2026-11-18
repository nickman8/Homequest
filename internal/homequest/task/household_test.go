package task

import (
	ch "donetick.com/core/internal/chore/model"
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sync"
	"testing"
	"time"
)

func householdFixture(t *testing.T) (*Service, *family.Service, int, time.Time) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/house.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	f, e := family.New(db)
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, id := range []int64{900001, 900002, 900003, 900004} {
		if e = f.Start(id, "Участник"); e != nil {
			t.Fatal(e)
		}
	}
	h, e := f.Create(900001, "Дом", "Europe/Berlin")
	if e != nil {
		t.Fatal(e)
	}
	token, _ := f.Invite(900001, now)
	for _, id := range []int64{900002, 900003} {
		if e = f.Join(id, token, now); e != nil {
			t.Fatal(e)
		}
	}
	f.Create(900004, "Чужой", "UTC")
	return s, f, h.ID, now
}
func TestRoomsPresetsAtomicScopeAndHistory(t *testing.T) {
	s, _, c, now := householdFixture(t)
	r, e := s.CreateRoom(900002, c, " Кухня ")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.CreateRoom(900001, c, "кухня")
	if e != nil || again.ID != r.ID {
		t.Fatal("room canonical duplicate", again, e)
	}
	if _, e = s.Rooms(900004, c); e == nil {
		t.Fatal("foreign rooms")
	}
	choices := PresetTasks("kitchen")
	if len(choices) < 2 {
		t.Fatal("preset empty")
	}
	choices[0].Points = 15
	ids, e := s.ApplyPreset(900002, c, "kitchen", choices, now)
	if e != nil {
		t.Fatal(e)
	}
	againIDs, e := s.ApplyPreset(900002, c, "kitchen", choices, now)
	if e != nil || len(againIDs) != 0 {
		t.Fatal("duplicate preset", e)
	}
	rows, e := s.RoomTasks(900001, c, r.ID)
	if e != nil || len(rows) != len(ids) {
		t.Fatal(rows, e)
	}
	if e = s.RenameRoom(900001, c, r.ID, "Зона кухни"); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteRoom(900002, c, r.ID); e != nil {
		t.Fatal(e)
	}
	rows, e = s.List(900001, c)
	if e != nil || len(rows) != len(ids) {
		t.Fatal("room deletion deleted tasks")
	}
	var n int64
	s.DB.Model(&ch.Chore{}).Count(&n)
	bad := PresetTasks("bathroom")
	bad[1].Points = -1
	if _, e = s.ApplyPreset(900001, c, "bathroom", bad, now); e == nil {
		t.Fatal("bad preset accepted")
	}
	var after int64
	s.DB.Model(&ch.Chore{}).Count(&after)
	if after != n {
		t.Fatal("partial import")
	}
}
func TestRotationAcceptanceDepartureAndHandoffRace(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	d, _ := f.Authorize(900003, c)
	item, _ := s.Create(900001, c, "Мусор", 10, nil)
	if e := s.SetSchedule(900001, c, item.ID, Schedule{Mode: "daily", Hour: 19}, now); e != nil {
		t.Fatal(e)
	}
	if e := s.SetRotation(900002, c, item.ID, []int{a.ID, b.ID}); e == nil {
		t.Fatal("non-author rotation")
	}
	if e := s.SetRotation(900001, c, item.ID, []int{a.ID, b.ID, d.ID}); e != nil {
		t.Fatal(e)
	}
	if e := s.Configure(900001, c, item.ID, true, false); e != nil {
		t.Fatal(e)
	}
	attempt, e := s.Submit(900001, c, item.ID, "", now)
	if e != nil {
		t.Fatal(e)
	}
	var def ch.Chore
	var inst Instance
	s.DB.First(&inst, item.ID)
	s.DB.First(&def, inst.ChoreID)
	if *def.AssignedTo != a.ID {
		t.Fatal("rotated before acceptance")
	}
	if e = s.Review(900002, c, attempt.ID, true, now.AddDate(0, 0, 20)); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.List(900001, c)
	if len(rows) != 1 || *rows[0].AssignedTo != b.ID {
		t.Fatal("no rotation", rows)
	}
	next := rows[0].ID
	offer, e := s.OfferHandoff(900002, c, next, d.ID, now.AddDate(0, 0, 20))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResolveHandoff(900001, c, offer.ID, "accept", now.AddDate(0, 0, 20)); e == nil {
		t.Fatal("wrong recipient")
	}
	var wg sync.WaitGroup
	for _, op := range []string{"accept", "decline"} {
		wg.Add(1)
		go func(op string) { defer wg.Done(); s.ResolveHandoff(900003, c, offer.ID, op, now.AddDate(0, 0, 20)) }(op)
	}
	wg.Wait()
	var stored Handoff
	s.DB.First(&stored, offer.ID)
	if stored.Status != "accepted" && stored.Status != "declined" {
		t.Fatal(stored)
	}
	old := stored.Status
	s.ResolveHandoff(900003, c, offer.ID, "accept", now.AddDate(0, 0, 20))
	s.DB.First(&stored, offer.ID)
	if stored.Status != old {
		t.Fatal("repeat changed decision")
	}
	if e = f.Remove(900001, d.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Handoffs(900003, c, now); e == nil {
		t.Fatal("departed access")
	}
}

func TestRotationUndoRestoresOriginalTurn(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	item, _ := s.Create(900001, c, "Очередь", 10, nil)
	s.SetSchedule(900001, c, item.ID, Schedule{Mode: "daily", Hour: 19}, now)
	s.SetRotation(900001, c, item.ID, []int{a.ID, b.ID})
	attempt, e := s.Submit(900001, c, item.ID, "", now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Undo(900001, c, attempt.ID, now.Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	rows, e := s.List(900001, c)
	if e != nil || len(rows) != 1 || rows[0].AssignedTo == nil || *rows[0].AssignedTo != a.ID {
		t.Fatal("undo failed to restore original turn", rows, e)
	}
}

func TestRotationCannotScheduleSelfReviewerAndStaleHandoff(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	item, _ := s.Create(900001, c, "Очередь", 10, nil)
	s.Configure(900001, c, item.ID, true, false)
	s.SetReviewer(900001, c, item.ID, a.ID)
	if e := s.SetRotation(900001, c, item.ID, []int{a.ID, b.ID}); e == nil {
		t.Fatal("rotation made self-review unavoidable")
	}
	s.SetReviewer(900001, c, item.ID, 0)
	s.Configure(900001, c, item.ID, false, false)
	s.Manage(900001, c, item.ID, "assign", a.ID)
	h, e := s.OfferHandoff(900001, c, item.ID, b.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Submit(900001, c, item.ID, "", now); e != nil {
		t.Fatal(e)
	}
	offers, e := s.Handoffs(900002, c, now)
	if e != nil || len(offers) != 0 {
		t.Fatal("closed task remains actionable", offers, e)
	}
	if e = s.ResolveHandoff(900002, c, h.ID, "accept", now); e == nil {
		t.Fatal("closed handoff accepted")
	}
}

func TestRotationSkipsDepartedCurrentAssignee(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	d, _ := f.Authorize(900003, c)
	item, _ := s.Create(900001, c, "Очередь", 10, nil)
	s.SetSchedule(900001, c, item.ID, Schedule{Mode: "daily", Hour: 19}, now)
	s.SetRotation(900001, c, item.ID, []int{a.ID, b.ID, d.ID})
	s.Submit(900001, c, item.ID, "", now)
	f.Remove(900001, b.ID)
	rows, e := s.List(900001, c)
	if e != nil || len(rows) != 1 || rows[0].AssignedTo == nil || *rows[0].AssignedTo != d.ID {
		t.Fatal("departed assignee trapped the task", rows, e)
	}
}

func TestExpiredHandoffPreservesAssignment(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	item, e := s.Create(900001, c, "Дело", 10, &a.ID)
	if e != nil {
		t.Fatal(e)
	}
	offer, e := s.OfferHandoff(900001, c, item.ID, b.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResolveHandoff(900002, c, offer.ID, "accept", now.AddDate(0, 0, 8)); e == nil {
		t.Fatal("expired accepted")
	}
	rows, e := s.List(900001, c)
	if e != nil || len(rows) != 1 || *rows[0].AssignedTo != a.ID {
		t.Fatal(rows, e)
	}
}

func TestReviewerCannotBecomeFutureRotatingPerformer(t *testing.T) {
	s, f, c, now := householdFixture(t)
	a, _ := f.Authorize(900001, c)
	b, _ := f.Authorize(900002, c)
	item, _ := s.Create(900001, c, "Очередь", 10, nil)
	s.SetSchedule(900001, c, item.ID, Schedule{Mode: "daily", Hour: 19}, now)
	if e := s.SetRotation(900001, c, item.ID, []int{a.ID, b.ID}); e != nil {
		t.Fatal(e)
	}
	if e := s.SetReviewer(900001, c, item.ID, b.ID); e == nil {
		t.Fatal("future performer became reviewer")
	}
	other, _ := s.Create(900001, c, "Вторая очередь", 10, nil)
	if e := s.SetReviewer(900001, c, other.ID, b.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.SetRotation(900001, c, other.ID, []int{a.ID, b.ID}); e == nil {
		t.Fatal("disabled approval hid reviewer conflict")
	}
}
