package agent

import (
	"donetick.com/core/internal/homequest/billing"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"strings"
	"testing"
)

func TestSettingsPlanChecksOwnerAndRuntimeModeAtApply(t *testing.T) {
	s, circle, now := setup(t)
	bs, e := billing.New(s.DB, "manual_test")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = stats.New(s.DB); e != nil {
		t.Fatal(e)
	}
	s.BillingMode = "manual_test"
	f := &family.Service{DB: s.DB}
	token, e := f.Invite(alice, now)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Join(bob, token, now); e != nil {
		t.Fatal(e)
	}
	owner, _ := s.Context(alice, now)
	member, _ := s.Context(bob, now)
	changes := []Change{{Operation: "weekly_goal", Points: 75}, {Operation: "premium_on"}}
	if _, e = s.Prepare(member, "unauthorized", changes, now); e == nil {
		t.Fatal("member could issue entitlement")
	}
	p, e := s.Prepare(owner, "weekly-premium", changes, now)
	if e != nil {
		t.Fatal(e)
	}
	if active, _ := bs.Active(alice, circle, now); active {
		t.Fatal("enabled before confirmation")
	}
	s.BillingMode = "disabled"
	if _, e = s.Apply(alice, p.ID, now); e == nil {
		t.Fatal("old plan bypassed billing mode")
	}
	s.BillingMode = "manual_test"
	if _, e = s.Apply(alice, p.ID, now); e != nil {
		t.Fatal(e)
	}
	if active, e := bs.Active(bob, circle, now); e != nil || !active {
		t.Fatal(active, e)
	}
}

func TestPersistedEmptyOffsetsAndCustomizedPreset(t *testing.T) {
	var change Change
	if err := json.Unmarshal([]byte(`{"operation":"reminder_offsets","offsets":[]}`), &change); err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	var restored Change
	if err = json.Unmarshal(saved, &restored); err != nil || validateFields(restored) != nil {
		t.Fatal("empty reminder plan lost during persistence")
	}
	s, circle, now := setup(t)
	ctx, err := s.Context(alice, now)
	if err != nil {
		t.Fatal(err)
	}
	var custom Change
	if err = json.Unmarshal([]byte(`{"operation":"preset_apply","name":"kitchen","choices":[{"Key":"sink","Name":"Раковина вечером","Enabled":true,"Points":11,"Every":3,"Hour":20,"Minute":25}]}`), &custom); err != nil {
		t.Fatal(err)
	}
	plan, err := s.Prepare(ctx, "custom-kitchen", []Change{custom}, now)
	if err != nil || !strings.Contains(plan.Summary, "20:25") || strings.Contains(plan.Summary, "холодильнике") {
		t.Fatal("preview does not match chosen tasks", err)
	}
	if _, err = s.Apply(alice, plan.ID, now); err != nil {
		t.Fatal(err)
	}
	rows, err := (&task.Service{DB: s.DB}).List(alice, circle)
	if err != nil || len(rows) != 1 {
		t.Fatal("unexpected preset result", len(rows), err)
	}
}
