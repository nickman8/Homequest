package bot

import (
	"donetick.com/core/internal/homequest/family"
	u "donetick.com/core/internal/user/model"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ReminderPlan struct {
	CircleID  int `gorm:"primaryKey;autoIncrement:false"`
	Version   int
	Offsets   string
	ChangedAt time.Time
}

func (ReminderPlan) TableName() string { return "hq_reminder_plans" }
func (b *Bot) reminderPlan(circle int, defaults []int) (ReminderPlan, []int, error) {
	p := ReminderPlan{CircleID: circle}
	e := b.DB.First(&p, "circle_id=?", circle).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return p, defaults, nil
	}
	if e != nil {
		return p, nil, e
	}
	var offsets []int
	if e = json.Unmarshal([]byte(p.Offsets), &offsets); e != nil {
		return p, nil, e
	}
	return p, offsets, nil
}
func (b *Bot) SetReminderPlan(uid int64, circle, version int, offsets []int) error {
	if len(offsets) > 5 {
		return errors.New("До пяти напоминаний")
	}
	values := append([]int{}, offsets...)
	sort.Ints(values)
	after := 0
	for i, n := range values {
		if n < -10080 || n > 10080 || (i > 0 && values[i-1] == n) {
			return errors.New("Интервалы до 7 дней, без повторов")
		}
		if n > 0 {
			after++
		}
	}
	if after > 1 {
		return errors.New("После срока — только одно мягкое напоминание")
	}
	return b.DB.Transaction(func(tx *gorm.DB) error {
		h, e := (&family.Service{DB: tx}).View(uid, circle)
		if e != nil {
			return e
		}
		if !h.Owner {
			return family.ErrDenied
		}
		child := *b
		child.DB = tx
		p, _, e := child.reminderPlan(circle, nil)
		if e != nil {
			return e
		}
		if p.Version != version {
			return errors.New("Настройки изменились. Откройте свежую карточку")
		}
		raw, _ := json.Marshal(values)
		p = ReminderPlan{CircleID: circle, Version: version + 1, Offsets: string(raw), ChangedAt: b.Now()}
		if e = tx.Save(&p).Error; e != nil {
			return e
		}
		return tx.Model(&Delivery{}).Where("circle_id=? AND notice=? AND instance_id<>? AND sent=?", circle, true, 0, false).Update("sent", true).Error
	})
}
func planText(offsets []int) string {
	if len(offsets) == 0 {
		return "Напоминания по сроку выключены"
	}
	parts := []string{}
	for _, n := range offsets {
		switch {
		case n < 0:
			parts = append(parts, fmt.Sprintf("за %d мин.", -n))
		case n == 0:
			parts = append(parts, "в срок")
		default:
			parts = append(parts, fmt.Sprintf("через %d мин.", n))
		}
	}
	return strings.Join(parts, ", ")
}
func (b *Bot) effectiveDefaults() []int {
	if b.ReminderOffsets == nil {
		return []int{-60, 0, 120}
	}
	return b.ReminderOffsets
}
func (b *Bot) notifications(uid, msg int64, circle int) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	_, offsets, e := b.reminderPlan(circle, b.effectiveDefaults())
	if e != nil {
		return e
	}
	user, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	pref := u.DefaultNotificationPreference(user.ID)
	if e = b.DB.First(pref, "user_id=?", user.ID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	rest := RestPreference{UserID: user.ID, QuietStart: 22, QuietEnd: 8}
	if e = b.DB.First(&rest, "user_id=?", user.ID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	status := "Личные напоминания включены"
	if pref.Profile == u.NotificationProfileQuiet {
		status = "Личные напоминания выключены"
	}
	text := status + "\nТихие часы: " + fmt.Sprintf("%02d–%02d", rest.QuietStart, rest.QuietEnd) + "\nДом: " + planText(offsets)
	if rest.VacationUntil.After(b.Now()) {
		text += "\nОтпуск до " + rest.VacationUntil.Format("02.01")
	}
	delivery, e := b.deliveryLabel(uid)
	if e != nil {
		return e
	}
	text += "\n" + delivery
	opts := []string{"rest", "Личные настройки"}
	if h.Owner {
		opts = append(opts, "family_reminders", "Для всего дома")
	}
	opts = append(opts, "home", "Дом")
	return b.card(uid, msg, "Напоминания", text, circle, opts...)
}
func (b *Bot) familyReminders(uid, msg int64, circle int) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	if !h.Owner {
		return b.say(uid, "Настройки дома доступны владельцу")
	}
	p, offsets, e := b.reminderPlan(circle, b.effectiveDefaults())
	if e != nil {
		return e
	}
	opts := []string{}
	for _, pair := range [][2]string{{"gentle", "Мягкий режим"}, {"due", "Только в срок"}, {"day", "За день и в срок"}, {"off", "Выключить по сроку"}, {"custom", "Другие интервалы"}} {
		opts = append(opts, fmt.Sprintf("rem_plan:%d:%s", p.Version, pair[0]), pair[1])
	}
	opts = append(opts, "notifications", "Назад")
	return b.card(uid, msg, "Напоминания дома", planText(offsets)+"\nИзменения действуют для будущих напоминаний. Сроки задач сохраняются.", circle, opts...)
}
func (b *Bot) reminderAction(uid, msg int64, a Action) (bool, error) {
	parts := strings.Split(a.Kind, ":")
	if a.Kind == "family_reminders" {
		return true, b.familyReminders(uid, msg, a.CircleID)
	}
	if parts[0] != "rem_plan" {
		return false, nil
	}
	if len(parts) != 3 {
		return true, family.ErrDenied
	}
	version, e := strconv.Atoi(parts[1])
	if e != nil {
		return true, e
	}
	h, e := b.Family.View(uid, a.CircleID)
	if e != nil || !h.Owner {
		return true, b.say(uid, "Настройки дома доступны владельцу")
	}
	p, _, e := b.reminderPlan(a.CircleID, nil)
	if e != nil {
		return true, e
	}
	if p.Version != version {
		return true, b.familyReminders(uid, msg, a.CircleID)
	}
	if parts[2] == "custom" {
		if e = b.Family.SetStep(uid, "reminder_plan", fmt.Sprintf("%d:%d", a.CircleID, version)); e != nil {
			return true, e
		}
		return true, b.say(uid, "Когда напоминать? Например: за 60, в срок, через 120. Числа — минуты. До пяти интервалов, после срока — один.")
	}
	presets := map[string][]int{"gentle": {-60, 0, 120}, "due": {0}, "day": {-1440, 0}, "off": {}}
	offsets, ok := presets[parts[2]]
	if !ok {
		return true, family.ErrDenied
	}
	if e = b.SetReminderPlan(uid, a.CircleID, version, offsets); e != nil {
		return true, b.say(uid, e.Error())
	}
	return true, b.familyReminders(uid, msg, a.CircleID)
}
func (b *Bot) reminderPlanMessage(uid int64, draft, text string) error {
	var circle, version int
	if _, e := fmt.Sscanf(draft, "%d:%d", &circle, &version); e != nil {
		return e
	}
	offsets := []int{}
	for _, part := range strings.Split(strings.ToLower(text), ",") {
		words := strings.Fields(part)
		n := 0
		if strings.Join(words, " ") != "в срок" {
			if len(words) != 2 || (words[0] != "за" && words[0] != "через") {
				return b.say(uid, "Пример: за 60, в срок, через 120")
			}
			value, e := strconv.Atoi(words[1])
			if e != nil || value < 1 {
				return b.say(uid, "Укажите положительное целое число минут")
			}
			n = value
			if words[0] == "за" {
				n = -n
			}
		}
		offsets = append(offsets, n)
	}
	if e := b.SetReminderPlan(uid, circle, version, offsets); e != nil {
		return b.say(uid, e.Error())
	}
	if e := b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.familyReminders(uid, 0, circle)
}
