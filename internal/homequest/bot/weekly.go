package bot

import (
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/stats"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

func weeklyText(r stats.WeeklyReport) string {
	loc, _ := time.LoadLocation(r.Timezone)
	return fmt.Sprintf("%s–%s · %s\nВместе принято дел: %d\nВклад: %d / %d баллов\nТребуют внимания сейчас: %d", r.Start.In(loc).Format("02.01"), r.End.In(loc).AddDate(0, 0, -1).Format("02.01"), r.Timezone, r.Count, r.Points, r.Goal, r.NeedsAttention)
}
func (b *Bot) weekly(uid, msg int64, circle, offset int) error {
	s := &stats.Service{DB: b.DB}
	r, e := s.Weekly(uid, circle, b.Now(), offset)
	if e != nil {
		return e
	}
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	opts := []string{"weekly:-1", "Прошлая неделя", "weekly:0", "Эта неделя"}
	if h.Owner {
		opts = append(opts, "weekly_goal", "Настроить цель")
		label, kind := "Выключить итог", "weekly_digest:off"
		if r.DigestOff {
			label, kind = "Включить итог", "weekly_digest:on"
		}
		opts = append(opts, kind, label)
	}
	opts = append(opts, "household", "Назад")
	return b.card(uid, msg, "Неделя нашего дома", weeklyText(r), circle, opts...)
}
func (b *Bot) weeklyAction(uid, msg int64, a Action) (bool, error) {
	parts := strings.Split(a.Kind, ":")
	switch parts[0] {
	case "weekly":
		if len(parts) != 2 {
			return true, family.ErrDenied
		}
		n, e := strconv.Atoi(parts[1])
		if e != nil {
			return true, e
		}
		return true, b.weekly(uid, msg, a.CircleID, n)
	case "weekly_goal":
		h, e := b.Family.View(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		if !h.Owner {
			return true, b.say(uid, "Цель задаёт владелец дома")
		}
		r, e := (&stats.Service{DB: b.DB}).Weekly(uid, a.CircleID, b.Now(), 0)
		if e != nil {
			return true, e
		}
		if e = b.Family.SetStep(uid, "weekly_goal", strconv.Itoa(a.CircleID)); e != nil {
			return true, e
		}
		return true, b.say(uid, fmt.Sprintf("Цель со следующего понедельника: %d. Введите 1–100000 баллов. Текущая и прошлые недели сохранят свои цели.", r.NextGoal))
	case "weekly_digest":
		if len(parts) != 2 || (parts[1] != "on" && parts[1] != "off") {
			return true, family.ErrDenied
		}
		if e := (&stats.Service{DB: b.DB}).SetDigest(uid, a.CircleID, parts[1] == "on"); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.weekly(uid, msg, a.CircleID, 0)
	}
	return false, nil
}

// Monday 09:00 local, only within a five-minute planning window. No backfill.
// Reuse the durable outbox, unique event keys, quiet hours and current membership.
func (b *Bot) PlanWeekly() error {
	return b.DB.Transaction(func(tx *gorm.DB) error {
		var identities []family.Identity
		if e := tx.Find(&identities).Error; e != nil {
			return e
		}
		f := &family.Service{DB: tx}
		s := &stats.Service{DB: tx}
		for _, id := range identities {
			homes, e := f.Homes(id.TelegramID)
			if e != nil {
				return e
			}
			for _, h := range homes {
				home, e := f.View(id.TelegramID, h.ID)
				if e != nil {
					continue
				}
				loc, e := time.LoadLocation(home.Timezone)
				if e != nil {
					return e
				}
				local := b.Now().In(loc)
				if local.Weekday() != time.Monday || local.Hour() != 9 || local.Minute() >= 5 {
					continue
				}
				r, e := s.Weekly(id.TelegramID, h.ID, b.Now(), -1)
				if e != nil {
					return e
				}
				if r.DigestOff || r.Count == 0 {
					continue
				}
				key := fmt.Sprintf("weekly:%d:%s:%d", h.ID, r.Start.In(loc).Format("2006-01-02"), id.TelegramID)
				raw, _ := json.Marshal(map[string]any{"chat_id": id.TelegramID, "text": "Итог недели\n" + weeklyText(r) + "\nСпасибо за общий уют. /start — открыть дом."})
				row := Delivery{Viewer: id.TelegramID, CircleID: h.ID, EventKey: &key, Notice: true, EventType: "hq_weekly", ScheduledFor: b.Now(), Next: b.Now(), Method: "sendMessage", Payload: string(raw)}
				if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
					return e
				}
			}
		}
		return nil
	})
}
