package stats

import (
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type GoalSetting struct {
	CircleID  int `gorm:"primaryKey;autoIncrement:false"`
	Goal      int
	DigestOff bool
}

func (GoalSetting) TableName() string { return "hq_goal_settings" }

type WeekGoal struct {
	CircleID int       `gorm:"primaryKey;autoIncrement:false"`
	Start    time.Time `gorm:"primaryKey"`
	Goal     int
}

func (WeekGoal) TableName() string { return "hq_week_goals" }

// Effective is a local Monday represented at UTC midnight, not an event instant.
type GoalRevision struct {
	CircleID  int       `gorm:"primaryKey;autoIncrement:false"`
	Effective time.Time `gorm:"primaryKey"`
	Goal      int
}

func (GoalRevision) TableName() string { return "hq_goal_revisions" }
func calendarDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type WeeklyReport struct {
	Start, End                          time.Time
	Timezone                            string
	Goal, Points, Count, NeedsAttention int
	NextGoal                            int
	DigestOff                           bool
}

func (s *Service) goal(circle int) (GoalSetting, error) {
	p := GoalSetting{CircleID: circle, Goal: 100}
	e := s.DB.First(&p, "circle_id=?", circle).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return p, nil
	}
	return p, e
}
func (s *Service) freeze(circle int, start time.Time, goal int) (int, error) {
	r := WeekGoal{CircleID: circle, Start: start, Goal: goal}
	if e := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&r).Error; e != nil {
		return 0, e
	}
	if e := s.DB.First(&r, "circle_id=? AND start=?", circle, start).Error; e != nil {
		return 0, e
	}
	return r.Goal, nil
}
func (s *Service) SetWeeklyGoal(tg int64, circle, goal int, now time.Time) error {
	if goal < 1 || goal > 100000 {
		return errors.New("Цель: от 1 до 100000 баллов")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		h, e := (&family.Service{DB: tx}).View(tg, circle)
		if e != nil {
			return e
		}
		if !h.Owner {
			return family.ErrDenied
		}
		child := &Service{DB: tx}
		if _, e = child.Weekly(tg, circle, now, 0); e != nil {
			return e
		}
		p, e := child.goal(circle)
		if e != nil {
			return e
		}
		p.Goal = goal
		if e = tx.Save(&p).Error; e != nil {
			return e
		}
		loc, e := time.LoadLocation(h.Timezone)
		if e != nil {
			return e
		}
		start, e := weekStart(now, h.Timezone)
		if e != nil {
			return e
		}
		effective := calendarDate(start.In(loc).AddDate(0, 0, 7))
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "circle_id"}, {Name: "effective"}}, DoUpdates: clause.AssignmentColumns([]string{"goal"})}).Create(&GoalRevision{CircleID: circle, Effective: effective, Goal: goal}).Error
	})
}
func (s *Service) SetDigest(tg int64, circle int, on bool) error {
	h, e := (&family.Service{DB: s.DB}).View(tg, circle)
	if e != nil {
		return e
	}
	if !h.Owner {
		return family.ErrDenied
	}
	p, e := s.goal(circle)
	if e != nil {
		return e
	}
	p.DigestOff = !on
	return s.DB.Save(&p).Error
}
func (s *Service) Weekly(tg int64, circle int, now time.Time, offset int) (WeeklyReport, error) {
	var out WeeklyReport
	if offset != 0 && offset != -1 {
		return out, family.ErrDenied
	}
	h, e := (&family.Service{DB: s.DB}).View(tg, circle)
	if e != nil {
		return out, e
	}
	loc, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return out, e
	}
	start, e := weekStart(now, h.Timezone)
	if e != nil {
		return out, e
	}
	start = start.In(loc).AddDate(0, 0, offset*7)
	out.Start = start.UTC()
	out.End = start.AddDate(0, 0, 7).UTC()
	out.Timezone = h.Timezone
	p, e := s.goal(circle)
	if e != nil {
		return out, e
	}
	out.NextGoal = p.Goal
	out.DigestOff = p.DigestOff
	// Resolve the policy even for empty weeks when no one opened the bot.
	// Cached snapshots preserve already observed goals.
	goalDate := calendarDate(start)
	goal := 100
	var revision GoalRevision
	e = s.DB.Where("circle_id=? AND effective<=?", circle, goalDate).Order("effective DESC").First(&revision).Error
	if e == nil {
		goal = revision.Goal
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	out.Goal, e = s.freeze(circle, goalDate, goal)
	if e != nil {
		return out, e
	}
	var sums struct {
		Points int
		Count  int
	}
	e = s.DB.Table("hq_attempts a").Select("COALESCE(SUM(h.points),0) AS points, COUNT(*) AS count").Joins("JOIN hq_task_instances i ON i.id=a.instance_id").Joins("JOIN chore_histories h ON h.id=a.history_id").Where("i.circle_id=? AND a.status=? AND a.accepted_at>=? AND a.accepted_at<?", circle, "accepted", out.Start, out.End).Scan(&sums).Error
	if e != nil {
		return out, e
	}
	out.Points = sums.Points
	out.Count = sums.Count
	var pending int64
	e = s.DB.Table("hq_task_instances i").Joins("JOIN chores c ON c.id=i.chore_id").Where("i.circle_id=? AND i.closed=? AND i.pending=? AND i.due_at<? AND c.is_active=? AND c.status<>?", circle, false, false, now, true, 2).Count(&pending).Error
	out.NeedsAttention = int(pending)
	return out, e
}
