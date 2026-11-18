package bot

import (
	"donetick.com/core/internal/homequest/billing"
	"donetick.com/core/internal/homequest/stats"
	"errors"
	"fmt"
)

func (b *Bot) premium(uid, msg int64, circle int) error {
	s := &billing.Service{DB: b.DB, Mode: b.BillingMode}
	active, e := s.Active(uid, circle, b.Now())
	if e != nil {
		return e
	}
	h, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	text := "Основные семейные функции доступны всем. Premium сейчас проверяется бесплатно."
	opts := []string{}
	if active {
		text = "Premium включён для дома. Это тестовое право, платежей нет."
	}
	if h.Owner && b.BillingMode == "manual_test" {
		if active {
			opts = append(opts, "premium_off", "Выключить тестовый Premium")
		} else {
			opts = append(opts, "premium_on", "Включить Premium для теста")
		}
	}
	opts = append(opts, "premium_stats", "Расширенный вклад", "settings", "Назад")
	return b.card(uid, msg, "Premium", text, circle, opts...)
}

// The entitlement check is server-side, including direct calls to this handler.
func (b *Bot) premiumStats(uid, msg int64, circle int) error {
	counts, e := (&stats.Service{DB: b.DB}).Extended(uid, circle, b.Now())
	if errors.Is(e, stats.ErrPremium) {
		return b.say(uid, "Расширенный вклад доступен с тестовым Premium.")
	}
	if e != nil {
		return e
	}
	return b.card(uid, msg, "Вклад дома подробнее", fmt.Sprintf("За всё время\nПринято: %d\nОжидают фото: %d\nОжидают проверки: %d\nПовторные попытки: %d", counts["accepted"], counts["photo"], counts["review"], counts["rejected"]), circle, "premium", "Назад")
}
