// Package wishlist ports the Item/ItemClaim split from cmintey/wishlist.
// See docs/REUSE.md and third_party/wishlist/LICENSE. Owner DTOs deliberately
// exclude claims entirely; authorization differs from the donor's self-claim mode.
package wishlist

import (
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/url"
	"strings"
)

type Item struct {
	ID       int `gorm:"primaryKey"`
	CircleID int `gorm:"index"`
	UserID   int
	Name     string
	URL      string
	Note     string
	Price    string
}

func (Item) TableName() string { return "hq_wishlist_items" }

// Quantity is one: the primary key makes concurrent reservation atomic.
type ItemClaim struct {
	ItemID      int `gorm:"primaryKey;autoIncrement:false"`
	ClaimedByID int
	Purchased   bool
}

func (ItemClaim) TableName() string { return "hq_wishlist_claims" }

type ClaimView struct {
	Mine      bool `json:"mine"`
	Purchased bool `json:"purchased"`
}
type View struct {
	ID     int        `json:"id"`
	UserID int        `json:"user_id"`
	Name   string     `json:"name"`
	URL    string     `json:"url"`
	Note   string     `json:"note"`
	Price  string     `json:"price"`
	Claim  *ClaimView `json:"claim,omitempty"`
}
type Service struct{ DB *gorm.DB }

func New(db *gorm.DB) (*Service, error) { return &Service{db}, db.AutoMigrate(&Item{}, &ItemClaim{}) }
func (s *Service) Create(tg int64, circle int, name, link, note, price string) (Item, error) {
	var item Item
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || len(link) > 1000 || len([]rune(note)) > 500 || len([]rune(price)) > 40 {
		return item, errors.New("Название до 100, заметка до 500, цена до 40 символов")
	}
	if link != "" {
		u, e := url.Parse(link)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
			return item, errors.New("Нужна обычная ссылка http или https")
		}
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		item = Item{CircleID: circle, UserID: u.ID, Name: name, URL: link, Note: note, Price: price}
		return tx.Create(&item).Error
	})
	return item, e
}
func (s *Service) List(tg int64, circle int) ([]View, error) {
	out := []View{}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var items []Item
		if e = tx.Where("circle_id = ?", circle).Order("id").Find(&items).Error; e != nil {
			return e
		}
		for _, item := range items {
			v := View{ID: item.ID, UserID: item.UserID, Name: item.Name, URL: item.URL, Note: item.Note, Price: item.Price}
			// Never even select hidden fields for the recipient. This DTO is shared by
			// widgets and any future API; AI must use task-only projections.
			if item.UserID != u.ID {
				var claim ItemClaim
				e = tx.First(&claim, "item_id = ?", item.ID).Error
				if e == nil {
					v.Claim = &ClaimView{Mine: claim.ClaimedByID == u.ID, Purchased: claim.Purchased}
				} else if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
			}
			out = append(out, v)
		}
		return nil
	})
	return out, e
}
func (s *Service) Claim(tg int64, circle, id int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var item Item
		if tx.First(&item, "id = ? AND circle_id = ? AND user_id <> ?", id, circle, u.ID).Error != nil {
			return family.ErrDenied
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ItemClaim{ItemID: id, ClaimedByID: u.ID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("Подарок уже забронирован")
		}
		return nil
	})
}
func (s *Service) UpdateClaim(tg int64, circle, id int, action string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		var item Item
		if tx.First(&item, "id = ? AND circle_id = ? AND user_id <> ?", id, circle, u.ID).Error != nil {
			return family.ErrDenied
		}
		var claim ItemClaim
		if tx.First(&claim, "item_id = ? AND claimed_by_id = ?", id, u.ID).Error != nil {
			return family.ErrDenied
		}
		switch action {
		case "purchase":
			return tx.Model(&claim).Update("purchased", true).Error
		case "release":
			return tx.Delete(&claim).Error
		default:
			return family.ErrDenied
		}
	})
}

// A single administrative operation releases all departed donors' reservations.
// It returns no count, item IDs or donor identity, including to gift recipients.
func (s *Service) ReleaseDeparted(tg int64, circle int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		h, e := (&family.Service{DB: tx}).View(tg, circle)
		if e != nil {
			return e
		}
		if !h.Owner {
			return family.ErrDenied
		}
		active := tx.Model(&c.UserCircle{}).Select("user_id").Where("circle_id = ? AND is_active = ?", circle, true)
		items := tx.Model(&Item{}).Select("id").Where("circle_id = ?", circle)
		return tx.Where("item_id IN (?) AND claimed_by_id NOT IN (?)", items, active).Delete(&ItemClaim{}).Error
	})
}
