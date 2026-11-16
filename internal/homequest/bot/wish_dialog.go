package bot

import (
	"crypto/rand"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/wishlist"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type wishDraft struct {
	Nonce                   string
	Revision                int
	Circle, ID, Version     int
	Name, Link, Note, Price string
	Step                    int
}

var wishSteps = []string{"wish_name", "wish_link", "wish_note", "wish_price", "wish_review"}

func (b *Bot) startWish(uid, msg int64, circle, id int) error {
	d := wishDraft{Circle: circle, Revision: 1}
	nonce := make([]byte, 8)
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	d.Nonce = hex.EncodeToString(nonce)
	if id != 0 {
		item, e := (&wishlist.Service{DB: b.DB}).Owned(uid, circle, id)
		if e != nil {
			return b.say(uid, "Это желание нельзя изменить")
		}
		d.ID = item.ID
		d.Version = item.Version
		d.Name = item.Name
		d.Link = item.URL
		d.Note = item.Note
		d.Price = item.Price
	}
	return b.saveWishDraft(uid, msg, d, "")
}
func (b *Bot) loadWishDraft(uid int64) (wishDraft, error) {
	var d wishDraft
	state, e := b.Family.Step(uid)
	if e != nil {
		return d, e
	}
	if json.Unmarshal([]byte(state.Draft), &d) != nil || d.Nonce == "" || d.Step < 0 || d.Step >= len(wishSteps) || state.Step != wishSteps[d.Step] {
		return d, family.ErrDenied
	}
	_, e = b.Family.Authorize(uid, d.Circle)
	return d, e
}
func (b *Bot) saveWishDraft(uid, msg int64, d wishDraft, hint string) error {
	raw, e := json.Marshal(d)
	if e != nil {
		return e
	}
	if e = b.Family.SetStep(uid, wishSteps[d.Step], string(raw)); e != nil {
		return e
	}
	title := fmt.Sprintf("Моё желание · %d/4", d.Step+1)
	prompt := []string{"Как называется желание?", "Пришлите ссылку http/https или пропустите.", "Добавьте заметку: например, цвет или размер.", "Укажите примерную цену или пропустите."}
	values := []string{d.Name, d.Link, d.Note, d.Price}
	opts := []string{}
	action := func(op, label string) {
		opts = append(opts, fmt.Sprintf("wish_flow:%s:%d:%s", d.Nonce, d.Revision, op), label)
	}
	text := ""
	if d.Step == 4 {
		title = "Проверьте желание"
		text = short(d.Name, 35) + "\nСсылка: " + short(d.Link, 30) + "\nЗаметка: " + short(d.Note, 35) + "\nЦена: " + short(d.Price, 20)
		action("save", "Сохранить")
	} else {
		text = prompt[d.Step]
		if values[d.Step] != "" {
			text += "\nСейчас: " + short(values[d.Step], 70)
		}
		if d.ID != 0 {
			action("next", "Оставить")
			if d.Step > 0 {
				action("clear", "Убрать поле")
			}
		} else if d.Step > 0 {
			action("clear", "Пропустить")
		}
	}
	if hint != "" {
		text = short(hint, 45) + "\n" + text
	}
	action("back", "Назад")
	action("cancel", "Отмена")
	return b.card(uid, msg, title, text, d.Circle, opts...)
}
func (b *Bot) wishDraftMessage(uid int64, text string) error {
	d, e := b.loadWishDraft(uid)
	if e != nil {
		return b.say(uid, "Откройте список подарков и начните заново")
	}
	if d.Step == 4 {
		return b.saveWishDraft(uid, 0, d, "Проверьте поля и нажмите «Сохранить».")
	}
	text = strings.TrimSpace(text)
	// Existing compact input remains supported, but also gets a confirmation screen.
	if d.Step == 0 && strings.Contains(text, "|") {
		p := strings.SplitN(text, "|", 4)
		for len(p) < 4 {
			p = append(p, "")
		}
		for i := range p {
			p[i] = strings.TrimSpace(p[i])
		}
		if e = wishlist.Validate(p[0], p[1], p[2], p[3]); e != nil {
			return b.saveWishDraft(uid, 0, d, e.Error())
		}
		d.Name, d.Link, d.Note, d.Price = p[0], p[1], p[2], p[3]
		d.Step = 4
	} else {
		next := d
		switch d.Step {
		case 0:
			next.Name = text
		case 1:
			next.Link = text
		case 2:
			next.Note = text
		case 3:
			next.Price = text
		}
		if e = wishlist.Validate(next.Name, next.Link, next.Note, next.Price); e != nil {
			return b.saveWishDraft(uid, 0, d, e.Error())
		}
		d = next
		d.Step++
	}
	d.Revision++
	return b.saveWishDraft(uid, 0, d, "")
}
func (b *Bot) wishFlow(uid, msg int64, parts []string) error {
	d, e := b.loadWishDraft(uid)
	if e != nil {
		return b.say(uid, "Этот черновик уже закрыт. Откройте список подарков.")
	}
	if len(parts) != 4 || parts[1] != d.Nonce || parts[2] != strconv.Itoa(d.Revision) {
		return b.saveWishDraft(uid, msg, d, "Это старая кнопка. Продолжите здесь.")
	}
	op := parts[3]
	switch op {
	case "cancel":
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.wishes(uid, msg, d.Circle, 0)
	case "back":
		if d.Step == 0 {
			if e = b.Family.SetStep(uid, "", ""); e != nil {
				return e
			}
			return b.wishes(uid, msg, d.Circle, 0)
		}
		d.Step--
	case "next", "clear":
		if d.Step >= 4 || (op == "next" && d.ID == 0) || (op == "clear" && d.Step == 0) {
			return family.ErrDenied
		}
		if op == "clear" {
			switch d.Step {
			case 1:
				d.Link = ""
			case 2:
				d.Note = ""
			case 3:
				d.Price = ""
			}
		}
		d.Step++
	case "save":
		if d.Step != 4 {
			return family.ErrDenied
		}
		s := &wishlist.Service{DB: b.DB}
		if d.ID == 0 {
			_, e = s.Create(uid, d.Circle, d.Name, d.Link, d.Note, d.Price)
		} else {
			e = s.Edit(uid, d.Circle, d.ID, d.Version, d.Name, d.Link, d.Note, d.Price)
		}
		if e != nil {
			if e = b.Family.SetStep(uid, "", ""); e != nil {
				return e
			}
			return b.say(uid, "Желание изменилось или стало недоступно. Откройте свежий список подарков.")
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.wishes(uid, msg, d.Circle, 0)
	default:
		return family.ErrDenied
	}
	d.Revision++
	return b.saveWishDraft(uid, msg, d, "")
}
