package agent

import (
	"encoding/json"
	"testing"

	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/task"
)

func TestReplyResourceScopeEnforcedByPrepareAndApply(t *testing.T) {
	for _, fault := range []string{"other_id", "other_kind", "new_entity", "changed_before_prepare", "changed_after_preview", "tampered_plan", "removed_member"} {
		t.Run(fault, func(t *testing.T) {
			s, c, now := setup(t)
			rw, e := reward.New(s.DB)
			if e != nil {
				t.Fatal(e)
			}
			first, e := rw.Create(alice, c, "Кино", "", 10)
			if e != nil {
				t.Fatal(e)
			}
			second, e := rw.Create(alice, c, "Чай", "", 10)
			if e != nil {
				t.Fatal(e)
			}
			scope, e := s.Context(alice, now)
			if e != nil {
				t.Fatal(e)
			}
			target, e := s.CaptureReply(alice, c, "reward", first.ID, now)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.BindReply(&scope, target, now); e != nil {
				t.Fatal(e)
			}
			change := Change{Operation: "reward_archive", ID: first.ID, Snapshot: target.Snapshot}
			switch fault {
			case "other_id":
				other, e := s.CaptureReply(alice, c, "reward", second.ID, now)
				if e != nil {
					t.Fatal(e)
				}
				change.ID, change.Snapshot = second.ID, other.Snapshot
			case "other_kind":
				r, e := (&task.Service{DB: s.DB}).Create(alice, c, "Пол", 10, nil)
				if e != nil {
					t.Fatal(e)
				}
				change = Change{Operation: "task_complete", ID: r.ID, Version: 1}
			case "new_entity":
				change = Change{Operation: "reward_create", Name: "Новое", Points: 10}
			case "changed_before_prepare":
				if e = rw.Edit(alice, c, first.ID, "Кино", 20, ""); e != nil {
					t.Fatal(e)
				}
			}
			p, e := s.Prepare(scope, "scoped-reply", []Change{change}, now)
			switch fault {
			case "other_id", "other_kind", "new_entity", "changed_before_prepare":
				if e == nil {
					t.Fatal("invalid scoped plan prepared", fault)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			switch fault {
			case "changed_after_preview":
				if e = rw.Edit(alice, c, first.ID, "Кино", 20, ""); e != nil {
					t.Fatal(e)
				}
			case "tampered_plan":
				other, e := s.CaptureReply(alice, c, "reward", second.ID, now)
				if e != nil {
					t.Fatal(e)
				}
				raw, _ := json.Marshal([]Change{{Operation: "reward_archive", ID: second.ID, Snapshot: other.Snapshot}})
				if e = s.DB.Model(&Plan{}).Where("id=?", p.ID).Update("changes", string(raw)).Error; e != nil {
					t.Fatal(e)
				}
			case "removed_member":
				f := &family.Service{DB: s.DB}
				invite, e := f.Invite(alice, now)
				if e != nil {
					t.Fatal(e)
				}
				if e = f.Join(bob, invite, now); e != nil {
					t.Fatal(e)
				}
				successor, e := f.Authorize(bob, c)
				if e != nil {
					t.Fatal(e)
				}
				if e = f.Transfer(alice, successor.ID); e != nil {
					t.Fatal(e)
				}
				if e = f.Leave(alice); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = s.Apply(alice, p.ID, now); e == nil {
				t.Fatal("invalid scoped plan applied", fault)
			}
		})
	}
}
