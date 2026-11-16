package bot

import (
	"crypto/rand"
	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	u "donetick.com/core/internal/user/model"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
)

type organizationDraft struct {
	Nonce, Kind, Key, Field                   string
	Circle, Instance, Version, Page, Revision int
	Choices                                   []task.PresetTask
	Members                                   []int
}

func (b *Bot) organization(uid, msg int64, circle int) error {
	return b.card(uid, msg, "Домашние планы", "Наборы дел помогут начать. Комнаты собирают связанные задачи.", circle, "rooms:0", "Комнаты", "presets", "Готовые наборы", "handoffs:0", "Передачи дел", "weekly:0", "Итог недели", "shopping:0:0", "Покупки", "agent", "Помощник", "home", "Дом")
}
func (b *Bot) rooms(uid, msg int64, circle, page int) error {
	rows, e := (&task.Service{DB: b.DB}).Rooms(uid, circle)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Комнаты", "Добавьте свою комнату или выберите готовый набор дел.", circle, "room_new", "Добавить комнату", "presets", "Готовые наборы", "household", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	return b.card(uid, msg, r.Name, "Комната объединяет задания. Удаление комнаты сохранит сами дела.", circle, fmt.Sprintf("room_tasks:%d:0", r.ID), "Дела комнаты", fmt.Sprintf("rooms:%d", (page+1)%len(rows)), "Следующая комната", "room_new", "Добавить", fmt.Sprintf("room_rename:%d", r.ID), "Переименовать", fmt.Sprintf("room_delete:%d", r.ID), "Убрать комнату", "household", "Назад")
}
func (b *Bot) roomTasks(uid, msg int64, circle, room, page int) error {
	c, e := (&agent.Service{DB: b.DB}).SelectRoom(uid, room, b.Now())
	if e != nil {
		return b.say(uid, "Комната больше недоступна")
	}
	rows, e := (&task.Service{DB: b.DB}).RoomTasks(uid, circle, room)
	if e != nil {
		return b.say(uid, "Комната больше недоступна")
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Дела комнаты", c.Label()+"\nПока нет открытых дел.", circle, "rooms:0", "К комнатам", "new_task", "Добавить задание")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	v := rows[page]
	return b.card(uid, msg, short(v.Name, 45), c.Label()+"\n"+fmt.Sprintf("%d баллов · %d из %d", v.Points, page+1, len(rows)), circle, fmt.Sprintf("done:%d", v.ID), "Выполнено", fmt.Sprintf("options:%d", v.ID), "Параметры", fmt.Sprintf("room_tasks:%d:%d", room, (page+1)%len(rows)), "Следующее", "rooms:0", "К комнатам")
}
func (b *Bot) saveOrganization(uid, msg int64, d organizationDraft) error {
	raw, _ := json.Marshal(d)
	if e := b.Family.SetStep(uid, "organization", string(raw)); e != nil {
		return e
	}
	op := func(name string) string { return fmt.Sprintf("org:%s:%d:%s", d.Nonce, d.Revision, name) }
	if d.Kind == "preset" {
		v := d.Choices[d.Page]
		included := "Включено"
		if !v.Enabled {
			included = "Пропущено"
		}
		return b.card(uid, msg, "Набор: "+task.PresetNames[d.Key], fmt.Sprintf("%d/3 · %s\n%s\n%d баллов, каждые %d дн. в %02d:%02d", d.Page+1, included, short(v.Name, 40), v.Points, v.Every, v.Hour, v.Minute), d.Circle, op("toggle"), "Включить / пропустить", op("edit"), "Изменить дело", op("next"), "Следующее", op("save"), "Добавить набор", op("cancel"), "Отмена")
	}
	h, e := b.Family.View(uid, d.Circle)
	if e != nil {
		return e
	}
	if d.Page >= len(h.Members) {
		d.Page = 0
	}
	m := h.Members[d.Page]
	selected := false
	for _, id := range d.Members {
		if id == m.ID {
			selected = true
		}
	}
	mark := "Не в очереди"
	if selected {
		mark = "В очереди"
	}
	return b.card(uid, msg, "Чередование дел", short(m.Name, 35)+"\n"+mark+fmt.Sprintf(" · выбрано %d\nОчередь меняется после принятия выполнения. Отпуск выключает только уведомления.", len(d.Members)), d.Circle, op("toggle"), "Включить / убрать", op("next"), "Следующий участник", op("save"), "Сохранить очередь", op("clear"), "Общий пул", op("cancel"), "Отмена")
}
func (b *Bot) startOrganization(uid, msg int64, circle, id int, kind, key string) error {
	d := organizationDraft{Circle: circle, Instance: id, Kind: kind, Key: key, Revision: 1}
	buf := make([]byte, 8)
	if _, e := rand.Read(buf); e != nil {
		return e
	}
	d.Nonce = hex.EncodeToString(buf)
	if kind == "preset" {
		d.Choices = task.PresetTasks(key)
		if len(d.Choices) == 0 {
			return family.ErrDenied
		}
	} else {
		var i task.Instance
		if b.DB.First(&i, "id=? AND circle_id=?", id, circle).Error != nil {
			return family.ErrDenied
		}
		d.Version = i.Version
		var e error
		d.Members, e = (&task.Service{DB: b.DB}).Rotation(uid, circle, id)
		if e != nil {
			return e
		}
	}
	return b.saveOrganization(uid, msg, d)
}
func (b *Bot) loadOrganization(uid int64) (organizationDraft, error) {
	var d organizationDraft
	step, e := b.Family.Step(uid)
	if e != nil {
		return d, e
	}
	if step.Step != "organization" || json.Unmarshal([]byte(step.Draft), &d) != nil || d.Nonce == "" {
		return d, family.ErrDenied
	}
	h, e := b.Family.Current(uid)
	if e != nil || h.ID != d.Circle {
		return d, family.ErrDenied
	}
	return d, nil
}
func (b *Bot) organizationMessage(uid int64, text string) error {
	d, e := b.loadOrganization(uid)
	if e != nil {
		return b.say(uid, "Черновик закрыт")
	}
	if d.Kind != "preset" || d.Field != "edit" {
		return b.saveOrganization(uid, 0, d)
	}
	parts := strings.SplitN(text, "|", 4)
	var points, every int
	var hour, minute int
	if len(parts) != 4 {
		return b.say(uid, "Пример: Протереть стол | 5 | 2 | 19:00")
	}
	points, e = strconv.Atoi(strings.TrimSpace(parts[1]))
	if e != nil {
		return b.say(uid, "Баллы должны быть целыми")
	}
	every, e = strconv.Atoi(strings.TrimSpace(parts[2]))
	if e != nil {
		return b.say(uid, "Интервал должен быть целым")
	}
	_, e = fmt.Sscanf(strings.TrimSpace(parts[3]), "%d:%d", &hour, &minute)
	name := strings.TrimSpace(parts[0])
	if e != nil || points < 1 || points > 10000 || every < 1 || every > 365 || hour < 0 || hour > 23 || minute < 0 || minute > 59 || name == "" || len([]rune(name)) > 100 {
		return b.say(uid, "Проверьте название, баллы 1–10000, дни 1–365 и время")
	}
	v := &d.Choices[d.Page]
	v.Name = name
	v.Points = points
	v.Every = every
	v.Hour = hour
	v.Minute = minute
	d.Field = ""
	d.Revision++
	return b.saveOrganization(uid, 0, d)
}
func (b *Bot) organizationAction(uid, msg int64, a Action) (bool, error) {
	p := strings.Split(a.Kind, ":")
	s := &task.Service{DB: b.DB}
	id, page := 0, 0
	if len(p) > 1 {
		id, _ = strconv.Atoi(p[1])
	}
	if len(p) > 2 {
		page, _ = strconv.Atoi(p[2])
	}
	switch p[0] {
	case "household":
		return true, b.organization(uid, msg, a.CircleID)
	case "rooms":
		return true, b.rooms(uid, msg, a.CircleID, id)
	case "presets":
		return true, b.card(uid, msg, "Готовые наборы", "Проверьте состав, измените баллы и расписание перед добавлением. Повторное добавление набора не создаёт дубли.", a.CircleID, "preset:kitchen", "Кухня", "preset:bathroom", "Ванная", "preset:daily", "Повседневные дела", "household", "Назад")
	case "preset":
		if len(p) != 2 {
			return true, family.ErrDenied
		}
		return true, b.startOrganization(uid, msg, a.CircleID, 0, "preset", p[1])
	case "room_tasks":
		return true, b.roomTasks(uid, msg, a.CircleID, id, page)
	case "room_new", "room_rename":
		if e := b.Family.SetStep(uid, p[0], fmt.Sprintf("%d:%d", a.CircleID, id)); e != nil {
			return true, e
		}
		return true, b.say(uid, "Как назвать комнату? До 40 символов. /cancel — отмена")
	case "room_delete":
		return true, b.card(uid, msg, "Убрать комнату?", "Задания и их история сохранятся, исчезнет только группировка.", a.CircleID, fmt.Sprintf("room_delete_yes:%d", id), "Убрать комнату", "rooms:0", "Отмена")
	case "room_delete_yes":
		if e := s.DeleteRoom(uid, a.CircleID, id); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.rooms(uid, msg, a.CircleID, 0)
	case "organize":
		return true, b.card(uid, msg, "Комната и очередь", "Здесь можно сгруппировать дело, настроить очередь или предложить передачу назначенного вам дела.", a.CircleID, fmt.Sprintf("pick_room:%d:0", id), "Выбрать комнату", fmt.Sprintf("rotation:%d", id), "Чередование", fmt.Sprintf("offer_to:%d:0", id), "Предложить передачу", "handoffs:0", "Мои передачи", "tasks", "Назад")
	case "rotation":
		return true, b.startOrganization(uid, msg, a.CircleID, id, "rotation", "")
	case "pick_room":
		rows, e := s.Rooms(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		if len(rows) == 0 {
			return true, b.rooms(uid, msg, a.CircleID, 0)
		}
		if page < 0 || page >= len(rows) {
			page = 0
		}
		r := rows[page]
		return true, b.card(uid, msg, "Комната задания", r.Name, a.CircleID, fmt.Sprintf("set_room:%d:%d", id, r.ID), "Выбрать", fmt.Sprintf("pick_room:%d:%d", id, (page+1)%len(rows)), "Следующая", fmt.Sprintf("set_room:%d:0", id), "Без комнаты", "tasks", "Назад")
	case "set_room":
		if e := s.SetRoom(uid, a.CircleID, id, page); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "offer_to":
		h, e := b.Family.View(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		u, e := b.Family.Authorize(uid, a.CircleID)
		if e != nil {
			return true, e
		}
		members := []family.Member{}
		for _, m := range h.Members {
			if m.ID != u.ID {
				members = append(members, m)
			}
		}
		if len(members) == 0 {
			return true, b.say(uid, "Для передачи нужен другой участник")
		}
		if page >= len(members) || page < 0 {
			page = 0
		}
		m := members[page]
		return true, b.card(uid, msg, "Предложить передачу", short(m.Name, 40)+"\nЗадание останется вашим, пока участник не согласится.", a.CircleID, fmt.Sprintf("offer_handoff:%d:%d", id, m.ID), "Предложить", fmt.Sprintf("offer_to:%d:%d", id, (page+1)%len(members)), "Следующий", "tasks", "Назад")
	case "offer_handoff":
		offer, err := s.OfferHandoff(uid, a.CircleID, id, page, b.Now())
		if err != nil {
			return true, b.say(uid, "Передать можно только назначенное вам открытое дело")
		}
		if err = b.notifyHandoff(offer); err != nil {
			return true, err
		}
		return true, b.handoffs(uid, msg, a.CircleID, 0)
	case "handoffs":
		return true, b.handoffs(uid, msg, a.CircleID, id)
	case "handoff":
		if len(p) != 3 {
			return true, family.ErrDenied
		}
		if e := s.ResolveHandoff(uid, a.CircleID, id, p[2], b.Now()); e != nil {
			return true, b.say(uid, "Предложение устарело или недоступно")
		}
		return true, b.handoffs(uid, msg, a.CircleID, 0)
	case "org":
		d, e := b.loadOrganization(uid)
		if e != nil {
			return true, b.say(uid, "Черновик уже закрыт")
		}
		if len(p) != 4 || p[1] != d.Nonce || p[2] != strconv.Itoa(d.Revision) {
			return true, b.saveOrganization(uid, msg, d)
		}
		op := p[3]
		if op == "cancel" {
			b.Family.SetStep(uid, "", "")
			return true, b.organization(uid, msg, a.CircleID)
		}
		if d.Kind == "preset" {
			switch op {
			case "next":
				d.Page = (d.Page + 1) % len(d.Choices)
			case "toggle":
				d.Choices[d.Page].Enabled = !d.Choices[d.Page].Enabled
			case "edit":
				d.Field = "edit"
				d.Revision++
				if e = b.saveOrganization(uid, msg, d); e != nil {
					return true, e
				}
				return true, b.say(uid, "Введите: название | баллы | каждые N дней | время. Пример: Протереть стол | 5 | 2 | 19:00")
			case "save":
				if _, e = s.ApplyPreset(uid, d.Circle, d.Key, d.Choices, b.Now()); e != nil {
					return true, b.say(uid, e.Error())
				}
				b.Family.SetStep(uid, "", "")
				return true, b.rooms(uid, msg, d.Circle, 0)
			default:
				return true, family.ErrDenied
			}
		} else {
			h, e := b.Family.View(uid, d.Circle)
			if e != nil {
				return true, e
			}
			if d.Page >= len(h.Members) {
				d.Page = 0
			}
			switch op {
			case "next":
				d.Page = (d.Page + 1) % len(h.Members)
			case "toggle":
				target := h.Members[d.Page].ID
				found := false
				next := []int{}
				for _, m := range d.Members {
					if m == target {
						found = true
					} else {
						next = append(next, m)
					}
				}
				if !found {
					next = append(next, target)
				}
				d.Members = next
			case "clear":
				d.Members = nil
			case "save":
				var i task.Instance
				if b.DB.First(&i, "id=? AND circle_id=? AND version=?", d.Instance, d.Circle, d.Version).Error != nil {
					return true, b.say(uid, "Задание изменилось; начните настройку заново")
				}
				if e = s.SetRotation(uid, d.Circle, d.Instance, d.Members); e != nil {
					return true, b.say(uid, e.Error())
				}
				b.Family.SetStep(uid, "", "")
				return true, b.tasks(uid, msg, 0)
			default:
				return true, family.ErrDenied
			}
		}
		d.Revision++
		return true, b.saveOrganization(uid, msg, d)
	}
	return false, nil
}
func (b *Bot) handoffs(uid, msg int64, circle, page int) error {
	rows, e := (&task.Service{DB: b.DB}).Handoffs(uid, circle, b.Now())
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Передачи дел", "Нет предложений, ожидающих ответа.", circle, "household", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	u, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	var name string
	b.DB.Table("chores c").Select("c.name").Joins("JOIN hq_task_instances i ON i.chore_id=c.id").Where("i.id=? AND i.circle_id=?", r.InstanceID, circle).Scan(&name)
	opts := []string{}
	if u.ID == r.ToID {
		opts = append(opts, fmt.Sprintf("handoff:%d:accept", r.ID), "Принять дело", fmt.Sprintf("handoff:%d:decline", r.ID), "Отказаться")
	} else {
		opts = append(opts, fmt.Sprintf("handoff:%d:cancel", r.ID), "Отозвать предложение")
	}
	opts = append(opts, fmt.Sprintf("handoffs:%d", (page+1)%len(rows)), "Следующее", "household", "Назад")
	return b.card(uid, msg, "Передача дела", short(name, 50)+"\nОжидает согласия. Срок и баллы сохраняются.", circle, opts...)
}

func (b *Bot) notifyHandoff(h task.Handoff) error {
	var recipient family.Identity
	if b.DB.First(&recipient, "user_id=?", h.ToID).Error != nil {
		return nil
	}
	home, e := b.Family.View(recipient.TelegramID, h.CircleID)
	if e != nil {
		return e
	}
	button, e := b.button(recipient.TelegramID, h.CircleID, "handoffs:0", "Посмотреть предложение")
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(map[string]any{"chat_id": recipient.TelegramID, "text": "В доме «" + short(home.Name, 40) + "» вам предлагают взять дело. Оно станет вашим только после согласия.", "reply_markup": map[string]any{"inline_keyboard": [][]map[string]string{{{"text": button.Label, "callback_data": button.Action}}}}})
	key := fmt.Sprintf("handoff:%d", h.ID)
	return b.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&Delivery{Viewer: recipient.TelegramID, CircleID: h.CircleID, HandoffID: h.ID, EventKey: &key, Method: "sendMessage", Payload: string(raw), Notice: true, EventType: string(u.NotificationEventOnAssignment), Next: b.Now(), ScheduledFor: b.Now()}).Error
}
