package bot

import (
	"crypto/rand"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/shopping"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type shoppingDraft struct {
	Nonce                                         string
	Revision, Circle, ItemID, Version, Step, Page int
	Items                                         []shopping.Input
}

func (b *Bot) shopping(uid, msg int64, circle, page int, bought bool) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	u, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	s := &shopping.Service{DB: b.DB}
	if e = s.ReleaseDeparted(circle, b.Now()); e != nil {
		return e
	}
	rows, e := s.List(uid, circle, bought)
	if e != nil {
		return e
	}
	mode := 0
	other, label := "shopping:0:1", "История покупок"
	if bought {
		mode = 1
		other, label = "shopping:0:0", "Нужно купить"
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Покупки для дома", "Список пуст. Покупки не меняют игровые баллы.", circle, "shop_new", "Добавить покупку", other, label, "household", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	text := short(r.Quantity, 20) + "\n" + short(r.Note, 40) + "\n"
	opts := []string{}
	op := func(kind, label string) {
		opts = append(opts, fmt.Sprintf("shop_change:%d:%d:%s", r.ID, r.Version, kind), label)
	}
	switch r.Status {
	case "needed":
		text += "Нужно купить"
		op("claim", "Куплю я")
		op("buy", "Куплено")
	case "claimed":
		text += "Уже взял на себя участник"
		for _, m := range h.Members {
			if m.ID == r.ClaimedBy {
				text = "Покупает: " + short(m.Name, 20) + "\n" + text
				break
			}
		}
		if r.ClaimedBy == u.ID {
			op("buy", "Куплено")
			op("release", "Освободить")
		}
	case "bought":
		text += "Куплено"
		if r.BoughtBy == u.ID || r.CreatedBy == u.ID || h.Owner {
			op("reopen", "Вернуть в список")
		}
	}
	opts = append(opts, fmt.Sprintf("shop_manage:%d:%d", r.ID, r.Version), "Параметры", fmt.Sprintf("shopping:%d:%d", (page+1)%len(rows), mode), "Следующая", other, label, "household", "Назад")
	return b.card(uid, msg, short(r.Name, 35), text, circle, opts...)
}
func (b *Bot) startShopping(uid, msg int64, circle, id, version int, items []shopping.Input) error {
	d := shoppingDraft{Circle: circle, Revision: 1, ItemID: id, Version: version, Items: items}
	nonce := make([]byte, 8)
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	d.Nonce = hex.EncodeToString(nonce)
	if len(items) > 0 {
		d.Step = 3
	} else {
		d.Items = []shopping.Input{{}}
	}
	if id != 0 {
		rows, e := (&shopping.Service{DB: b.DB}).List(uid, circle, false)
		if e != nil {
			return e
		}
		found := false
		for _, r := range rows {
			if r.ID == id && r.Version == version {
				d.Items = []shopping.Input{{Name: r.Name, Quantity: r.Quantity, Note: r.Note}}
				found = true
			}
		}
		if !found {
			return b.say(uid, "Покупка уже изменилась")
		}
	}
	return b.saveShopping(uid, msg, d, "")
}
func (b *Bot) loadShopping(uid int64) (shoppingDraft, error) {
	var d shoppingDraft
	s, e := b.Family.Step(uid)
	if e != nil {
		return d, e
	}
	if s.Step != "shopping_draft" || json.Unmarshal([]byte(s.Draft), &d) != nil || d.Nonce == "" || len(d.Items) < 1 || d.Page < 0 || d.Page >= len(d.Items) || d.Step < 0 || d.Step > 3 {
		return d, family.ErrDenied
	}
	h, e := b.Family.Current(uid)
	if e != nil || h.ID != d.Circle {
		return d, family.ErrDenied
	}
	return d, nil
}
func (b *Bot) saveShopping(uid, msg int64, d shoppingDraft, hint string) error {
	raw, _ := json.Marshal(d)
	if e := b.Family.SetStep(uid, "shopping_draft", string(raw)); e != nil {
		return e
	}
	v := d.Items[d.Page]
	op := func(kind string) string { return fmt.Sprintf("shop_flow:%s:%d:%s", d.Nonce, d.Revision, kind) }
	opts := []string{}
	title := fmt.Sprintf("Покупка · %d/3", d.Step+1)
	text := ""
	if d.Step == 3 {
		title = "Проверьте покупки"
		text = fmt.Sprintf("%d из %d\n%s\nКоличество: %s\n%s", d.Page+1, len(d.Items), short(v.Name, 40), short(v.Quantity, 25), short(v.Note, 50))
		opts = append(opts, op("save"), "Сохранить", op("edit"), "Изменить позицию")
		if len(d.Items) > 1 {
			opts = append(opts, op("next_item"), "Следующая")
		}
	} else {
		prompts := []string{"Что купить? До 100 символов. Можно перечислить несколько позиций через запятую.", "Сколько? Например: 2 л, 3 штуки. Можно пропустить.", "Уточнение: бренд, размер, особенности. Можно пропустить."}
		values := []string{v.Name, v.Quantity, v.Note}
		text = prompts[d.Step]
		if values[d.Step] != "" {
			text += "\nСейчас: " + short(values[d.Step], 40)
			opts = append(opts, op("keep"), "Оставить")
		}
		if d.Step > 0 {
			opts = append(opts, op("skip"), "Пропустить")
		}
	}
	if hint != "" {
		text = short(hint, 45) + "\n" + text
	}
	opts = append(opts, op("back"), "Назад", op("cancel"), "Отмена")
	return b.card(uid, msg, title, text, d.Circle, opts...)
}
func (b *Bot) shoppingMessage(uid int64, text string) error {
	d, e := b.loadShopping(uid)
	if e != nil {
		return b.say(uid, "Черновик закрыт")
	}
	if d.Step == 3 {
		return b.saveShopping(uid, 0, d, "Нажмите «Сохранить» после проверки")
	}
	v := d.Items[d.Page]
	text = strings.TrimSpace(text)
	switch d.Step {
	case 0:
		if d.ItemID == 0 && len(d.Items) == 1 && (strings.Contains(text, ",") || strings.Contains(text, "\n")) {
			items := []shopping.Input{}
			for _, name := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' }) {
				item := shopping.Input{Name: strings.TrimSpace(name)}
				if e = shopping.Validate(item); e != nil {
					return b.saveShopping(uid, 0, d, e.Error())
				}
				items = append(items, item)
			}
			if len(items) < 1 || len(items) > 20 {
				return b.saveShopping(uid, 0, d, "От 1 до 20 позиций")
			}
			d.Items = items
			d.Step = 3
			d.Revision++
			return b.saveShopping(uid, 0, d, "")
		}
		v.Name = text
	case 1:
		v.Quantity = text
	case 2:
		v.Note = text
	}
	if e = shopping.Validate(v); e != nil {
		return b.saveShopping(uid, 0, d, e.Error())
	}
	d.Items[d.Page] = v
	d.Step++
	d.Revision++
	return b.saveShopping(uid, 0, d, "")
}
func (b *Bot) shoppingAction(uid, msg int64, a Action) (bool, error) {
	p := strings.Split(a.Kind, ":")
	id, version := 0, 0
	if len(p) > 1 {
		id, _ = strconv.Atoi(p[1])
	}
	if len(p) > 2 {
		version, _ = strconv.Atoi(p[2])
	}
	s := &shopping.Service{DB: b.DB}
	switch p[0] {
	case "shopping":
		return true, b.shopping(uid, msg, a.CircleID, id, version == 1)
	case "shop_new":
		return true, b.startShopping(uid, msg, a.CircleID, 0, 0, nil)
	case "shop_edit":
		return true, b.startShopping(uid, msg, a.CircleID, id, version, nil)
	case "shop_manage":
		h, e := b.Family.View(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		u, e := b.Family.Authorize(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		opts := []string{fmt.Sprintf("shop_details:%d", id), "Подробнее"}
		rows, e := s.List(uid, a.CircleID, false)
		if e != nil {
			return true, e
		}
		for _, r := range rows {
			if r.ID == id && (r.CreatedBy == u.ID || h.Owner) {
				opts = append(opts, fmt.Sprintf("shop_edit:%d:%d", id, r.Version), "Править", fmt.Sprintf("shop_archive:%d:%d", id, r.Version), "Убрать")
			}
		}
		opts = append(opts, "shop_new", "Добавить покупку", "shopping:0:0", "Назад")
		return true, b.card(uid, msg, "Параметры покупки", "Править позицию может её автор или владелец дома.", a.CircleID, opts...)
	case "shop_details":
		for _, bought := range []bool{false, true} {
			rows, e := s.List(uid, a.CircleID, bought)
			if e != nil {
				return true, e
			}
			for _, r := range rows {
				if r.ID == id {
					return true, b.say(uid, r.Name+"\n"+r.Quantity+"\n"+r.Note)
				}
			}
		}
		return true, b.say(uid, "Покупка недоступна")
	case "shop_archive":
		return true, b.card(uid, msg, "Убрать покупку?", "Запись сохранится в истории, баллы не изменятся.", a.CircleID, fmt.Sprintf("shop_change:%d:%d:archive", id, version), "Да, убрать", "shopping:0:0", "Назад")
	case "shop_change":
		if len(p) != 4 {
			return true, family.ErrDenied
		}
		if e := s.Change(uid, a.CircleID, id, version, p[3], nil, b.Now()); e != nil {
			return true, b.say(uid, "Покупка уже изменилась или действие недоступно. Откройте свежий список")
		}
		return true, b.shopping(uid, msg, a.CircleID, 0, p[3] == "buy")
	case "shop_flow":
		d, e := b.loadShopping(uid)
		if e != nil {
			return true, b.say(uid, "Черновик уже закрыт")
		}
		if len(p) != 4 || p[1] != d.Nonce || p[2] != strconv.Itoa(d.Revision) {
			return true, b.saveShopping(uid, msg, d, "Это старая кнопка")
		}
		switch p[3] {
		case "cancel":
			b.Family.SetStep(uid, "", "")
			return true, b.shopping(uid, msg, d.Circle, 0, false)
		case "back":
			if d.Step == 0 {
				b.Family.SetStep(uid, "", "")
				return true, b.shopping(uid, msg, d.Circle, 0, false)
			}
			d.Step--
		case "edit":
			d.Step = 0
		case "next_item":
			d.Page = (d.Page + 1) % len(d.Items)
		case "keep", "skip":
			if d.Step >= 3 {
				return true, family.ErrDenied
			}
			if p[3] == "skip" {
				if d.Step == 0 {
					return true, family.ErrDenied
				}
				if d.Step == 1 {
					d.Items[d.Page].Quantity = ""
				} else {
					d.Items[d.Page].Note = ""
				}
			}
			if e = shopping.Validate(d.Items[d.Page]); e != nil {
				return true, b.saveShopping(uid, msg, d, e.Error())
			}
			d.Step++
		case "save":
			if d.Step != 3 {
				return true, family.ErrDenied
			}
			if d.ItemID == 0 {
				e = s.CreateBatch(uid, d.Circle, d.Nonce, d.Items, b.Now())
			} else {
				e = s.Change(uid, d.Circle, d.ItemID, d.Version, "edit", &d.Items[0], b.Now())
			}
			if e != nil {
				return true, b.say(uid, "Покупка изменилась или поля некорректны. Откройте свежий список")
			}
			b.Family.SetStep(uid, "", "")
			return true, b.shopping(uid, msg, d.Circle, 0, false)
		default:
			return true, family.ErrDenied
		}
		d.Revision++
		return true, b.saveShopping(uid, msg, d, "")
	}
	return false, nil
}
