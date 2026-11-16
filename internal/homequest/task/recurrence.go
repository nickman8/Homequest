package task

import (
	"context"
	scheduler "donetick.com/core/internal/chore"
	ch "donetick.com/core/internal/chore/model"
	"donetick.com/core/internal/homequest/family"
	"errors"
	"gorm.io/gorm"
	"time"
)

type Schedule struct {
	Mode   string
	Days   []string
	Every  int
	Hour   int
	Minute int
}

func (s *Service) SetSchedule(tg int64, circle, id int, rule Schedule, now time.Time) error {
	if rule.Hour < 0 || rule.Hour > 23 || rule.Minute < 0 || rule.Minute > 59 {
		return errors.New("Некорректное время")
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
		var instance Instance
		if tx.First(&instance, "id = ? AND circle_id = ? AND closed = ? AND pending = ?", id, circle, false, false).Error != nil {
			return family.ErrDenied
		}
		var definition ch.Chore
		if e = tx.First(&definition, instance.ChoreID).Error; e != nil {
			return e
		}
		if definition.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		loc, e := time.LoadLocation(home.Timezone)
		if e != nil {
			return e
		}
		local := now.In(loc)
		base := time.Date(local.Year(), local.Month(), local.Day(), rule.Hour, rule.Minute, 0, 0, loc).AddDate(0, 0, -1)
		definition.FrequencyMetadataV2 = &ch.FrequencyMetadata{Timezone: home.Timezone, Time: base.Format(time.RFC3339)}
		definition.NextDueDate = &base
		definition.IsRolling = false
		switch rule.Mode {
		case "once":
			definition.FrequencyType = ch.FrequencyTypeOnce
			definition.NextDueDate = nil
		case "daily":
			definition.FrequencyType = ch.FrequencyTypeDaily
		case "weekdays":
			definition.FrequencyType = ch.FrequencyTypeDayOfTheWeek
			if len(rule.Days) == 0 || len(rule.Days) > 7 {
				return errors.New("Выберите дни недели")
			}
			for _, d := range rule.Days {
				switch d {
				case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
					v := d
					definition.FrequencyMetadataV2.Days = append(definition.FrequencyMetadataV2.Days, &v)
				default:
					return errors.New("Некорректный день недели")
				}
			}
		case "interval", "after":
			if rule.Every < 1 || rule.Every > 365 {
				return errors.New("Интервал: от 1 до 365 дней")
			}
			definition.FrequencyType = ch.FrequencyTypeInterval
			definition.Frequency = rule.Every
			unit := "days"
			definition.FrequencyMetadataV2.Unit = &unit
			definition.IsRolling = rule.Mode == "after"
		default:
			return errors.New("Неизвестный режим повторения")
		}
		next, e := scheduler.NextAfterAcceptance(context.Background(), &definition, now)
		if e != nil {
			return e
		}
		definition.NextDueDate = next
		if e = tx.Model(&definition).Select("frequency_type", "frequency", "frequency_meta_v2", "next_due_date", "is_rolling").Updates(&definition).Error; e != nil {
			return e
		}
		return tx.Model(&instance).Updates(map[string]any{"due_at": next, "version": gorm.Expr("version + 1")}).Error
	})
}

func (s *Service) SetDue(tg int64, circle, id int, text string) error {
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
		var instance Instance
		if tx.First(&instance, "id = ? AND circle_id = ? AND closed = ? AND pending = ?", id, circle, false, false).Error != nil {
			return family.ErrDenied
		}
		var definition ch.Chore
		if tx.First(&definition, instance.ChoreID).Error != nil {
			return family.ErrDenied
		}
		if definition.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		var due *time.Time
		if text != "без срока" {
			value, e := time.Parse("2006-01-02 15:04", text)
			if e != nil {
				return errors.New("Пример срока: 2026-11-20 19:00")
			}
			loc, e := time.LoadLocation(home.Timezone)
			if e != nil {
				return e
			}
			resolved, e := scheduler.ResolveLocalTime(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), loc)
			if e != nil {
				return e
			}
			utc := resolved.UTC()
			due = &utc
		}
		if e = tx.Model(&definition).Update("next_due_date", due).Error; e != nil {
			return e
		}
		return tx.Model(&instance).Updates(map[string]any{"due_at": due, "version": gorm.Expr("version + 1")}).Error
	})
}

// ChangeTimezone preserves wall-clock slots for recurring tasks. One-off UTC
// deadlines and historical attempts remain fixed; only future rules are moved.
func (s *Service) ChangeTimezone(tg int64, zone string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		home, e := f.Current(tg)
		if e != nil {
			return e
		}
		oldLoc, e := time.LoadLocation(home.Timezone)
		if e != nil {
			return e
		}
		if e = f.SetTimezone(tg, zone); e != nil {
			return e
		}
		loc, e := time.LoadLocation(zone)
		if e != nil {
			return e
		}
		var instances []Instance
		if e = tx.Where("circle_id = ? AND closed = ?", home.ID, false).Find(&instances).Error; e != nil {
			return e
		}
		for _, instance := range instances {
			var definition ch.Chore
			if e = tx.First(&definition, instance.ChoreID).Error; e != nil {
				return e
			}
			if definition.FrequencyType == ch.FrequencyTypeOnce || definition.FrequencyType == ch.FrequencyTypeNoRepeat {
				continue
			}
			meta := definition.FrequencyMetadataV2
			if meta == nil {
				return errors.New("schedule metadata missing")
			}
			meta.Timezone = zone
			if meta.Time != "" {
				t, e := time.Parse(time.RFC3339, meta.Time)
				if e != nil {
					return e
				}
				t = t.In(oldLoc)
				v, e := scheduler.ResolveLocalTime(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), loc)
				if e != nil {
					return e
				}
				meta.Time = v.In(loc).Format(time.RFC3339)
			}
			var due *time.Time
			if instance.DueAt != nil {
				t := instance.DueAt.In(oldLoc)
				v, e := scheduler.ResolveLocalTime(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), loc)
				if e != nil {
					return e
				}
				utc := v.UTC()
				due = &utc
			}
			definition.NextDueDate = due
			if e = tx.Model(&definition).Select("frequency_meta_v2", "next_due_date").Updates(&definition).Error; e != nil {
				return e
			}
			if e = tx.Model(&instance).Updates(map[string]any{"due_at": due, "version": gorm.Expr("version + 1")}).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
