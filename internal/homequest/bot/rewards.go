package bot

import (
	"donetick.com/core/internal/homequest/reward"
	"fmt"
	"strconv"
	"strings"
)

func (b *Bot) rewards(uid, msg int64, circle, page int) error {
	s := &reward.Service{DB: b.DB}
	rows, e := s.List(uid, circle)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Семейные награды", "Договоритесь о приятных наградах. Здесь нет денежных платежей.", circle, "reward_new", "Добавить награду", "redemptions:0", "Заявки", "points", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	name := []rune(r.Name)
	if len(name) > 50 {
		name = name[:50]
	}
	description := []rune(r.Description)
	if len(description) > 70 {
		description = append(description[:69], '…')
	}
	return b.card(uid, msg, string(name), fmt.Sprintf("%d баллов\n%s", r.PointsCost, string(description)), circle, fmt.Sprintf("reward_request:%d", r.ID), "Запросить", fmt.Sprintf("rewards:%d", (page+1)%len(rows)), "Следующая", fmt.Sprintf("reward_manage:%d", r.ID), "Каталог", "redemptions:0", "Заявки", "points", "Назад")
}
func (b *Bot) rewardMessage(uid int64, step, draft, text string) error {
	home, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	parts := strings.SplitN(text, "|", 3)
	if len(parts) < 2 {
		return b.say(uid, "Пример: Выбрать фильм | 10 | Для семейного вечера")
	}
	cost, e := strconv.Atoi(strings.TrimSpace(parts[1]))
	if e != nil {
		return b.say(uid, "Цена должна быть целым числом")
	}
	name := strings.TrimSpace(parts[0])
	s := &reward.Service{DB: b.DB}
	if step == "reward_new" {
		description := ""
		if len(parts) == 3 {
			description = strings.TrimSpace(parts[2])
		}
		_, e = s.Create(uid, home.ID, name, description, cost)
	} else {
		id, err := strconv.Atoi(draft)
		if err != nil {
			return err
		}
		if len(parts) == 3 {
			e = s.Edit(uid, home.ID, id, name, cost, strings.TrimSpace(parts[2]))
		} else {
			e = s.Edit(uid, home.ID, id, name, cost)
		}
	}
	if e != nil {
		return b.say(uid, e.Error())
	}
	if e = b.Family.SetStep(uid, "", ""); e != nil {
		return e
	}
	return b.rewards(uid, 0, home.ID, 0)
}
func (b *Bot) rewardAction(uid, msg int64, a Action) (bool, error) {
	parts := strings.Split(a.Kind, ":")
	id := 0
	if len(parts) > 1 {
		id, _ = strconv.Atoi(parts[1])
	}
	s := &reward.Service{DB: b.DB}
	switch parts[0] {
	case "rewards":
		return true, b.rewards(uid, msg, a.CircleID, id)
	case "reward_new", "reward_edit":
		if e := b.Family.SetStep(uid, parts[0], strconv.Itoa(id)); e != nil {
			return true, e
		}
		return true, b.say(uid, "Введите: название | цена в баллах | описание. /cancel — отмена")
	case "reward_request":
		if _, e := s.Request(uid, a.CircleID, id, a.Token, b.Now()); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.redemptions(uid, msg, a.CircleID, 0)
	case "reward_manage":
		return true, b.card(uid, msg, "Каталог наград", "Изменение каталога не меняет уже созданные заявки.", a.CircleID, "reward_new", "Добавить", fmt.Sprintf("reward_edit:%d", id), "Изменить", fmt.Sprintf("reward_archive:%d", id), "Убрать из каталога", "rewards:0", "Назад")
	case "reward_archive":
		if e := s.Archive(uid, a.CircleID, id); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.rewards(uid, msg, a.CircleID, 0)
	case "redemptions":
		return true, b.redemptions(uid, msg, a.CircleID, id)
	case "redemption":
		if len(parts) != 3 {
			return true, b.home(uid, msg)
		}
		if e := s.Resolve(uid, a.CircleID, id, parts[2], b.Now()); e != nil {
			return true, b.say(uid, "Эта заявка уже изменена или действие вам недоступно")
		}
		return true, b.redemptions(uid, msg, a.CircleID, 0)
	}
	return false, nil
}
func (b *Bot) redemptions(uid, msg int64, circle, page int) error {
	s := &reward.Service{DB: b.DB}
	rows, e := s.Requests(uid, circle)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "Заявки на награды", "Пока никто не запросил награду.", circle, "rewards:0", "Каталог", "points", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	user, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	status := map[int]string{0: "Ожидает принятия", 1: "Принята, ожидает выдачи", 2: "Отказ · баллы возвращены", 3: "Выдана", 4: "Отменена · баллы возвращены"}[int(r.Status)]
	opts := []string{}
	if user.ID == r.UserID && (r.Status == 0 || r.Status == 1) {
		opts = append(opts, fmt.Sprintf("redemption:%d:cancel", r.ID), "Отменить запрос")
	}
	if user.ID != r.UserID {
		if r.Status == 0 {
			opts = append(opts, fmt.Sprintf("redemption:%d:approve", r.ID), "Принять")
		}
		if r.Status == 1 {
			opts = append(opts, fmt.Sprintf("redemption:%d:deliver", r.ID), "Отметить выдачу")
		}
		if r.Status == 0 || r.Status == 1 {
			opts = append(opts, fmt.Sprintf("redemption:%d:reject", r.ID), "Отказать")
		}
	}
	opts = append(opts, fmt.Sprintf("redemptions:%d", (page+1)%len(rows)), "Следующая", "rewards:0", "Каталог")
	name := []rune(r.RewardName)
	if len(name) > 50 {
		name = name[:50]
	}
	return b.card(uid, msg, string(name), fmt.Sprintf("%s\n%d баллов списано при запросе", status, r.PointsCost), circle, opts...)
}
