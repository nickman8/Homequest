package family

import (
	c "donetick.com/core/internal/circle/model"
	"gorm.io/gorm"
)

type HomeChoice struct {
	ID      int
	Name    string
	Current bool
}

// Membership selection deliberately reads only the caller's homes, without
// granting access to any domain entity in a non-current home.
func (s *Service) Homes(tg int64) ([]HomeChoice, error) {
	user, e := actor(s.DB, tg)
	if e != nil {
		return nil, e
	}
	var rows []HomeChoice
	e = s.DB.Table("circles c").Select("c.id,c.name").Joins("JOIN user_circles m ON m.circle_id=c.id").Where("m.user_id=? AND m.is_active=? AND c.disabled=?", user.ID, true, false).Order("c.id").Scan(&rows).Error
	for i := range rows {
		rows[i].Current = rows[i].ID == user.CircleID
	}
	return rows, e
}
func (s *Service) Switch(tg int64, circle int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		var membership c.UserCircle
		if tx.First(&membership, "user_id=? AND circle_id=? AND is_active=?", user.ID, circle, true).Error != nil {
			return ErrDenied
		}
		var home c.Circle
		if tx.First(&home, "id=? AND disabled=?", circle, false).Error != nil {
			return ErrDenied
		}
		if e = tx.Model(&user).Update("circle_id", circle).Error; e != nil {
			return e
		}
		return tx.Model(&Identity{}).Where("telegram_id=?", tg).Updates(map[string]any{"step": "", "draft": ""}).Error
	})
}
