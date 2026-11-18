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
	text := short(r.Note, 40) + "\n" + short(r.Price, 20)
	opts := []string{}
	if r.UserID == u.ID {
		text = "Моё желание\n" + text
		opts = append(opts, fmt.Sprintf("wish_edit:%d", r.ID), "Править", fmt.Sprintf("wish_archive:%d", r.ID), "Убрать")
	} else if r.Claim == nil {
		opts = append(opts, fmt.Sprintf("wish_claim:%d", r.ID), "Забронировать тайно")
	} else if r.Claim.Mine {
		if r.Claim.Changed {
			text = "Желание обновлено — проверьте детали.\n" + text
		}
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
		return true, b.startWish(uid, msg, a.CircleID, 0)
	case "wish_flow":
		return true, b.wishFlow(uid, msg, parts)
	case "wish_edit":
		return true, b.startWish(uid, msg, a.CircleID, id)
	case "wish_archive":
		if _, e := s.Owned(uid, a.CircleID, id); e != nil {
			return true, b.say(uid, "Это желание нельзя изменить")
		}
		return true, b.card(uid, msg, "Убрать желание?", "Оно исчезнет из семейного списка.", a.CircleID, fmt.Sprintf("wish_archive_confirm:%d", id), "Да, убрать", "wishes:0", "Назад")
	case "wish_archive_confirm":
		if e := s.Archive(uid, a.CircleID, id, a.Version); e != nil {
			return true, b.say(uid, "Желание изменилось. Откройте список заново.")
		}
		return true, b.wishes(uid, msg, a.CircleID, 0)
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
