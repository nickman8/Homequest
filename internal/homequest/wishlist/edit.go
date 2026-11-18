package wishlist

import (
	"donetick.com/core/internal/homequest/family"
	"gorm.io/gorm"
	"strings"
)

// Owner mutations never consult claims: their result cannot disclose a surprise.
func (s *Service) Edit(tg int64, circle, id, version int, name, link, note, price string) error {
	name = strings.TrimSpace(name)
	if e := Validate(name, link, note, price); e != nil {
		return e
	}
	return s.mutateOwned(tg, circle, id, version, map[string]any{"name": name, "url": link, "note": note, "price": price, "version": version + 1})
}
func (s *Service) Archive(tg int64, circle, id, version int) error {
	return s.mutateOwned(tg, circle, id, version, map[string]any{"archived": true, "version": version + 1})
}
func (s *Service) mutateOwned(tg int64, circle, id, version int, fields map[string]any) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		result := tx.Model(&Item{}).Where("id=? AND circle_id=? AND user_id=? AND version=? AND archived=?", id, circle, u.ID, version, false).Updates(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return family.ErrDenied
		}
		return nil
	})
}
func (s *Service) Owned(tg int64, circle, id int) (Item, error) {
	var item Item
	u, e := (&family.Service{DB: s.DB}).Authorize(tg, circle)
	if e != nil {
		return item, e
	}
	if s.DB.First(&item, "id=? AND circle_id=? AND user_id=? AND archived=?", id, circle, u.ID, false).Error != nil {
		return Item{}, family.ErrDenied
	}
	return item, nil
}
