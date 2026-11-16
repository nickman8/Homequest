package bot

import (
	"donetick.com/core/internal/homequest/wishlist"
	"fmt"
	"strconv"
	"strings"
)

func short(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
func (b *Bot) wishes(uid, msg int64, circle, page int) error {
	s := &wishlist.Service{DB: b.DB}
	rows, e := s.List(uid, circle)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Подарки близким", "Добавьте желание: баллы здесь не нужны.", circle, "wish_new", "Добавить желание", "home", "Дом")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	u, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	text := short(r.Note, 60) + "\n" + short(r.Price, 30)
	opts := []string{}
	if r.UserID == u.ID {
		text = "Моё желание\n" + text
	} else if r.Claim == nil {
		opts = append(opts, fmt.Sprintf("wish_claim:%d", r.ID), "Забронировать тайно")
	} else if r.Claim.Mine {
		if r.Claim.Purchased {
			text = "Вы купили этот подарок\n" + text
		} else {
			opts = append(opts, fmt.Sprintf("wish_purchase:%d", r.ID), "Уже куплено")
		}
		opts = append(opts, fmt.Sprintf("wish_release:%d", r.ID), "Снять мою бронь")
	} else {
		text = "Подарок уже выбрал другой участник\n" + text
	}
	opts = append(opts, fmt.Sprintf("wish_info:%d", r.ID), "Подробнее", fmt.Sprintf("wishes:%d", (page+1)%len(rows)), "Далее", "wish_new", "Добавить", "home", "Дом")
	return b.card(uid, msg, short(r.Name, 40), text, circle, opts...)
}
func (b *Bot) wishMessage(uid int64, text string) error {
	h, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	p := strings.SplitN(text, "|", 4)
	for len(p) < 4 {
		p = append(p, "")
	}
	for i := range p {
		p[i] = strings.TrimSpace(p[i])
	}
	if _, e = (&wishlist.Service{DB: b.DB}).Create(uid, h.ID, p[0], p[1], p[2], p[3]); e != nil {
		return b.say(uid, e.Error())
	}
	if e = b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.wishes(uid, 0, h.ID, 0)
}
func (b *Bot) wishAction(uid, msg int64, a Action) (bool, error) {
	parts := strings.Split(a.Kind, ":")
	id := 0
	if len(parts) > 1 {
		id, _ = strconv.Atoi(parts[1])
	}
	s := &wishlist.Service{DB: b.DB}
	switch parts[0] {
	case "wishes":
		return true, b.wishes(uid, msg, a.CircleID, id)
	case "wish_new":
		if e := b.Family.SetStep(uid, "wish_new", ""); e != nil {
			return true, e
		}
		return true, b.say(uid, "Введите: название | ссылка | заметка | примерная цена. Достаточно названия; остальные поля необязательны. /cancel — отмена")
	case "wish_claim", "wish_purchase", "wish_release":
		var e error
		if parts[0] == "wish_claim" {
			e = s.Claim(uid, a.CircleID, id)
		} else {
			e = s.UpdateClaim(uid, a.CircleID, id, strings.TrimPrefix(parts[0], "wish_"))
		}
		if e != nil {
			return true, b.say(uid, "Бронь изменилась или действие недоступно. Откройте список заново.")
		}
		return true, b.wishes(uid, msg, a.CircleID, 0)
	case "wish_info":
		rows, e := s.List(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		for _, r := range rows {
			if r.ID == id {
				return true, b.say(uid, r.Name+"\n"+r.Note+"\n"+r.Price+"\n"+r.URL)
			}
		}
		return true, b.say(uid, "Желание недоступно")
	case "wish_cleanup":
		if e := s.ReleaseDeparted(uid, a.CircleID); e != nil {
			return true, b.say(uid, "Действие доступно владельцу дома")
		}
		return true, b.say(uid, "Правила для ушедших участников применены: их брони освобождены, если они были.")
	}
	return false, nil
}
