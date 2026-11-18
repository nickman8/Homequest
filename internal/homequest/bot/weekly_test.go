package bot

import (
	"donetick.com/core/internal/homequest/stats"
	"strings"
	"testing"
	"time"
)

func TestWeeklyGoalSnapshotsAndDigestRestart(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "Europe/Berlin")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	h, _ := j.b.Family.Current(900001)
	s := &stats.Service{DB: j.db}
	if e := s.SetWeeklyGoal(900002, h.ID, 20, j.now); e == nil {
		t.Fatal("member changed goal")
	}
	if e := s.SetWeeklyGoal(900001, h.ID, 20, j.now); e != nil {
		t.Fatal(e)
	}
	before, e := s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || before.Goal != 100 {
		t.Fatal("current goal rewritten", before, e)
	}
	j.send(900001, "/task", "")
	j.send(900001, "Мусор", "")
	j.send(900001, "10", "")
	j.click(900001, "Выполнено")
	j.now = time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)
	if e = j.b.PlanWeekly(); e != nil {
		t.Fatal(e)
	}
	var rows []Delivery
	j.db.Where("event_type=?", "hq_weekly").Find(&rows)
	if len(rows) != 2 {
		t.Fatal("digest recipient count", len(rows))
	}
	for _, r := range rows {
		if !strings.Contains(r.Payload, "10 / 100") {
			t.Fatal("wrong week or goal", r.Payload)
		}
	}
	j.restart()
	j.b.PlanWeekly()
	var n int64
	j.db.Model(&Delivery{}).Where("event_type=?", "hq_weekly").Count(&n)
	if n != 2 {
		t.Fatal("restart duplicated digest")
	}
	after, e := s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || after.Goal != 20 || after.Points != 0 {
		t.Fatal(after, e)
	}
	j.now = j.now.AddDate(0, 0, 14).Add(10 * time.Hour)
	j.b.PlanWeekly()
	j.db.Model(&Delivery{}).Where("event_type=?", "hq_weekly").Count(&n)
	if n != 2 {
		t.Fatal("restart backfilled missed digests")
	}
}

func TestWeeklyGoalSurvivesTimezoneChange(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "Europe/Berlin")
	h, _ := j.b.Family.Current(900001)
	s := &stats.Service{DB: j.db}
	if e := s.SetWeeklyGoal(900001, h.ID, 20, j.now); e != nil {
		t.Fatal(e)
	}
	if e := j.b.Family.SetTimezone(900001, "America/New_York"); e != nil {
		t.Fatal(e)
	}
	r, e := s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || r.Goal != 100 {
		t.Fatal("timezone rewrote current goal", r, e)
	}
	j.now = time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC)
	r, e = s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || r.Goal != 20 {
		t.Fatal(r, e)
	}
	if e = j.b.Family.SetTimezone(900001, "Asia/Tokyo"); e != nil {
		t.Fatal(e)
	}
	r, e = s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || r.Goal != 20 {
		t.Fatal(r, e)
	}
	r, e = s.Weekly(900001, h.ID, j.now, -1)
	if e != nil || r.Goal != 100 {
		t.Fatal("past goal changed", r, e)
	}
}

func TestWeeklyGoalForAnUnvisitedEmptyWeek(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	h, _ := j.b.Family.Current(900001)
	s := &stats.Service{DB: j.db}
	if e := s.SetWeeklyGoal(900001, h.ID, 20, j.now); e != nil {
		t.Fatal(e)
	}
	// No task, view or worker ran during the week starting March 2.
	j.now = time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	r, e := s.Weekly(900001, h.ID, j.now, -1)
	if e != nil || r.Goal != 20 {
		t.Fatal("unvisited week lost configured goal", r, e)
	}
	if e = s.SetWeeklyGoal(900001, h.ID, 30, j.now); e != nil {
		t.Fatal(e)
	}
	r, e = s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || r.Goal != 20 {
		t.Fatal("current week rewritten", r, e)
	}
	j.now = j.now.AddDate(0, 0, 7)
	r, e = s.Weekly(900001, h.ID, j.now, 0)
	if e != nil || r.Goal != 30 {
		t.Fatal("next week goal missing", r, e)
	}
}
