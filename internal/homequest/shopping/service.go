package shopping

import (
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type Input struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Note     string `json:"note"`
}
type Item struct {
	ID                                      int `gorm:"primaryKey"`
	CircleID                                int `gorm:"index"`
	CreatedBy, ClaimedBy, BoughtBy, Version int
	Name, Quantity, Note, Status            string
	UpdatedAt                               time.Time
}

func (Item) TableName() string { return "hq_shopping_items" }

type Batch struct {
	CircleID  int    `gorm:"primaryKey;autoIncrement:false"`
	Request   string `gorm:"primaryKey"`
	CreatedBy int
}

func (Batch) TableName() string { return "hq_shopping_batches" }

type Event struct {
	ID                                int `gorm:"primaryKey"`
	CircleID, ItemID, UserID, Version int
	Action, Snapshot                  string
	At                                time.Time
}

func (Event) TableName() string { return "hq_shopping_events" }

type Service struct{ DB *gorm.DB }

func New(db *gorm.DB) (*Service, error) {
	return &Service{db}, db.AutoMigrate(&Item{}, &Batch{}, &Event{})
}
func Validate(v Input) error {
	if strings.TrimSpace(v.Name) == "" || len([]rune(v.Name)) > 100 || len([]rune(v.Quantity)) > 40 || len([]rune(v.Note)) > 300 {
		return errors.New("Название 1–100 символов, количество до 40, заметка до 300")
	}
	return nil
}
func (s *Service) CreateBatch(tg int64, circle int, key string, items []Input, now time.Time) error {
	if key == "" || len(key) > 100 || len(items) < 1 || len(items) > 20 {
		return errors.New("Добавьте от 1 до 20 позиций")
	}
	for _, v := range items {
		if e := Validate(v); e != nil {
			return e
		}
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Batch{CircleID: circle, Request: key, CreatedBy: u.ID})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		for _, v := range items {
			row := Item{CircleID: circle, CreatedBy: u.ID, Name: strings.TrimSpace(v.Name), Quantity: strings.TrimSpace(v.Quantity), Note: strings.TrimSpace(v.Note), Status: "needed", Version: 1, UpdatedAt: now}
			if e = tx.Create(&row).Error; e != nil {
				return e
			}
			if e = record(tx, row, u.ID, "create", now); e != nil {
				return e
			}
		}
		return nil
	})
}
func (s *Service) List(tg int64, circle int, bought bool) ([]Item, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var rows []Item
	q := s.DB.Where("circle_id=?", circle)
	if bought {
		q = q.Where("status=?", "bought")
	} else {
		q = q.Where("status IN ?", []string{"needed", "claimed"})
	}
	e := q.Order("id").Find(&rows).Error
	return rows, e
}
func record(tx *gorm.DB, row Item, user int, action string, now time.Time) error {
	return tx.Create(&Event{CircleID: row.CircleID, ItemID: row.ID, UserID: user, Version: row.Version, Action: action, Snapshot: row.Name + " | " + row.Quantity + " | " + row.Note, At: now}).Error
}
func (s *Service) Change(tg int64, circle, id, version int, op string, value *Input, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		u, e := f.Authorize(tg, circle)
		if e != nil {
			return e
		}
		h, e := f.View(tg, circle)
		if e != nil {
			return e
		}
		var row Item
		if tx.First(&row, "id=? AND circle_id=? AND version=?", id, circle, version).Error != nil {
			return family.ErrDenied
		}
		switch op {
		case "claim":
			if row.Status != "needed" {
				return family.ErrDenied
			}
			row.Status = "claimed"
			row.ClaimedBy = u.ID
		case "release":
			if row.Status != "claimed" || row.ClaimedBy != u.ID {
				return family.ErrDenied
			}
			row.Status = "needed"
			row.ClaimedBy = 0
		case "buy":
			if row.Status != "needed" && (row.Status != "claimed" || row.ClaimedBy != u.ID) {
				return family.ErrDenied
			}
			row.Status = "bought"
			row.BoughtBy = u.ID
		case "reopen":
			if row.Status != "bought" || (row.BoughtBy != u.ID && row.CreatedBy != u.ID && !h.Owner) {
				return family.ErrDenied
			}
			row.Status = "needed"
			row.ClaimedBy = 0
			row.BoughtBy = 0
		case "edit", "archive":
			if (row.CreatedBy != u.ID && !h.Owner) || (row.Status != "needed" && row.Status != "claimed") {
				return family.ErrDenied
			}
			if op == "archive" {
				row.Status = "archived"
			} else {
				if value == nil {
					return family.ErrDenied
				}
				if e = Validate(*value); e != nil {
					return e
				}
				row.Name = strings.TrimSpace(value.Name)
				row.Quantity = strings.TrimSpace(value.Quantity)
				row.Note = strings.TrimSpace(value.Note)
			}
		default:
			return family.ErrDenied
		}
		row.Version++
		row.UpdatedAt = now
		r := tx.Model(&Item{}).Where("id=? AND circle_id=? AND version=?", id, circle, version).Select("name", "quantity", "note", "status", "claimed_by", "bought_by", "version", "updated_at").Updates(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return family.ErrDenied
		}
		return record(tx, row, u.ID, op, now)
	})
}

// A departed shopper releases their reservation; purchase history remains.
func (s *Service) ReleaseDeparted(circle int, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var rows []Item
		if e := tx.Where("circle_id=? AND status=? AND claimed_by NOT IN (SELECT user_id FROM user_circles WHERE circle_id=? AND is_active=?)", circle, "claimed", circle, true).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			row.Status = "needed"
			row.ClaimedBy = 0
			row.Version++
			row.UpdatedAt = now
			if e := tx.Save(&row).Error; e != nil {
				return e
			}
			if e := record(tx, row, 0, "departure", now); e != nil {
				return e
			}
		}
		return nil
	})
}
