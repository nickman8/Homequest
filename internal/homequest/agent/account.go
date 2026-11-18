package agent

import (
	"donetick.com/core/internal/homequest/stats"
	"time"
)

// Account is read-only: personal spendable balance, accepted contribution/XP,
// and the existing household ranking. No entitlement or gift claims are exposed.
type Account struct {
	Summary stats.Summary `json:"summary"`
	Ranking []stats.Rank  `json:"ranking"`
}

func (s *Service) Account(c Context, now time.Time) (Account, error) {
	var out Account
	if e := s.Check(c, now); e != nil {
		return out, e
	}
	svc := &stats.Service{DB: s.DB}
	var e error
	out.Summary, e = svc.Summary(c.Viewer, c.CircleID, now)
	if e != nil {
		return out, e
	}
	out.Ranking, e = svc.Ranking(c.Viewer, c.CircleID, now)
	return out, e
}
