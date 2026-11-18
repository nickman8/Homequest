// Package reward adapts Donetick Reward/RewardRedemption and points transactions.
package reward

import (
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	p "donetick.com/core/internal/points"
	r "donetick.com/core/internal/reward/model"
	"errors"
	"gorm.io/gorm"
	"strings"
	"time"
)

var ErrInsufficient = errors.New("Пока не хватает баллов")

const Delivered = r.RedemptionStatusDelivered
const Cancelled = r.RedemptionStatusCancelled

type Receipt struct {
	Key          string `gorm:"primaryKey"`
	UserID       int
	CircleID     int
	RedemptionID int `gorm:"uniqueIndex"`
	DeliveredAt  *time.Time
}

func (Receipt) TableName() string { return "hq_redemption_receipts" }

type Service struct{ DB *gorm.DB }

func New(db *gorm.DB) (*Service, error) {
	return &Service{db}, db.AutoMigrate(&r.Reward{}, &r.RewardRedemption{}, &Receipt{})
}
func (s *Service) Create(tg int64, circle int, name, description string, cost int) (r.Reward, error) {
	var out r.Reward
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 || len([]rune(description)) > 500 || cost < 1 || cost > 100000 {
		return out, errors.New("Название до 100 символов; цена от 1 до 100000")
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		out = r.Reward{CircleID: circle, Name: name, Description: description, PointsCost: cost, IsActive: true, CreatedBy: user.ID}
		return tx.Create(&out).Error
	})
	return out, e
}
func (s *Service) Edit(tg int64, circle, id int, name string, cost int, description ...string) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 || cost < 1 || cost > 100000 {
		return errors.New("Некорректное название или цена")
	}
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
		var reward r.Reward
		if tx.First(&reward, "id = ? AND circle_id = ?", id, circle).Error != nil {
			return family.ErrDenied
		}
		if reward.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		updates := map[string]any{"name": name, "points_cost": cost}
		if len(description) > 0 {
			if len([]rune(description[0])) > 500 {
				return errors.New("Описание до 500 символов")
			}
			updates["description"] = description[0]
		}
		return tx.Model(&reward).Updates(updates).Error
	})
}
func (s *Service) List(tg int64, circle int) ([]r.Reward, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var rows []r.Reward
	e := s.DB.Where("circle_id = ? AND is_active = ?", circle, true).Order("id").Find(&rows).Error
	return rows, e
}
func (s *Service) Request(tg int64, circle, id int, key string, now time.Time) (r.RewardRedemption, error) {
	var out r.RewardRedemption
	if len(key) < 1 || len(key) > 64 {
		return out, family.ErrDenied
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var receipt Receipt
		err := tx.First(&receipt, "key = ?", key).Error
		if err == nil {
			if receipt.UserID != user.ID || receipt.CircleID != circle {
				return family.ErrDenied
			}
			return tx.First(&out, receipt.RedemptionID).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var reward r.Reward
		if tx.First(&reward, "id = ? AND circle_id = ? AND is_active = ?", id, circle, true).Error != nil {
			return family.ErrDenied
		}
		balance, e := (&task.Service{DB: tx}).Balance(tg, circle)
		if e != nil {
			return e
		}
		if balance < reward.PointsCost {
			return ErrInsufficient
		}
		out = r.RewardRedemption{RewardID: id, CircleID: circle, UserID: user.ID, RewardName: reward.Name, PointsCost: reward.PointsCost, Status: r.RedemptionStatusPending, RequestedAt: now.UTC()}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		if e = tx.Create(&Receipt{Key: key, UserID: user.ID, CircleID: circle, RedemptionID: out.ID}).Error; e != nil {
			return e
		}
		if e = tx.Create(&p.PointsHistory{Action: p.PointsHistoryActionRedeem, Points: reward.PointsCost, UserID: user.ID, CircleID: circle, CreatedBy: user.ID, CreatedAt: now.UTC()}).Error; e != nil {
			return e
		}
		return tx.Model(&c.UserCircle{}).Where("circle_id = ? AND user_id = ?", circle, user.ID).Update("points_redeemed", gorm.Expr("points_redeemed + ?", reward.PointsCost)).Error
	})
	return out, e
}
func (s *Service) Resolve(tg int64, circle, id int, action string, now time.Time) error {
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
		var request r.RewardRedemption
		if tx.First(&request, "id = ? AND circle_id = ?", id, circle).Error != nil {
			return family.ErrDenied
		}
		target := request.Status
		switch action {
		case "approve":
			if user.ID == request.UserID {
				return family.ErrDenied
			}
			if request.Status == r.RedemptionStatusApproved {
				return nil
			}
			if request.Status != r.RedemptionStatusPending {
				return family.ErrDenied
			}
			target = r.RedemptionStatusApproved
		case "deliver":
			if user.ID == request.UserID {
				return family.ErrDenied
			}
			if request.Status == Delivered {
				return nil
			}
			if request.Status != r.RedemptionStatusApproved {
				return family.ErrDenied
			}
			target = Delivered
		case "reject":
			if user.ID == request.UserID {
				return family.ErrDenied
			}
			if request.Status == r.RedemptionStatusRejected {
				return nil
			}
			if request.Status != r.RedemptionStatusPending && request.Status != r.RedemptionStatusApproved {
				return family.ErrDenied
			}
			target = r.RedemptionStatusRejected
		case "cancel":
			if user.ID != request.UserID && !home.Owner {
				return family.ErrDenied
			}
			if request.Status == Cancelled {
				return nil
			}
			if request.Status != r.RedemptionStatusPending && request.Status != r.RedemptionStatusApproved {
				return family.ErrDenied
			}
			target = Cancelled
		default:
			return family.ErrDenied
		}
		old := request.Status
		r0 := tx.Model(&request).Where("status = ?", old).Updates(map[string]any{"status": target, "resolved_at": now.UTC(), "resolved_by": user.ID})
		if r0.Error != nil {
			return r0.Error
		}
		if r0.RowsAffected != 1 {
			return errors.New("Заявка уже изменена")
		}
		if target == Delivered {
			return tx.Model(&Receipt{}).Where("redemption_id = ?", request.ID).Update("delivered_at", now.UTC()).Error
		}
		if target == Cancelled || target == r.RedemptionStatusRejected {
			if e = tx.Create(&p.PointsHistory{Action: p.PointsHistoryActionAdd, Points: request.PointsCost, UserID: request.UserID, CircleID: circle, CreatedBy: user.ID, CreatedAt: now.UTC()}).Error; e != nil {
				return e
			}
			return tx.Model(&c.UserCircle{}).Where("circle_id = ? AND user_id = ?", circle, request.UserID).Update("points_redeemed", gorm.Expr("points_redeemed - ?", request.PointsCost)).Error
		}
		return nil
	})
}
func (s *Service) Requests(tg int64, circle int) ([]r.RewardRedemption, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var rows []r.RewardRedemption
	e := s.DB.Where("circle_id = ?", circle).Order("id DESC").Find(&rows).Error
	return rows, e
}

func (s *Service) Archive(tg int64, circle, id int) error {
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
		var reward r.Reward
		if tx.First(&reward, "id = ? AND circle_id = ?", id, circle).Error != nil {
			return family.ErrDenied
		}
		if reward.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		return tx.Model(&reward).Update("is_active", false).Error
	})
}
