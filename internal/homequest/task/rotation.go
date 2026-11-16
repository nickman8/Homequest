package task

import (
	ch "donetick.com/core/internal/chore/model"
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"time"
)

type Handoff struct {
	ID                                          int `gorm:"primaryKey"`
	CircleID, InstanceID, Version, FromID, ToID int
	Status                                      string
	Expires                                     time.Time
}

func (Handoff) TableName() string { return "hq_handoffs" }
func editable(tx *gorm.DB, tg int64, circle, id int) (Instance, ch.Chore, error) {
	var i Instance
	var d ch.Chore
	f := &family.Service{DB: tx}
	u, e := f.Authorize(tg, circle)
	if e != nil {
		return i, d, e
	}
	h, e := f.View(tg, circle)
	if e != nil {
		return i, d, e
	}
	if tx.First(&i, "id=? AND circle_id=? AND closed=? AND pending=?", id, circle, false, false).Error != nil {
		return i, d, family.ErrDenied
	}
	if tx.First(&d, "id=? AND circle_id=?", i.ChoreID, circle).Error != nil {
		return i, d, family.ErrDenied
	}
	if d.CreatedBy != u.ID && !h.Owner {
		return i, d, family.ErrDenied
	}
	return i, d, nil
}
func (s *Service) SetRotation(tg int64, circle, id int, members []int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		i, d, e := editable(tx, tg, circle, id)
		if e != nil {
			return e
		}
		if len(members) > 30 || len(members) == 1 {
			return errors.New("Выберите минимум двух участников или отключите очередь")
		}
		seen := map[int]bool{}
		var rows []ch.ChoreAssignees
		for _, user := range members {
			var n int64
			if e = tx.Model(&c.UserCircle{}).Where("circle_id=? AND user_id=? AND is_active=?", circle, user, true).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 || seen[user] {
				return family.ErrDenied
			}
			seen[user] = true
			rows = append(rows, ch.ChoreAssignees{ChoreID: d.ID, UserID: user})
		}
		var rule Rule
		tx.First(&rule, "chore_id=?", d.ID)
		if rule.ReviewerID != nil && seen[*rule.ReviewerID] {
			return errors.New("Проверяющий входит в очередь. Выберите проверку любым другим участником")
		}
		if e = tx.Where("chore_id=?", d.ID).Delete(&ch.ChoreAssignees{}).Error; e != nil {
			return e
		}
		var assigned *int
		strategy := ch.AssignmentStrategyNoAssignee
		if len(rows) > 0 {
			if e = tx.Create(&rows).Error; e != nil {
				return e
			}
			assigned = &members[0]
			strategy = ch.AssignmentStrategyRoundRobin
		}
		if e = tx.Model(&d).Updates(map[string]any{"assign_strategy": strategy, "assigned_to": assigned}).Error; e != nil {
			return e
		}
		return tx.Model(&i).Update("version", gorm.Expr("version+1")).Error
	})
}

// Adapted from Donetick v0.1.86 handler.go AssignmentStrategyRoundRobin: retain
// ordered ChoreAssignees and rotate only at accepted_at, filtering departed users.
func rotate(tx *gorm.DB, d *ch.Chore) error {
	if d.AssignStrategy != ch.AssignmentStrategyRoundRobin {
		return nil
	}
	var rows []ch.ChoreAssignees
	if e := tx.Where("chore_id=?", d.ID).Order("id").Find(&rows).Error; e != nil {
		return e
	}
	var active []int
	if e := tx.Model(&c.UserCircle{}).Where("circle_id=? AND is_active=?", d.CircleID, true).Pluck("user_id", &active).Error; e != nil {
		return e
	}
	eligible := map[int]bool{}
	for _, id := range active {
		eligible[id] = true
	}
	index := -1
	for k, v := range rows {
		if d.AssignedTo != nil && v.UserID == *d.AssignedTo {
			index = k
		}
	}
	var next *int
	for k := 1; k <= len(rows); k++ {
		id := rows[(index+k)%len(rows)].UserID
		if eligible[id] {
			next = &id
			break
		}
	}
	d.AssignedTo = next
	return tx.Model(d).Update("assigned_to", next).Error
}
func (s *Service) ReconcileRotation(circle int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var rows []Instance
		if e := tx.Table("hq_task_instances i").Select("i.*").Joins("JOIN chores d ON d.id=i.chore_id").Where("i.circle_id=? AND i.closed=? AND i.pending=? AND d.assign_strategy=? AND d.assigned_to IS NOT NULL AND d.assigned_to NOT IN (SELECT user_id FROM user_circles WHERE circle_id=? AND is_active=?)", circle, false, false, ch.AssignmentStrategyRoundRobin, circle, true).Find(&rows).Error; e != nil {
			return e
		}
		for _, i := range rows {
			var d ch.Chore
			if e := tx.First(&d, i.ChoreID).Error; e != nil {
				return e
			}
			if e := rotate(tx, &d); e != nil {
				return e
			}
			if e := tx.Model(&i).Update("version", gorm.Expr("version+1")).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Service) Rotation(tg int64, circle, id int) ([]int, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var i Instance
	if s.DB.First(&i, "id=? AND circle_id=?", id, circle).Error != nil {
		return nil, family.ErrDenied
	}
	var ids []int
	e := s.DB.Table("chore_assignees a").Select("a.user_id").Joins("JOIN user_circles m ON m.user_id=a.user_id AND m.circle_id=? AND m.is_active=?", circle, true).Where("a.chore_id=?", i.ChoreID).Order("a.id").Scan(&ids).Error
	return ids, e
}
func (s *Service) OfferHandoff(tg int64, circle, id, to int, now time.Time) (Handoff, error) {
	var out Handoff
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var i Instance
		var d ch.Chore
		if tx.First(&i, "id=? AND circle_id=? AND closed=? AND pending=?", id, circle, false, false).Error != nil {
			return family.ErrDenied
		}
		if tx.First(&d, i.ChoreID).Error != nil || d.AssignedTo == nil || *d.AssignedTo != u.ID || d.Status == ch.ChoreStatusPaused {
			return family.ErrDenied
		}
		var n int64
		tx.Model(&c.UserCircle{}).Where("circle_id=? AND user_id=? AND is_active=?", circle, to, true).Count(&n)
		var rule Rule
		tx.First(&rule, "chore_id=?", d.ID)
		if d.RequireApproval && rule.ReviewerID != nil && *rule.ReviewerID == to {
			return errors.New("Проверяющий не может принять дело на себя")
		}
		if n != 1 || to == u.ID {
			return family.ErrDenied
		}
		if e = tx.Model(&Handoff{}).Where("instance_id=? AND status=?", id, "pending").Update("status", "cancelled").Error; e != nil {
			return e
		}
		out = Handoff{CircleID: circle, InstanceID: id, Version: i.Version, FromID: u.ID, ToID: to, Status: "pending", Expires: now.Add(7 * 24 * time.Hour)}
		return tx.Create(&out).Error
	})
	return out, e
}
func (s *Service) Handoffs(tg int64, circle int, now time.Time) ([]Handoff, error) {
	u, e := (&family.Service{DB: s.DB}).Authorize(tg, circle)
	if e != nil {
		return nil, e
	}
	var rows []Handoff
	e = s.DB.Table("hq_handoffs h").Select("h.*").Joins("JOIN hq_task_instances i ON i.id=h.instance_id AND i.version=h.version AND i.closed=? AND i.pending=?", false, false).Where("h.circle_id=? AND (h.from_id=? OR h.to_id=?) AND h.status=? AND h.expires>? AND h.from_id IN (SELECT user_id FROM user_circles WHERE circle_id=? AND is_active=?) AND h.to_id IN (SELECT user_id FROM user_circles WHERE circle_id=? AND is_active=?)", circle, u.ID, u.ID, "pending", now, circle, true, circle, true).Order("h.id").Scan(&rows).Error
	return rows, e
}
func (s *Service) ResolveHandoff(tg int64, circle, id int, op string, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var h Handoff
		if tx.First(&h, "id=? AND circle_id=? AND status=? AND expires>?", id, circle, "pending", now).Error != nil {
			return family.ErrDenied
		}
		if (op == "cancel" && u.ID != h.FromID) || (op != "cancel" && u.ID != h.ToID) {
			return family.ErrDenied
		}
		status := map[string]string{"accept": "accepted", "decline": "declined", "cancel": "cancelled"}[op]
		if status == "" {
			return family.ErrDenied
		}
		var i Instance
		var d ch.Chore
		if tx.First(&i, "id=? AND circle_id=? AND version=? AND closed=? AND pending=?", h.InstanceID, circle, h.Version, false, false).Error != nil {
			return family.ErrDenied
		}
		if tx.First(&d, i.ChoreID).Error != nil || d.AssignedTo == nil || *d.AssignedTo != h.FromID || d.Status == ch.ChoreStatusPaused {
			return family.ErrDenied
		}
		var n int64
		tx.Model(&c.UserCircle{}).Where("circle_id=? AND user_id=? AND is_active=?", circle, h.FromID, true).Count(&n)
		if n != 1 {
			return family.ErrDenied
		}
		if op == "accept" {
			if e = tx.Model(&d).Update("assigned_to", h.ToID).Error; e != nil {
				return e
			}
			if e = tx.Model(&i).Update("version", gorm.Expr("version+1")).Error; e != nil {
				return e
			}
		}
		return tx.Model(&h).Update("status", status).Error
	})
}
