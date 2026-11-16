package stats

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"time"
)

type Service struct {
	DB         *gorm.DB
	WeeklyGoal int
}
type Summary struct {
	Balance   int
	Week      int
	XP        int
	HouseXP   int
	WeekStart time.Time
	Timezone  string
}
type Rank struct {
	UserID int
	Name   string
	Points int
	Rank   int
}
type Badge struct {
	ID        int    `gorm:"primaryKey"`
	CircleID  int    `gorm:"uniqueIndex:hq_badge_key"`
	UserID    int    `gorm:"uniqueIndex:hq_badge_key"`
	Code      string `gorm:"uniqueIndex:hq_badge_key"`
	CreatedAt time.Time
}

func (Badge) TableName() string { return "hq_badges" }
func New(db *gorm.DB) (*Service, error) {
	return &Service{DB: db, WeeklyGoal: 100}, db.AutoMigrate(&Badge{})
}
func weekStart(now time.Time, zone string) (time.Time, error) {
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return time.Time{}, e
	}
	t := now.In(loc)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7).UTC(), nil
}
func (s *Service) earned(circle, user int, since *time.Time) (int, error) {
	q := s.DB.Table("hq_attempts a").Joins("JOIN hq_task_instances i ON i.id = a.instance_id").Joins("JOIN chore_histories h ON h.id = a.history_id").Where("i.circle_id = ? AND a.status = ?", circle, "accepted")
	if user != 0 {
		q = q.Where("a.user_id = ?", user)
	}
	if since != nil {
		q = q.Where("a.accepted_at >= ?", *since)
	}
	var sum int
	e := q.Select("COALESCE(SUM(h.points),0)").Scan(&sum).Error
	return sum, e
}
func (s *Service) Summary(tg int64, circle int, now time.Time) (Summary, error) {
	var out Summary
	f := &family.Service{DB: s.DB}
	user, e := f.Authorize(tg, circle)
	if e != nil {
		return out, e
	}
	home, e := f.View(tg, circle)
	if e != nil {
		return out, e
	}
	out.Timezone = home.Timezone
	out.WeekStart, e = weekStart(now, home.Timezone)
	if e != nil {
		return out, e
	}
	out.Balance, e = (&task.Service{DB: s.DB}).Balance(tg, circle)
	if e != nil {
		return out, e
	}
	out.Week, e = s.earned(circle, user.ID, &out.WeekStart)
	if e != nil {
		return out, e
	}
	out.XP, e = s.earned(circle, user.ID, nil)
	if e != nil {
		return out, e
	}
	out.HouseXP, e = s.earned(circle, 0, nil)
	return out, e
}
func (s *Service) Ranking(tg int64, circle int, now time.Time) ([]Rank, error) {
	home, e := (&family.Service{DB: s.DB}).View(tg, circle)
	if e != nil {
		return nil, e
	}
	start, e := weekStart(now, home.Timezone)
	if e != nil {
		return nil, e
	}
	var rows []Rank
	for _, m := range home.Members {
		points, e := s.earned(circle, m.ID, &start)
		if e != nil {
			return nil, e
		}
		rows = append(rows, Rank{UserID: m.ID, Name: m.Name, Points: points})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Points == rows[j].Points {
			return rows[i].UserID < rows[j].UserID
		}
		return rows[i].Points > rows[j].Points
	})
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].Points == rows[i-1].Points {
			rows[i].Rank = rows[i-1].Rank
		}
	}
	return rows, nil
}

var BadgeNames = map[string]string{"first": "Первое дело", "ten": "10 домашних дел", "fifty": "50 домашних дел", "photo": "Первый фотоотчёт", "days": "Вклад в три разных дня", "together": "Общая недельная цель"}

func (s *Service) Badges(tg int64, circle int, now time.Time) ([]string, error) {
	f := &family.Service{DB: s.DB}
	user, e := f.Authorize(tg, circle)
	if e != nil {
		return nil, e
	}
	home, e := f.View(tg, circle)
	if e != nil {
		return nil, e
	}
	loc, e := time.LoadLocation(home.Timezone)
	if e != nil {
		return nil, e
	}
	var attempts []task.Attempt
	if e = s.DB.Table("hq_attempts a").Select("a.*").Joins("JOIN hq_task_instances i ON i.id = a.instance_id").Where("i.circle_id = ? AND a.user_id = ? AND a.status = ?", circle, user.ID, "accepted").Find(&attempts).Error; e != nil {
		return nil, e
	}
	photo := false
	days := map[string]bool{}
	for _, a := range attempts {
		photo = photo || a.PhotoID != ""
		days[a.AcceptedAt.In(loc).Format("2006-01-02")] = true
	}
	start, e := weekStart(now, home.Timezone)
	if e != nil {
		return nil, e
	}
	week, e := s.earned(circle, 0, &start)
	if e != nil {
		return nil, e
	}
	conditions := []struct {
		Code string
		Met  bool
	}{{"first", len(attempts) >= 1}, {"ten", len(attempts) >= 10}, {"fifty", len(attempts) >= 50}, {"photo", photo}, {"days", len(days) >= 3}, {"together", week >= s.WeeklyGoal && len(home.Members) >= 2}}
	var names []string
	for _, condition := range conditions {
		if !condition.Met {
			continue
		}
		row := Badge{CircleID: circle, UserID: user.ID, Code: condition.Code, CreatedAt: now.UTC()}
		if e = s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return nil, e
		}

	}
	var saved []Badge
	if e = s.DB.Where("circle_id = ? AND user_id = ?", circle, user.ID).Order("id").Find(&saved).Error; e != nil {
		return nil, e
	}
	for _, badge := range saved {
		names = append(names, BadgeNames[badge.Code])
	}
	return names, nil
}
