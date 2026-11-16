package bot

import (
	"donetick.com/core/internal/homequest/task"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseSchedule(text string) (task.Schedule, error) {
	r := task.Schedule{}
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return r, errors.New("Пустое расписание")
	}
	if text == "без повтора" {
		r.Mode = "once"
		return r, nil
	}
	if len(words) == 3 && words[0] == "после" {
		n, e := strconv.Atoi(words[1])
		r.Mode = "after"
		r.Every = n
		return r, e
	}
	clock := strings.Split(words[len(words)-1], ":")
	if len(clock) != 2 {
		return r, errors.New("Время укажите как 19:00")
	}
	var e error
	r.Hour, e = strconv.Atoi(clock[0])
	if e != nil {
		return r, e
	}
	r.Minute, e = strconv.Atoi(clock[1])
	if e != nil {
		return r, e
	}
	switch {
	case len(words) == 3 && words[0] == "каждый" && words[1] == "день":
		r.Mode = "daily"
	case len(words) == 4 && words[0] == "каждые":
		r.Mode = "interval"
		r.Every, e = strconv.Atoi(words[1])
		if e != nil {
			return r, e
		}
	case len(words) == 2:
		r.Mode = "weekdays"
		days := map[string]string{"пн": "monday", "вт": "tuesday", "ср": "wednesday", "чт": "thursday", "пт": "friday", "сб": "saturday", "вс": "sunday"}
		for _, d := range strings.Split(words[0], ",") {
			if days[d] == "" {
				return r, errors.New("Дни: пн,вт,ср,чт,пт,сб,вс")
			}
			r.Days = append(r.Days, days[d])
		}
	default:
		return r, errors.New("Используйте один из примеров расписания")
	}
	return r, nil
}
func (b *Bot) scheduleMessage(uid int64, draft, text string) error {
	var id int
	if _, e := fmt.Sscan(draft, &id); e != nil {
		return e
	}
	h, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	rule, e := parseSchedule(text)
	if e != nil {
		return b.say(uid, e.Error())
	}
	if e = (&task.Service{DB: b.DB}).SetSchedule(uid, h.ID, id, rule, b.Now()); e != nil {
		return b.say(uid, e.Error())
	}
	if e = b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.tasks(uid, 0, 0)
}
func dueText(due *time.Time, zone string) string {
	if due == nil {
		return "Без срока"
	}
	loc, e := time.LoadLocation(zone)
	if e != nil {
		return "Срок недоступен"
	}
	return due.In(loc).Format("02.01 15:04") + " · " + zone
}

func (b *Bot) editTaskMessage(uid int64, step, draft, text string) error {
	id, e := strconv.Atoi(draft)
	if e != nil {
		return e
	}
	h, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	s := &task.Service{DB: b.DB}
	if step == "task_due" {
		e = s.SetDue(uid, h.ID, id, text)
	} else {
		parts := strings.SplitN(text, "|", 3)
		if len(parts) < 2 {
			return b.say(uid, "Пример: 10 | Вынести мусор")
		}
		points, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return b.say(uid, "Баллы должны быть целым числом")
		}
		if len(parts) == 3 && len([]rune(parts[2])) > 1000 {
			return b.say(uid, "Описание до 1000 символов")
		}
		e = s.Edit(uid, h.ID, id, strings.TrimSpace(parts[1]), points)
		if e == nil && len(parts) == 3 {
			e = s.SetDescription(uid, h.ID, id, strings.TrimSpace(parts[2]))
		}
	}
	if e != nil {
		return b.say(uid, e.Error())
	}
	if e = b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.tasks(uid, 0, 0)
}
func (b *Bot) assignees(uid, msg int64, circle, id, page int) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	if page < 0 || page >= len(h.Members) {
		page = 0
	}
	m := h.Members[page]
	name := []rune(m.Name)
	if len(name) > 40 {
		name = name[:40]
	}
	return b.card(uid, msg, "Исполнитель", string(name), circle, fmt.Sprintf("assignee:%d:%d", id, m.ID), "Назначить", fmt.Sprintf("assign:%d:%d", id, (page+1)%len(h.Members)), "Следующий", fmt.Sprintf("assignee:%d:0", id), "Может выполнить любой", "tasks", "Назад")
}

func scheduleText(s task.Schedule) string {
	clock := fmt.Sprintf(" · %02d:%02d", s.Hour, s.Minute)
	switch s.Mode {
	case "daily":
		return "Каждый день" + clock
	case "weekdays":
		names := map[string]string{"monday": "пн", "tuesday": "вт", "wednesday": "ср", "thursday": "чт", "friday": "пт", "saturday": "сб", "sunday": "вс"}
		days := make([]string, 0, len(s.Days))
		for _, day := range s.Days {
			days = append(days, names[day])
		}
		return "По дням: " + strings.Join(days, ", ") + clock
	case "interval":
		return fmt.Sprintf("Каждые %d дн.", s.Every) + clock
	case "after":
		return fmt.Sprintf("Через %d дн. после принятия", s.Every)
	default:
		return "Без повторения"
	}
}
