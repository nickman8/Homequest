package task

import (
	"context"
	scheduler "donetick.com/core/internal/chore"
	ch "donetick.com/core/internal/chore/model"
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	p "donetick.com/core/internal/points"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Rule struct {
	ReviewerID    *int
	ChoreID       int `gorm:"primaryKey;autoIncrement:false"`
	PhotoRequired bool
}

func (Rule) TableName() string { return "hq_task_rules" }
func (s *Service) Configure(tg int64, circle, id int, approval, photo bool) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		user, e := f.Authorize(tg, circle)
		if e != nil {
			return e
		}
		home, e := f.View(tg, circle)
		if e != nil {
			return e
		}
		if approval && len(home.Members) < 2 {
			return errors.New("Для подтверждения нужен ещё один взрослый участник")
		}
		var instance Instance
		if tx.First(&instance, "id = ? AND circle_id = ? AND closed = ? AND pending = ?", id, circle, false, false).Error != nil {
			return family.ErrDenied
		}
		var chore ch.Chore
		if e = tx.First(&chore, instance.ChoreID).Error; e != nil {
			return e
		}
		if chore.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		if e = tx.Model(&chore).Update("require_approval", approval).Error; e != nil {
			return e
		}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chore_id"}}, DoUpdates: clause.Assignments(map[string]any{"photo_required": photo})}).Create(&Rule{ChoreID: chore.ID, PhotoRequired: photo}).Error; e != nil {
			return e
		}
		return tx.Model(&instance).Update("version", gorm.Expr("version + 1")).Error
	})
}
func (s *Service) Submit(tg int64, circle, id int, photo string, now time.Time) (Attempt, error) {
	var out Attempt
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var instance Instance
		if tx.First(&instance, "id = ? AND circle_id = ?", id, circle).Error != nil {
			return family.ErrDenied
		}
		var chore ch.Chore
		if tx.First(&chore, "id = ? AND circle_id = ?", instance.ChoreID, circle).Error != nil {
			return family.ErrDenied
		}
		if chore.AssignedTo != nil && *chore.AssignedTo != user.ID {
			return family.ErrDenied
		}
		if instance.Closed || instance.Pending {
			if tx.Where("instance_id = ? AND status IN ?", id, []string{"accepted", "photo", "review"}).Order("id DESC").First(&out).Error != nil {
				return errors.New("Задание изменилось")
			}
			if out.UserID != user.ID {
				return errors.New("Задание уже отправлено другим участником")
			}
			return nil
		}
		if !chore.IsActive || chore.Status == ch.ChoreStatusPaused {
			return family.ErrDenied
		}
		r := tx.Model(&Instance{}).Where("id = ? AND pending = ? AND closed = ?", id, false, false).Update("pending", true)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return errors.New("Задание изменилось")
		}
		points := 0
		if chore.Points != nil {
			points = *chore.Points
		}
		now = now.UTC()
		history := ch.ChoreHistory{ChoreID: chore.ID, CompletedBy: user.ID, PerformedAt: &now, DueDate: instance.DueAt, Points: &points, Status: ch.ChoreHistoryStatusPendingApproval}
		if e = tx.Create(&history).Error; e != nil {
			return e
		}
		out = Attempt{InstanceID: id, HistoryID: history.ID, UserID: user.ID, PhotoID: photo, Approval: chore.RequireApproval, Status: "review"}
		var rule Rule
		_ = tx.First(&rule, "chore_id = ?", chore.ID).Error
		out.ReviewerID = rule.ReviewerID
		if out.Approval && out.ReviewerID != nil && *out.ReviewerID == user.ID {
			return errors.New("Назначенный проверяющий не может подтвердить собственное выполнение")
		}
		if rule.PhotoRequired && photo == "" {
			out.Status = "photo"
		}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		if out.Status != "photo" && !out.Approval {
			return accept(tx, &out, &instance, now)
		}
		return nil
	})
	return out, e
}
func accept(tx *gorm.DB, a *Attempt, i *Instance, now time.Time) error {
	if a.Status == "accepted" {
		return nil
	}
	if a.Status != "review" {
		return family.ErrDenied
	}
	var history ch.ChoreHistory
	if e := tx.First(&history, a.HistoryID).Error; e != nil {
		return e
	}
	if history.Points == nil {
		return errors.New("missing price snapshot")
	}
	ledger := p.PointsHistory{Action: p.PointsHistoryActionAdd, Points: *history.Points, CreatedAt: now.UTC(), CreatedBy: a.UserID, UserID: a.UserID, CircleID: i.CircleID}
	if e := tx.Create(&ledger).Error; e != nil {
		return e
	}
	r := tx.Model(a).Where("status = ?", "review").Updates(map[string]any{"status": "accepted", "accepted_at": now.UTC(), "ledger_id": ledger.ID})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return errors.New("Попытка уже обработана")
	}
	if e := tx.Model(&history).Update("status", ch.ChoreHistoryStatusCompleted).Error; e != nil {
		return e
	}
	if e := tx.Model(&c.UserCircle{}).Where("user_id = ? AND circle_id = ?", a.UserID, i.CircleID).Update("points", gorm.Expr("points + ?", *history.Points)).Error; e != nil {
		return e
	}
	if e := tx.Model(i).Updates(map[string]any{"closed": true, "pending": false}).Error; e != nil {
		return e
	}
	var definition ch.Chore
	if e := tx.First(&definition, i.ChoreID).Error; e != nil {
		return e
	}
	next, e := scheduler.NextAfterAcceptance(context.Background(), &definition, now)
	if e != nil {
		return e
	}
	if next != nil {
		if e = tx.Create(&Instance{ChoreID: i.ChoreID, CircleID: i.CircleID, DueAt: next, Version: 1}).Error; e != nil {
			return e
		}
	}
	if e = tx.Model(&definition).Updates(map[string]any{"is_active": next != nil, "next_due_date": next}).Error; e != nil {
		return e
	}
	a.Status = "accepted"
	a.AcceptedAt = now.UTC()
	a.LedgerID = &ledger.ID
	return nil
}
func loadAttempt(tx *gorm.DB, tg int64, circle, id int) (Attempt, Instance, int, error) {
	var a Attempt
	var i Instance
	user, e := (&family.Service{DB: tx}).Authorize(tg, circle)
	if e != nil {
		return a, i, 0, e
	}
	if tx.First(&a, id).Error != nil {
		return a, i, 0, family.ErrDenied
	}
	if tx.First(&i, "id = ? AND circle_id = ?", a.InstanceID, circle).Error != nil {
		return a, i, 0, family.ErrDenied
	}
	return a, i, user.ID, nil
}
func (s *Service) Attach(tg int64, circle, id int, photo string, now time.Time) error {
	if photo == "" || len(photo) > 512 {
		return family.ErrDenied
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		a, i, user, e := loadAttempt(tx, tg, circle, id)
		if e != nil {
			return e
		}
		if user != a.UserID || a.Status != "photo" {
			return family.ErrDenied
		}
		if e = tx.Model(&a).Updates(map[string]any{"photo_id": photo, "status": "review"}).Error; e != nil {
			return e
		}
		a.Status = "review"
		a.PhotoID = photo
		if !a.Approval {
			return accept(tx, &a, &i, now)
		}
		return nil
	})
}
func (s *Service) Review(tg int64, circle, id int, approve bool, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		a, i, user, e := loadAttempt(tx, tg, circle, id)
		if e != nil {
			return e
		}
		if !CanReview(tx, a, circle, user) {
			return family.ErrDenied
		}
		if a.Status == "accepted" && approve || a.Status == "rejected" && !approve {
			return nil
		}
		if a.Status != "review" {
			return family.ErrDenied
		}
		if e = tx.Model(&a).Updates(map[string]any{"reviewed_by": user, "reviewed_at": now.UTC()}).Error; e != nil {
			return e
		}
		if approve {
			return accept(tx, &a, &i, now)
		}
		r := tx.Model(&a).Where("status = ?", "review").Update("status", "rejected")
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return family.ErrDenied
		}
		if e = tx.Model(&ch.ChoreHistory{}).Where("id = ?", a.HistoryID).Update("status", ch.ChoreHistoryStatusRejected).Error; e != nil {
			return e
		}
		return tx.Model(&i).Update("pending", false).Error
	})
}
func (s *Service) Photo(tg int64, circle, id int) (string, error) {
	a, _, _, e := loadAttempt(s.DB, tg, circle, id)
	if e != nil {
		return "", e
	}
	if a.PhotoID == "" {
		return "", family.ErrDenied
	}
	return a.PhotoID, nil
}
func (s *Service) Undo(tg int64, circle, id int, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		a, i, user, e := loadAttempt(tx, tg, circle, id)
		if e != nil {
			return e
		}
		if user != a.UserID || a.Approval {
			return family.ErrDenied
		}
		if a.Status == "undone" {
			return nil
		}
		if a.Status != "accepted" || now.Sub(a.AcceptedAt) > 5*time.Minute || now.Before(a.AcceptedAt) {
			return errors.New("Отмена доступна 5 минут после выполнения")
		}
		var history ch.ChoreHistory
		if e = tx.First(&history, a.HistoryID).Error; e != nil {
			return e
		}
		if history.Points == nil {
			return family.ErrDenied
		}
		balance, e := (&Service{DB: tx}).Balance(tg, circle)
		if e != nil {
			return e
		}
		if balance < *history.Points {
			return errors.New("Недостаточно свободных баллов для отмены")
		}
		var successors []Instance
		if e = tx.Where("chore_id = ? AND id > ? AND closed = ?", i.ChoreID, i.ID, false).Find(&successors).Error; e != nil {
			return e
		}
		for _, successor := range successors {
			var n int64
			if e = tx.Model(&Attempt{}).Where("instance_id = ?", successor.ID).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return errors.New("Следующее повторение уже используется; обычная отмена недоступна")
			}
		}
		// A later accepted occurrence also prevents reopening old history.
		var later int64
		if e = tx.Model(&Attempt{}).Joins("JOIN hq_task_instances i ON i.id = hq_attempts.instance_id").Where("i.chore_id = ? AND i.id > ? AND hq_attempts.status = ?", i.ChoreID, i.ID, "accepted").Count(&later).Error; e != nil {
			return e
		}
		if later > 0 {
			return errors.New("Следующее повторение уже принято")
		}
		for _, successor := range successors {
			if e = tx.Model(&successor).Update("closed", true).Error; e != nil {
				return e
			}
		}
		refund := p.PointsHistory{Action: p.PointsHistoryActionRemove, Points: *history.Points, CreatedAt: now.UTC(), CreatedBy: user, UserID: user, CircleID: circle}
		if e = tx.Create(&refund).Error; e != nil {
			return e
		}
		r := tx.Model(&a).Where("status = ?", "accepted").Updates(map[string]any{"status": "undone", "undo_ledger_id": refund.ID})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return family.ErrDenied
		}
		if e = tx.Model(&c.UserCircle{}).Where("user_id = ? AND circle_id = ?", user, circle).Update("points", gorm.Expr("points - ?", *history.Points)).Error; e != nil {
			return e
		}
		if e = tx.Model(&i).Updates(map[string]any{"closed": false, "pending": false, "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
		return tx.Model(&ch.Chore{}).Where("id = ?", i.ChoreID).Updates(map[string]any{"is_active": true, "next_due_date": i.DueAt}).Error
	})
}

// If a designated reviewer leaves the family, any other current adult may
// review. The performer still cannot approve their own work.
func CanReview(db *gorm.DB, a Attempt, circle, user int) bool {
	if user == a.UserID || !a.Approval {
		return false
	}
	if a.ReviewerID == nil || *a.ReviewerID == user {
		return true
	}
	var count int64
	if db.Model(&c.UserCircle{}).Where("circle_id = ? AND user_id = ? AND is_active = ?", circle, *a.ReviewerID, true).Count(&count).Error != nil {
		return false
	}
	return count == 0
}
func (s *Service) SetReviewer(tg int64, circle, id, target int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		user, e := f.Authorize(tg, circle)
		if e != nil {
			return e
		}
		home, e := f.View(tg, circle)
		if e != nil {
			return e
		}
		var i Instance
		if tx.First(&i, "id = ? AND circle_id = ? AND closed = ? AND pending = ?", id, circle, false, false).Error != nil {
			return family.ErrDenied
		}
		var chore ch.Chore
		if tx.First(&chore, i.ChoreID).Error != nil {
			return family.ErrDenied
		}
		if chore.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		var reviewer *int
		if target != 0 {
			var m c.UserCircle
			if tx.First(&m, "user_id = ? AND circle_id = ? AND is_active = ?", target, circle, true).Error != nil {
				return family.ErrDenied
			}
			if chore.AssignedTo != nil && *chore.AssignedTo == target {
				return errors.New("Исполнитель и проверяющий должны быть разными")
			}
			reviewer = &target
		}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chore_id"}}, DoUpdates: clause.Assignments(map[string]any{"reviewer_id": reviewer})}).Create(&Rule{ChoreID: chore.ID, ReviewerID: reviewer}).Error; e != nil {
			return e
		}
		return tx.Model(&i).Update("version", gorm.Expr("version + 1")).Error
	})
}
