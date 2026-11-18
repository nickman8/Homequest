package agent

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/task"
	"donetick.com/core/internal/homequest/wishlist"
	"strings"
	"testing"
)

func TestShoppingAgentLifecycleAndStaleAtomicPlan(t *testing.T) {
	s, c, now := setup(t)
	sh, e := shopping.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	scope, _ := s.Context(alice, now)
	if e = sh.CreateBatch(alice, c, "seed", []shopping.Input{{Name: "Молоко", Quantity: "1 л"}}, now); e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"claim", "release", "edit", "buy", "reopen", "archive"} {
		rows, e := s.Resources(scope, "shopping", now)
		if e != nil || len(rows) != 1 {
			t.Fatal(rows, e)
		}
		change := Change{Operation: "shopping_" + op, ID: rows[0].ID, Snapshot: rows[0].Snapshot}
		if op == "edit" {
			change.Items = []shopping.Input{{Name: "Молоко", Quantity: "2 л"}}
		}
		p, e := s.Prepare(scope, "shopping-"+op, []Change{change}, now)
		if e != nil {
			t.Fatal(op, e)
		}
		before, _ := s.Resources(scope, "shopping", now)
		if before[0].Snapshot != rows[0].Snapshot {
			t.Fatal("preview mutated shopping")
		}
		if _, e = s.Apply(alice, p.ID, now); e != nil {
			t.Fatal(op, e)
		}
		if _, e = s.Apply(alice, p.ID, now); e != nil {
			t.Fatal("replay", op, e)
		}
	}
	if e = sh.CreateBatch(alice, c, "seed2", []shopping.Input{{Name: "Хлеб"}}, now); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.Resources(scope, "shopping", now)
	p, e := s.Prepare(scope, "stale", []Change{{Operation: "room_create", Name: "Кладовая"}, {Operation: "shopping_buy", ID: rows[0].ID, Snapshot: rows[0].Snapshot}}, now)
	if e != nil {
		t.Fatal(e)
	}
	items, _ := sh.List(alice, c, false)
	if e = sh.Change(alice, c, items[0].ID, items[0].Version, "edit", &shopping.Input{Name: "Другой хлеб"}, now); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, p.ID, now); e == nil {
		t.Fatal("stale snapshot accepted")
	}
	rooms, _ := (&task.Service{DB: s.DB}).Rooms(alice, c)
	if len(rooms) != 0 {
		t.Fatal("partial plan committed")
	}
	foreign := scope
	foreign.Viewer = stranger
	if _, e = s.Resources(foreign, "shopping", now); e == nil {
		t.Fatal("foreign read")
	}
}

func TestRewardAgentSnapshotSpendRefundAndRights(t *testing.T) {
	s, c, now := setup(t)
	rw, e := reward.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	f := &family.Service{DB: s.DB}
	invite, _ := f.Invite(alice, now)
	if e = f.Join(bob, invite, now); e != nil {
		t.Fatal(e)
	}
	ts := &task.Service{DB: s.DB}
	item, e := ts.Create(alice, c, "Дело", 100, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ts.Submit(bob, c, item.ID, "", now); e != nil {
		t.Fatal(e)
	}
	a, _ := s.Context(alice, now)
	b, _ := s.Context(bob, now)
	p, e := s.Prepare(a, "catalog", []Change{{Operation: "reward_create", Name: "Кино", Points: 30, Description: "Выбрать фильм"}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.Resources(b, "reward", now)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	change := Change{Operation: "reward_request", ID: rows[0].ID, Snapshot: rows[0].Snapshot}
	p, e = s.Prepare(b, "old-price", []Change{change}, now)
	if e != nil {
		t.Fatal(e)
	}
	balance, _ := ts.Balance(bob, c)
	if balance != 100 {
		t.Fatal("preview spent balance")
	}
	if e = rw.Edit(alice, c, rows[0].ID, "Кино", 40); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(bob, p.ID, now); e == nil {
		t.Fatal("changed price accepted")
	}
	rows, _ = s.Resources(b, "reward", now)
	change.Snapshot = rows[0].Snapshot
	p, e = s.Prepare(b, "request", []Change{change}, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(bob, p.ID, now); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(bob, p.ID, now); e != nil {
		t.Fatal(e)
	}
	balance, _ = ts.Balance(bob, c)
	if balance != 60 {
		t.Fatal(balance)
	}
	requests, _ := s.Resources(b, "redemption", now)
	resolve := Change{Operation: "redemption_approve", ID: requests[0].ID, Snapshot: requests[0].Snapshot}
	if _, e = s.Prepare(b, "self-approval", []Change{resolve}, now); e == nil {
		t.Fatal("self approval")
	}
	p, e = s.Prepare(a, "approve", []Change{resolve}, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	// Catalog edits do not change the request snapshot or its refund.
	if e = rw.Edit(alice, c, rows[0].ID, "Другой фильм", 80); e != nil {
		t.Fatal(e)
	}
	requests, _ = s.Resources(a, "redemption", now)
	resolve = Change{Operation: "redemption_reject", ID: requests[0].ID, Snapshot: requests[0].Snapshot}
	p, e = s.Prepare(a, "refund", []Change{resolve}, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	balance, _ = ts.Balance(bob, c)
	if balance != 100 {
		t.Fatal("refund mismatch", balance)
	}
	// Family scope is checked even for a correctly guessed snapshot.
	other, _ := f.Create(stranger, "Другой дом", "UTC")
	_ = other
	d, _ := s.Context(stranger, now)
	if _, e = s.Prepare(d, "foreign", []Change{resolve}, now); e == nil {
		t.Fatal("foreign redemption")
	}
}

func TestCreateConfiguredTaskAtomically(t *testing.T) {
	s, c, now := setup(t)
	f := &family.Service{DB: s.DB}
	inv, _ := f.Invite(alice, now)
	if e := f.Join(bob, inv, now); e != nil {
		t.Fatal(e)
	}
	a, _ := f.Authorize(alice, c)
	b, _ := f.Authorize(bob, c)
	scope, _ := s.Context(alice, now)
	change := Change{Operation: "task_create", Name: "Помыть плиту", Points: 25, Description: "Сначала остудить", AssignedTo: &b.ID, Reviewer: &a.ID, Approval: true, Photo: true, Due: "2026-11-17 19:00"}
	p, e := s.Prepare(scope, "rich-create", []Change{change}, now)
	if e != nil {
		t.Fatal(e)
	}
	rows, _ := (&task.Service{DB: s.DB}).List(alice, c)
	if len(rows) != 0 {
		t.Fatal("preview created task")
	}
	out, e := s.Apply(alice, p.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	rows, _ = (&task.Service{DB: s.DB}).List(alice, c)
	if len(rows) != 1 || rows[0].AssignedTo == nil || *rows[0].AssignedTo != b.ID {
		t.Fatal("assignment lost", rows)
	}
	attempt, e := (&task.Service{DB: s.DB}).Submit(bob, c, out[0].ID, "", now)
	if e != nil {
		t.Fatal(e)
	}
	if attempt.Status != "photo" || !attempt.Approval || attempt.ReviewerID == nil || *attempt.ReviewerID != a.ID {
		t.Fatal("requirements lost", attempt)
	}
	// A failing last step rolls back the new task and its schedule too.
	change.Reviewer = &b.ID
	if _, e = s.Prepare(scope, "bad-self-review", []Change{change}, now); e == nil {
		t.Fatal("self reviewer accepted")
	}
	var total int64
	s.DB.Model(&task.Instance{}).Where("circle_id=?", c).Count(&total)
	if total != 1 {
		t.Fatal("invalid create leaked task", total)
	}
}

func TestShoppingConcurrentPlansHaveOneWinner(t *testing.T) {
	s, c, now := setup(t)
	sh, e := shopping.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	f := &family.Service{DB: s.DB}
	invite, _ := f.Invite(alice, now)
	if e = f.Join(bob, invite, now); e != nil {
		t.Fatal(e)
	}
	if e = sh.CreateBatch(alice, c, "race", []shopping.Input{{Name: "Хлеб"}}, now); e != nil {
		t.Fatal(e)
	}
	plans := []Plan{}
	for _, uid := range []int64{alice, bob} {
		scope, _ := s.Context(uid, now)
		rows, _ := s.Resources(scope, "shopping", now)
		p, e := s.Prepare(scope, "buy", []Change{{Operation: "shopping_buy", ID: rows[0].ID, Snapshot: rows[0].Snapshot}}, now)
		if e != nil {
			t.Fatal(e)
		}
		plans = append(plans, p)
	}
	results := make(chan error, 2)
	for _, p := range plans {
		go func(p Plan) { _, e := s.Apply(p.Viewer, p.ID, now); results <- e }(p)
	}
	wins := 0
	for range plans {
		if <-results == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("race winners", wins)
	}
	var n int64
	s.DB.Model(&shopping.Event{}).Where("action=?", "buy").Count(&n)
	if n != 1 {
		t.Fatal("purchase recorded twice", n)
	}
}

func TestRewardApplyRechecksAvailableBalanceAndDeparture(t *testing.T) {
	s, c, now := setup(t)
	rw, e := reward.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	f := &family.Service{DB: s.DB}
	invite, _ := f.Invite(alice, now)
	if e = f.Join(bob, invite, now); e != nil {
		t.Fatal(e)
	}
	ts := &task.Service{DB: s.DB}
	i, _ := ts.Create(alice, c, "Работа", 50, nil)
	if _, e = ts.Submit(bob, c, i.ID, "", now); e != nil {
		t.Fatal(e)
	}
	r, _ := rw.Create(alice, c, "Награда", "", 30)
	scope, _ := s.Context(bob, now)
	rows, _ := s.Resources(scope, "reward", now)
	p, e := s.Prepare(scope, "request", []Change{{Operation: "reward_request", ID: r.ID, Snapshot: rows[0].Snapshot}}, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = rw.Request(bob, c, r.ID, "ordinary-button", now); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(bob, p.ID, now); e == nil {
		t.Fatal("overspent balance")
	}
	balance, _ := ts.Balance(bob, c)
	if balance != 20 {
		t.Fatal(balance)
	}
	requests, _ := rw.Requests(bob, c)
	if len(requests) != 1 {
		t.Fatal("failed apply left request")
	}
	// Previously valid resource read is unusable after membership removal.
	if e = f.Leave(bob); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Resources(scope, "redemption", now); e == nil {
		t.Fatal("departed reader")
	}
	if _, e = s.Apply(bob, p.ID, now); e == nil {
		t.Fatal("departed applicant")
	}
}

func TestAgentAccountSeparatesSpendingFromContribution(t *testing.T) {
	s, c, now := setup(t)
	rw, e := reward.New(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	ts := &task.Service{DB: s.DB}
	i, _ := ts.Create(alice, c, "Работа", 50, nil)
	if _, e = ts.Submit(alice, c, i.ID, "", now); e != nil {
		t.Fatal(e)
	}
	r, _ := rw.Create(alice, c, "Награда", "", 30)
	if _, e = rw.Request(alice, c, r.ID, "spend", now); e != nil {
		t.Fatal(e)
	}
	scope, _ := s.Context(alice, now)
	account, e := s.Account(scope, now)
	if e != nil {
		t.Fatal(e)
	}
	if account.Summary.Balance != 20 || account.Summary.Week != 50 || account.Summary.XP != 50 || len(account.Ranking) != 1 {
		t.Fatal(account)
	}
	scope.Viewer = stranger
	if _, e = s.Account(scope, now); e == nil {
		t.Fatal("foreign account read")
	}
}

func TestAgentGiftSurpriseAndRoomLifecycle(t *testing.T) {
	s, _, now := setup(t)
	_, err := wishlist.New(s.DB)
	if err != nil {
		t.Fatal(err)
	}
	f := &family.Service{DB: s.DB}
	invite, err := f.Invite(alice, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Join(bob, invite, now); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Context(alice, now)
	b, _ := s.Context(bob, now)
	apply := func(who Context, ch Change) {
		t.Helper()
		p, e := s.Prepare(who, ch.Operation, []Change{ch}, now)
		if e != nil {
			t.Fatal(ch.Operation, e)
		}
		if _, e = s.Apply(who.Viewer, p.ID, now); e != nil {
			t.Fatal(e)
		}
	}
	apply(a, Change{Operation: "wish_create", Name: "Атлас"})
	read := func(who Context, kind string) Resource {
		t.Helper()
		rows, e := s.Resources(who, kind, now)
		if e != nil || len(rows) != 1 {
			t.Fatal(rows, e)
		}
		return rows[0]
	}
	owner := read(a, "wish")
	for _, op := range []string{"wish_claim", "wish_purchase", "wish_release"} {
		gift := read(b, "wish")
		apply(b, Change{Operation: op, ID: gift.ID, Snapshot: gift.Snapshot})
		if got := read(a, "wish"); got != owner {
			t.Fatalf("recipient projection changed after %s", op)
		}
	}
	if _, e := s.Prepare(a, "self", []Change{{Operation: "wish_claim", ID: owner.ID, Snapshot: owner.Snapshot}}, now); e == nil {
		t.Fatal("self claim")
	}
	apply(a, Change{Operation: "wish_edit", ID: owner.ID, Snapshot: owner.Snapshot, Name: "Атлас мира"})
	if _, e := s.Prepare(a, "stale", []Change{{Operation: "wish_archive", ID: owner.ID, Snapshot: owner.Snapshot}}, now); e == nil {
		t.Fatal("stale item")
	}
	owner = read(a, "wish")
	apply(a, Change{Operation: "wish_archive", ID: owner.ID, Snapshot: owner.Snapshot})
	apply(a, Change{Operation: "room_create", Name: "Кухня"})
	room := read(a, "room")
	apply(a, Change{Operation: "room_rename", ID: room.ID, Snapshot: room.Snapshot, Name: "Столовая"})
	room = read(a, "room")
	apply(a, Change{Operation: "room_delete", ID: room.ID, Snapshot: room.Snapshot})
}

func TestAtomicTaskInNewRoom(t *testing.T) {
	s, c, now := setup(t)
	a, _ := s.Context(alice, now)
	plan := []Change{{Operation: "room_create", Name: "Кухня"}, {Operation: "task_create", Name: "Помыть чашку", Points: 3, RoomFromStep: 1}}
	p, e := s.Prepare(a, "kitchen", plan, now)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.Summary, "3 баллов · Кухня") {
		t.Fatal(p.Summary)
	}
	result, e := s.Apply(alice, p.ID, now)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := (&task.Service{DB: s.DB}).RoomTasks(alice, c, result[0].ID)
	if e != nil || len(rows) != 1 || rows[0].ID != result[1].ID {
		t.Fatal(rows, e)
	}
	plan[1].RoomFromStep = 2
	if _, e = s.Prepare(a, "bad-ref", plan, now); e == nil {
		t.Fatal("accepted forward reference")
	}
}
