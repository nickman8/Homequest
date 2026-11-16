package bot

import (
	ch "donetick.com/core/internal/chore/model"
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	n "donetick.com/core/internal/notifier/model"
	planner "donetick.com/core/internal/notifier/service"
	u "donetick.com/core/internal/user/model"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

type RestPreference struct {
	UserID        int `gorm:"primaryKey;autoIncrement:false"`
	QuietStart    int
	QuietEnd      int
	VacationUntil time.Time
}

func (RestPreference) TableName() string { return "hq_rest_preferences" }
func deliveryTime(now time.Time, zone string, p RestPreference) (time.Time, bool) {
	if p.VacationUntil.After(now) {
		return now, true
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return now, true
	}
	quiet := func(t time.Time) bool {
		h := t.In(loc).Hour()
		if p.QuietStart == p.QuietEnd {
			return false
		}
		if p.QuietStart < p.QuietEnd {
			return h >= p.QuietStart && h < p.QuietEnd
		}
		return h >= p.QuietStart || h < p.QuietEnd
	}
	if !quiet(now) {
		return now, false
	}
	for i := 1; i <= 26*60; i++ {
		next := now.Add(time.Duration(i) * time.Minute)
		if !quiet(next) {
			return next, false
		}
	}
	return now, true
}
func (b *Bot) PlanReminders(offsets []int) error {
	if len(offsets) > 5 {
		return errors.New("at most five reminder offsets")
	}
	return b.DB.Transaction(func(tx *gorm.DB) error {
		var instances []task.Instance
		if e := tx.Where("closed = ? AND pending = ? AND due_at IS NOT NULL", false, false).Find(&instances).Error; e != nil {
			return e
		}
		child := *b
		child.DB = tx
		child.Family = &family.Service{DB: tx}
		for _, instance := range instances {
			var definition ch.Chore
			if e := tx.First(&definition, "id = ? AND is_active = ?", instance.ChoreID, true).Error; e != nil {
				continue
			}
			if definition.Status == ch.ChoreStatusPaused {
				continue
			}
			definition.NextDueDate = instance.DueAt
			definition.NotificationMetadataV2 = &ch.NotificationMetadata{}
			for _, offset := range offsets {
				if offset < -10080 || offset > 10080 {
					return errors.New("reminder offset out of range")
				}
				definition.NotificationMetadataV2.Templates = append(definition.NotificationMetadataV2.Templates, &ch.NotificationTemplate{Value: offset, Unit: ch.NotificationTemplateUnitMinute})
			}
			var recipients []struct {
				TelegramID int64
				UserID     int
			}
			q := tx.Table("hq_identities").Select("hq_identities.telegram_id,hq_identities.user_id").Joins("JOIN user_circles ON user_circles.user_id = hq_identities.user_id").Where("user_circles.circle_id = ? AND user_circles.is_active = ?", instance.CircleID, true)
			if definition.AssignedTo != nil {
				q = q.Where("hq_identities.user_id = ?", *definition.AssignedTo)
			}
			if e := q.Scan(&recipients).Error; e != nil {
				return e
			}
			for _, recipient := range recipients {
				user := &c.UserCircleDetail{UserCircle: c.UserCircle{UserID: recipient.UserID, CircleID: instance.CircleID}, NotificationType: n.NotificationPlatformTelegram, TargetID: strconv.FormatInt(recipient.TelegramID, 10)}
				planned := planner.PlanTemplatesAt(&definition, user, nil, nil, b.Now())
				for _, notification := range planned {
					key := fmt.Sprintf("rem:%d:%d:%d:%d", instance.ID, instance.Version, recipient.UserID, notification.ScheduledFor.Unix())
					row := Delivery{Method: "sendMessage", Viewer: recipient.TelegramID, CircleID: instance.CircleID, InstanceID: instance.ID, Version: instance.Version, EventKey: &key, Notice: true, Next: notification.ScheduledFor, ScheduledFor: notification.ScheduledFor, EventType: fmt.Sprint(notification.RawEvent["type"])}
					r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
					if r.Error != nil {
						return r.Error
					}
					if r.RowsAffected == 0 {
						continue
					}
					button, e := child.button(recipient.TelegramID, instance.CircleID, fmt.Sprintf("snooze:%d", row.ID), "Через 30 минут")
					if e != nil {
						return e
					}
					text := "Мягкое напоминание: " + definition.Name + ". Откройте /start → Сегодня."
					raw, e := json.Marshal(map[string]any{"chat_id": recipient.TelegramID, "text": text, "reply_markup": map[string]any{"inline_keyboard": [][]map[string]string{{{"text": button.Label, "callback_data": button.Action}}}}})
					if e != nil {
						return e
					}
					if e = tx.Model(&row).Update("payload", string(raw)).Error; e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
}
func (b *Bot) Snooze(uid int64, id uint, d time.Duration) error {
	if d < time.Minute || d > 24*time.Hour {
		return errors.New("invalid snooze duration")
	}
	return b.DB.Transaction(func(tx *gorm.DB) error {
		var row Delivery
		if tx.First(&row, "id = ? AND viewer = ? AND notice = ?", id, uid, true).Error != nil {
			return family.ErrDenied
		}
		if _, e := (&family.Service{DB: tx}).Authorize(uid, row.CircleID); e != nil {
			return e
		}
		var instance task.Instance
		if tx.First(&instance, "id = ? AND version = ? AND closed = ? AND pending = ?", row.InstanceID, row.Version, false, false).Error != nil {
			return family.ErrDenied
		}
		return tx.Model(&row).Updates(map[string]any{"sent": false, "next": b.Now().Add(d), "scheduled_for": b.Now().Add(d), "attempts": 0}).Error
	})
}
func (b *Bot) noticeReady(row Delivery) (bool, error) {
	if row.InstanceID != 0 {
		var instance task.Instance
		if b.DB.First(&instance, "id = ? AND circle_id = ? AND version = ? AND closed = ? AND pending = ?", row.InstanceID, row.CircleID, row.Version, false, false).Error != nil {
			return false, b.DB.Model(&row).Update("sent", true).Error
		}
		var definition ch.Chore
		if b.DB.First(&definition, "id = ? AND is_active = ? AND status != ?", instance.ChoreID, true, ch.ChoreStatusPaused).Error != nil {
			return false, b.DB.Model(&row).Update("sent", true).Error
		}
	}
	if !row.Notice {
		return true, nil
	}
	if !row.ScheduledFor.IsZero() && b.Now().Sub(row.ScheduledFor) > 5*time.Minute {
		return false, b.DB.Model(&row).Update("sent", true).Error
	}
	user, e := b.Family.Authorize(row.Viewer, row.CircleID)
	if e != nil {
		return false, b.DB.Model(&row).Update("sent", true).Error
	}
	pref := u.DefaultNotificationPreference(user.ID)
	e = b.DB.First(pref, "user_id = ?", user.ID).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return false, e
	}
	if !pref.Enabled(u.NotificationEventType(row.EventType)) {
		return false, b.DB.Model(&row).Update("sent", true).Error
	}
	rest := RestPreference{UserID: user.ID, QuietStart: 22, QuietEnd: 8}
	e = b.DB.First(&rest, "user_id = ?", user.ID).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return false, e
	}
	home, e := b.Family.Current(row.Viewer)
	if e != nil {
		return false, e
	}
	next, drop := deliveryTime(b.Now(), home.Timezone, rest)
	if drop {
		return false, b.DB.Model(&row).Update("sent", true).Error
	}
	if next.After(b.Now()) {
		return false, b.DB.Model(&row).Updates(map[string]any{"next": next, "scheduled_for": next}).Error
	}
	return true, nil
}
func (b *Bot) SetRest(uid int64, mute bool, quietStart, quietEnd int, vacation time.Time) error {
	if quietStart < 0 || quietStart > 23 || quietEnd < 0 || quietEnd > 23 {
		return errors.New("Часы: от 0 до 23")
	}
	home, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	user, e := b.Family.Authorize(uid, home.ID)
	if e != nil {
		return e
	}
	return b.DB.Transaction(func(tx *gorm.DB) error {
		profile := u.NotificationProfileBalanced
		if mute {
			profile = u.NotificationProfileQuiet
		}
		if e := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&u.UserNotificationPreference{UserID: user.ID, Profile: profile}).Error; e != nil {
			return e
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&RestPreference{UserID: user.ID, QuietStart: quietStart, QuietEnd: quietEnd, VacationUntil: vacation}).Error
	})
}

func (b *Bot) notice(uid int64, circle int, event, text string) error {
	raw, e := json.Marshal(map[string]any{"chat_id": uid, "text": text})
	if e != nil {
		return e
	}
	return b.DB.Create(&Delivery{Viewer: uid, CircleID: circle, Method: "sendMessage", Payload: string(raw), Notice: true, EventType: event, Next: b.Now(), ScheduledFor: b.Now()}).Error
}
func (b *Bot) restMessage(uid int64, text string) error {
	var start, end, days int
	mute := false
	vacation := time.Time{}
	switch {
	case text == "выкл":
		mute = true
		start = 22
		end = 8
	case strings.HasPrefix(text, "вкл "):
		if _, e := fmt.Sscanf(text, "вкл %d %d", &start, &end); e != nil {
			return b.say(uid, "Пример: вкл 22 8")
		}
	default:
		if _, e := fmt.Sscanf(text, "отпуск %d", &days); e != nil || days < 1 || days > 365 {
			return b.say(uid, "Примеры: вкл 22 8; выкл; отпуск 7")
		}
		start = 22
		end = 8
		vacation = b.Now().AddDate(0, 0, days)
	}
	if e := b.SetRest(uid, mute, start, end, vacation); e != nil {
		return b.say(uid, e.Error())
	}
	if e := b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.home(uid, 0)
}
