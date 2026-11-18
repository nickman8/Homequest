package task

import (
	"donetick.com/core/internal/homequest/family"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestLateApprovalCreatesOnlyFutureInstance(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/repeat.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	_ = f.Start(900002, "B")
	h, _ := f.Create(900001, "Дом", "Europe/Berlin")
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	token, _ := f.Invite(900001, now)
	_ = f.Join(900002, token, now)
	item, _ := s.Create(900001, h.ID, "Мусор", 10, nil)
	if e = s.SetSchedule(900001, h.ID, item.ID, Schedule{Mode: "weekdays", Days: []string{"tuesday"}, Hour: 19}, now); e != nil {
		t.Fatal(e)
	}
	if e = s.Configure(900001, h.ID, item.ID, true, false); e != nil {
		t.Fatal(e)
	}
	attempt, e := s.Submit(900002, h.ID, item.ID, "", now.Add(72*time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	var count int64
	db.Model(&Instance{}).Count(&count)
	if count != 1 {
		t.Fatal("created while awaiting approval")
	}
	accepted := time.Date(2026, 4, 8, 22, 0, 0, 0, time.UTC)
	if e = s.Review(900001, h.ID, attempt.ID, true, accepted); e != nil {
		t.Fatal(e)
	}
	var next Instance
	db.Where("closed = ?", false).First(&next)
	if next.ID == item.ID || next.DueAt == nil || !next.DueAt.Equal(time.Date(2026, 4, 14, 17, 0, 0, 0, time.UTC)) {
		t.Fatal(next)
	}
	if _, e = s.Submit(900002, h.ID, item.ID, "", accepted); e != nil {
		t.Fatal(e)
	}
	db.Model(&Instance{}).Count(&count)
	if count != 2 {
		t.Fatal("old callback created another repeat")
	}
}

func TestTimezoneChangePreservesLocalScheduleAndInvalidatesCards(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(t.TempDir()+"/zone.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	f, _ := family.New(db)
	s, _ := New(db)
	_ = f.Start(900001, "A")
	h, _ := f.Create(900001, "Дом", "Europe/Berlin")
	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	item, _ := s.Create(900001, h.ID, "Мусор", 10, nil)
	if e := s.SetSchedule(900001, h.ID, item.ID, Schedule{Mode: "weekdays", Days: []string{"tuesday"}, Hour: 19}, now); e != nil {
		t.Fatal(e)
	}
	var before Instance
	db.First(&before, item.ID)
	if e := s.ChangeTimezone(900001, "Asia/Almaty"); e != nil {
		t.Fatal(e)
	}
	var after Instance
	db.First(&after, item.ID)
	if after.Version <= before.Version || after.DueAt == nil || after.DueAt.Hour() != 14 {
		t.Fatal(after)
	}
	if _, e := s.Submit(900001, h.ID, item.ID, "", time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)); e != nil {
		t.Fatal(e)
	}
	var next Instance
	db.Where("closed = ?", false).First(&next)
	if next.DueAt == nil || next.DueAt.Hour() != 14 || next.DueAt.Day() != 24 {
		t.Fatal(next)
	}
}

func TestDailyWallClockSurvivesCreatingRuleAroundGap(t *testing.T) {
	for _, tc := range []struct {
		zone                     string
		month                    time.Month
		transition, hour, minute int
	}{
		{"Europe/Berlin", time.March, 29, 2, 30},
		{"America/New_York", time.March, 8, 2, 15},
		{"Australia/Lord_Howe", time.October, 4, 2, 10},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			db, e := gorm.Open(sqlite.Open(t.TempDir()+"/home.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if e != nil {
				t.Fatal(e)
			}
			f, e := family.New(db)
			if e != nil {
				t.Fatal(e)
			}
			s, e := New(db)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.Start(900001, "A"); e != nil {
				t.Fatal(e)
			}
			h, e := f.Create(900001, "Дом", tc.zone)
			if e != nil {
				t.Fatal(e)
			}
			loc, e := time.LoadLocation(tc.zone)
			if e != nil {
				t.Fatal(e)
			}
			for _, day := range []int{tc.transition, tc.transition + 1} {
				now := time.Date(2026, tc.month, day, 12, 0, 0, 0, loc)
				item, e := s.Create(900001, h.ID, "Полить цветы", 5, nil)
				if e != nil {
					t.Fatal(e)
				}
				if e = s.SetSchedule(900001, h.ID, item.ID, Schedule{Mode: "daily", Hour: tc.hour, Minute: tc.minute}, now); e != nil {
					t.Fatal(e)
				}
				var got Instance
				if e = db.First(&got, item.ID).Error; e != nil {
					t.Fatal(e)
				}
				want := time.Date(2026, tc.month, day+1, tc.hour, tc.minute, 0, 0, loc)
				if got.DueAt == nil || !got.DueAt.Equal(want) {
					t.Errorf("creation %v: due %v; want %v", now, got.DueAt, want)
				}
			}
		})
	}
}
