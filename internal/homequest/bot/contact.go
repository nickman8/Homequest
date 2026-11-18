package bot

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Telegram reachability is personal, separate from family reminder preferences.
// Generation prevents an old in-flight 403 from undoing a newer private update.
type Contact struct {
	Viewer          int64 `gorm:"primaryKey;autoIncrement:false"`
	Generation      int
	Blocked         bool
	LastDeliveredAt time.Time
	LastFailedAt    time.Time
}

func (Contact) TableName() string { return "hq_contacts" }
func (b *Bot) touchContact(uid int64) error {
	return b.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "viewer"}}, DoUpdates: clause.Assignments(map[string]any{"generation": gorm.Expr("generation+1"), "blocked": false})}).Create(&Contact{Viewer: uid, Generation: 1}).Error
}
func (b *Bot) contact(uid int64) (Contact, error) {
	c := Contact{Viewer: uid}
	e := b.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&c).Error
	if e != nil {
		return c, e
	}
	e = b.DB.First(&c, "viewer=?", uid).Error
	return c, e
}
func (b *Bot) contactUnavailable(row Delivery, c Contact) error {
	return b.DB.Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&Contact{}).Where("viewer=? AND generation=?", row.Viewer, c.Generation).Updates(map[string]any{"blocked": true, "last_failed_at": b.Now()})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			// Keep future reminders, but never replay a backlog when the user returns.
			if e := tx.Model(&Delivery{}).Where("viewer=? AND sent=? AND (notice=? OR next<=?)", row.Viewer, false, false, b.Now()).Update("sent", true).Error; e != nil {
				return e
			}
		}
		return tx.Model(&row).Updates(map[string]any{"sent": true, "attempts": 5}).Error
	})
}
func (b *Bot) deliveryLabel(uid int64) (string, error) {
	var c Contact
	e := b.DB.First(&c, "viewer=?", uid).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "Доставка ещё не проверена", nil
	}
	if e != nil {
		return "", e
	}
	if c.Blocked {
		return "Telegram отклонил доставку. Откройте личный диалог с ботом", nil
	}
	if c.LastDeliveredAt.IsZero() || c.LastFailedAt.After(c.LastDeliveredAt) {
		return "Диалог открыт, доставка проверяется", nil
	}
	return "Доставка в личный чат работает", nil
}
