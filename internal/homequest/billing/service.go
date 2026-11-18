// Package billing keeps entitlement separate from game points. No live provider
// is connected and no Telegram invoice/payment methods are used.
package billing

import (
	"context"
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Entitlement struct {
	CircleID               int `gorm:"primaryKey;autoIncrement:false"`
	Source                 string
	ProviderSubscriptionID string
	ProviderPaymentID      string
	StartsAt               time.Time
	ExpiresAt              *time.Time
	RevokedAt              *time.Time
	CancelAtPeriodEnd      bool
}

func (Entitlement) TableName() string { return "hq_entitlements" }

// Future providers must verify signatures, deduplicate (provider,event ID), and
// apply period/cancel/refund atomically. This contract is not a payment adapter.
type Event struct {
	Provider, ID, SubscriptionID, PaymentID, Kind string
	CircleID                                      int
	StartsAt                                      time.Time
	ExpiresAt                                     *time.Time
}
type BillingProvider interface {
	VerifyEvent(context.Context, []byte) (Event, error)
}
type PaymentEvent struct {
	Provider    string `gorm:"primaryKey"`
	EventID     string `gorm:"primaryKey"`
	ProcessedAt time.Time
}

func (PaymentEvent) TableName() string { return "hq_payment_events" }

type EntitlementService interface {
	Active(int64, int, time.Time) (bool, error)
	Manual(int64, int, bool, time.Time) error
}
type Service struct {
	DB   *gorm.DB
	Mode string
}

func New(db *gorm.DB, mode string) (*Service, error) {
	return &Service{db, mode}, db.AutoMigrate(&Entitlement{}, &PaymentEvent{})
}
func (s *Service) Manual(tg int64, circle int, enabled bool, now time.Time) error {
	if s.Mode != "manual_test" {
		return errors.New("Ручной Premium доступен только в тестовом режиме")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		h, e := (&family.Service{DB: tx}).View(tg, circle)
		if e != nil {
			return e
		}
		if !h.Owner {
			return family.ErrDenied
		}
		var existing Entitlement
		e = tx.First(&existing, "circle_id = ?", circle).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil && existing.Source != "manual_test" {
			return errors.New("Внешнее право нельзя изменить тестовой кнопкой")
		}
		row := Entitlement{CircleID: circle, Source: "manual_test", StartsAt: now}
		if !enabled {
			row.RevokedAt = &now
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
	})
}
func (s *Service) Active(tg int64, circle int, now time.Time) (bool, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return false, e
	}
	var row Entitlement
	e := s.DB.First(&row, "circle_id = ?", circle).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return row.RevokedAt == nil && !row.StartsAt.After(now) && (row.ExpiresAt == nil || row.ExpiresAt.After(now)), nil
}
