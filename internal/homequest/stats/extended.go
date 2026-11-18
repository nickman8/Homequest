package stats

import (
	"donetick.com/core/internal/homequest/billing"
	"errors"
	"time"
)

var ErrPremium = errors.New("Расширенная статистика доступна с Premium")

// Extended counts require a current household membership and active entitlement.
func (s *Service) Extended(uid int64, circle int, now time.Time) (map[string]int, error) {
	active, e := (&billing.Service{DB: s.DB}).Active(uid, circle, now)
	if e != nil {
		return nil, e
	}
	if !active {
		return nil, ErrPremium
	}
	var rows []struct {
		Status string
		Count  int
	}
	e = s.DB.Table("hq_attempts a").Select("a.status, COUNT(*) AS count").Joins("JOIN hq_task_instances i ON i.id=a.instance_id").Where("i.circle_id=?", circle).Group("a.status").Scan(&rows).Error
	if e != nil {
		return nil, e
	}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Status] = row.Count
	}
	return counts, nil
}
