package agent

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
	"time"
)

const alice int64 = 910001
const bob int64 = 910002
const stranger int64 = 910004

func setup(t *testing.T) (*Service, int, time.Time) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/agent.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	if _, e = task.New(db); e != nil {
		t.Fatal(e)
	}
	for _, id := range []int64{alice, bob, stranger} {
		if e = f.Start(id, "Участник"); e != nil {
			t.Fatal(e)
		}
	}
	h, e := f.Create(alice, "Тёплый дом", "Europe/Berlin")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	s, e := New(db)
	if e != nil {
		t.Fatal(e)
	}
	return s, h.ID, now
}
func TestContextPersistenceIsolationAndInvalidation(t *testing.T) {
	s, c, now := setup(t)
	tasks := &task.Service{DB: s.DB}
	room, e := tasks.CreateRoom(alice, c, "Кухня")
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.Context(alice, now)
	if e != nil {
		t.Fatal(e)
	}
	selected, e := s.SelectRoom(alice, room.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	if selected.RoomID != room.ID || selected.Revision <= before.Revision {
		t.Fatal("room not selected")
	}
	again, e := (&Service{DB: s.DB}).Context(alice, now)
	if e != nil || again.RoomID != room.ID {
		t.Fatal("restart lost focus", e)
	}
	if _, e = s.SelectRoom(stranger, room.ID, now); e == nil {
		t.Fatal("foreign room accepted")
	}
	if e = s.Check(before, now); e == nil {
		t.Fatal("old revision accepted")
	}
	if e = tasks.DeleteRoom(alice, c, room.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Check(selected, now); e == nil {
		t.Fatal("deleted room accepted")
	}
	refreshed, e := s.Context(alice, now)
	if e != nil || refreshed.RoomID != 0 {
		t.Fatal("deleted focus not cleared", e)
	}
}
func TestPlanPreviewAtomicApplyReplayAndStale(t *testing.T) {
	s, c, now := setup(t)
	ts := &task.Service{DB: s.DB}
	room, _ := ts.CreateRoom(alice, c, "Кухня")
	a, _ := ts.Create(alice, c, "Раковина", 10, nil)
	b, _ := ts.Create(alice, c, "Пол", 5, nil)
	ts.SetRoom(alice, c, a.ID, room.ID)
	ts.SetRoom(alice, c, b.ID, room.ID)
	snap, _ := s.SelectRoom(alice, room.ID, now)
	rows, e := s.Tasks(snap, 0, now)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Prepare(snap, "request-1", []Change{{Operation: "task_due", ID: a.ID, Version: rows[0].Version, Due: "2026-11-17 19:00"}, {Operation: "task_due", ID: b.ID, Version: rows[1].Version, Due: "2026-11-17 19:00"}}, now)
	if e != nil {
		t.Fatal(e)
	}
	var i task.Instance
	s.DB.First(&i, a.ID)
	if i.DueAt != nil {
		t.Fatal("preview wrote due")
	}
	out, e := s.Apply(alice, p.ID, now)
	if e != nil || len(out) != 2 {
		t.Fatal("apply failed", e)
	}
	s.DB.First(&i, a.ID)
	if i.DueAt == nil || i.DueAt.Hour() != 18 {
		t.Fatal("local due not respected")
	}
	v := i.Version
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal("replay", e)
	}
	s.DB.First(&i, a.ID)
	if i.Version != v {
		t.Fatal("duplicate effect")
	}
	rows, _ = s.Tasks(snap, 0, now)
	stale, e := s.Prepare(snap, "request-2", []Change{{Operation: "task_due", ID: a.ID, Version: rows[0].Version, Due: "2026-11-18 19:00"}, {Operation: "task_due", ID: b.ID, Version: rows[1].Version, Due: "2026-11-18 19:00"}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = ts.Edit(alice, c, b.ID, "Новый пол", 20); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, stale.ID, now); e == nil {
		t.Fatal("stale plan accepted")
	}
	s.DB.First(&i, a.ID)
	if i.Version != v {
		t.Fatal("partial stale batch committed")
	}
	if _, e = s.Apply(stranger, p.ID, now); e == nil {
		t.Fatal("foreign plan accepted")
	}
}

func TestPreviewRollbackPermissionsAndDuplicateConcurrentApply(t *testing.T) {
	s, c, now := setup(t)
	ts := &task.Service{DB: s.DB}
	f := &family.Service{DB: s.DB}
	invitation, e := f.Invite(alice, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Join(bob, invitation, now); e != nil {
		t.Fatal(e)
	}
	item, e := ts.Create(alice, c, "Пол", 10, nil)
	if e != nil {
		t.Fatal(e)
	}
	scope, _ := s.Context(alice, now)
	rows, _ := s.Tasks(scope, 0, now)
	// Configure + completion in one dry-run must not leak attempts or ledger writes.
	p, e := s.Prepare(scope, "photo-batch", []Change{{Operation: "task_configure", ID: item.ID, Version: rows[0].Version, Approval: true, Photo: true}, {Operation: "task_complete", ID: item.ID, Version: rows[0].Version}}, now)
	if e != nil {
		t.Fatal(e)
	}
	var count int64
	s.DB.Model(&task.Attempt{}).Count(&count)
	if count != 0 {
		t.Fatal("preview leaked attempt")
	}
	errors := make(chan error, 2)
	for n := 0; n < 2; n++ {
		go func() { _, err := s.Apply(alice, p.ID, now); errors <- err }()
	}
	for n := 0; n < 2; n++ {
		if e = <-errors; e != nil {
			t.Fatal(e)
		}
	}
	s.DB.Model(&task.Attempt{}).Count(&count)
	if count != 1 {
		t.Fatal("concurrent duplicate attempts", count)
	}
	balance, e := ts.Balance(alice, c)
	if e != nil || balance != 0 {
		t.Fatal("photo/approval bypass", balance, e)
	}
	var a task.Attempt
	s.DB.First(&a)
	if a.Status != "photo" {
		t.Fatal(a.Status)
	}
	// A member cannot prepare a change to the owner's other task.
	other, _ := ts.Create(alice, c, "Другой пол", 5, nil)
	member, _ := s.Context(bob, now)
	if _, e = s.Prepare(member, "forbidden", []Change{{Operation: "task_edit", ID: other.ID, Version: 1, Points: 999}}, now); e == nil {
		t.Fatal("member edited owner task")
	}
	// Earlier valid writes also roll back when a later domain operation fails.
	if _, e = s.Prepare(scope, "invalid-batch", []Change{{Operation: "task_due", ID: other.ID, Version: 1, Due: "2026-11-20 19:00"}, {Operation: "task_assign", ID: other.ID, Version: 1, Target: 99999}}, now); e == nil {
		t.Fatal("foreign assignee accepted")
	}
	var i task.Instance
	s.DB.First(&i, other.ID)
	if i.Version != 1 || i.DueAt != nil {
		t.Fatal("failed preview wrote task")
	}
}

func TestMemoryTTLAndPlanApplyAfterRevocation(t *testing.T) {
	s, c, now := setup(t)
	ts := &task.Service{DB: s.DB}
	f := &family.Service{DB: s.DB}
	invite, e := f.Invite(alice, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Join(bob, invite, now); e != nil {
		t.Fatal(e)
	}
	item, _ := ts.Create(bob, c, "Моё дело", 5, nil)
	scope, _ := s.Context(bob, now)
	p, e := s.Prepare(scope, "member-plan", []Change{{Operation: "task_due", ID: item.ID, Version: 1, Due: "2026-11-20 19:00"}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Remember(scope, []int{item.ID}, "синтетическая реплика", "ответ", now); e != nil {
		t.Fatal(e)
	}
	if e = f.Leave(bob); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(bob, p.ID, now); e == nil {
		t.Fatal("departed member applied plan")
	}
	if e = s.Purge(now.Add(25 * time.Hour)); e != nil {
		t.Fatal(e)
	}
	var sess Session
	s.DB.First(&sess, "viewer=?", bob)
	if sess.History != "" || sess.References != "" {
		t.Fatal("expired memory retained")
	}
	var expired Plan
	s.DB.First(&expired, "id=?", p.ID)
	if expired.Changes != "" || expired.Summary != "" || expired.Result != "" {
		t.Fatal("expired plan retained content")
	}
}

func TestPlanCannotSilentlyIgnoreRequestedFields(t *testing.T) {
	s, _, now := setup(t)
	scope, _ := s.Context(alice, now)
	if _, e := s.Prepare(scope, "unsupported-create-field", []Change{{Operation: "task_create", Name: "Обязательное фото", Points: 10, Photo: true}}, now); e == nil {
		t.Fatal("photo requirement silently discarded")
	}
	var n int64
	s.DB.Model(&task.Instance{}).Count(&n)
	if n != 0 {
		t.Fatal("invalid plan created task")
	}
}

func TestPreviewNamesActualTargetRoom(t *testing.T) {
	s, c, now := setup(t)
	ts := &task.Service{DB: s.DB}
	kitchen, _ := ts.CreateRoom(alice, c, "Кухня")
	bath, _ := ts.CreateRoom(alice, c, "Ванная")
	item, _ := ts.Create(alice, c, "Помыть раковину", 10, nil)
	if e := ts.SetRoom(alice, c, item.ID, bath.ID); e != nil {
		t.Fatal(e)
	}
	scope, _ := s.SelectRoom(alice, kitchen.ID, now)
	p, e := s.Prepare(scope, "explicit-other-room", []Change{{Operation: "task_due", ID: item.ID, Version: 2, Due: "2026-11-17 19:00"}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.Summary, "Ванная") {
		t.Fatal("preview hides the actual target room")
	}
}
