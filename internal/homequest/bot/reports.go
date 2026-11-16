package bot

import (
	"donetick.com/core/internal/homequest/task"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (b *Bot) submitted(uid, msg int64, circle int, a task.Attempt) error {
	switch a.Status {
	case "photo":
		if e := b.Family.SetStep(uid, "photo_attach", strconv.Itoa(a.ID)); e != nil {
			return e
		}
		return b.say(uid, "Пришлите фотографию отчёта обычным фото в этот чат. /cancel завершит диалог; отчёт останется в разделе «Отчёты».")
	case "review":
		if e := b.PlanReviews(); e != nil {
			return e
		}
		return b.card(uid, msg, "Ожидает подтверждения", "Баллы появятся после принятия другим участником.", circle, "reports", "Отчёты", "home", "Дом")
	case "accepted":
		if !a.Approval {
			if e := b.card(uid, 0, "Готово", "Баллы начислены. Ошибочное выполнение можно отменить в течение 5 минут.", circle, fmt.Sprintf("undo:%d", a.ID), "Отменить выполнение", "home", "Дом"); e != nil {
				return e
			}
		}
		return b.tasks(uid, msg, 0)
	default:
		return b.home(uid, msg)
	}
}
func (b *Bot) photo(uid int64, file string) error {
	id, e := b.Family.Step(uid)
	if e != nil {
		return e
	}
	home, e := b.Family.Current(uid)
	if e != nil {
		return b.home(uid, 0)
	}
	n, e := strconv.Atoi(id.Draft)
	if e != nil {
		return b.say(uid, "Сначала выберите задание или отчёт, к которому нужно прикрепить фото")
	}
	s := &task.Service{DB: b.DB}
	switch id.Step {
	case "photo_submit":
		a, e := s.Submit(uid, home.ID, n, file, b.Now())
		if e != nil {
			return b.say(uid, "Не удалось прикрепить фото к этому заданию")
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.submitted(uid, 0, home.ID, a)
	case "photo_attach":
		if e = s.Attach(uid, home.ID, n, file, b.Now()); e != nil {
			return b.say(uid, "Этот отчёт больше не ожидает фото")
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		var a task.Attempt
		if e = b.DB.First(&a, n).Error; e != nil {
			return e
		}
		return b.submitted(uid, 0, home.ID, a)
	default:
		return b.say(uid, "Выберите «Параметры → Выполнить с фото» в карточке задания")
	}
}
func (b *Bot) reportAction(uid, msg int64, a Action) (bool, error) {
	s := &task.Service{DB: b.DB}
	parts := strings.Split(a.Kind, ":")
	id := 0
	if len(parts) > 1 {
		n, e := strconv.Atoi(parts[1])
		if e != nil {
			return true, e
		}
		id = n
	}
	switch parts[0] {
	case "history":
		return true, b.history(uid, msg, a.CircleID, id)
	case "ranking":
		return true, b.ranking(uid, msg, a.CircleID, id)
	case "badges":
		return true, b.badges(uid, msg, a.CircleID)
	case "house":
		return true, b.house(uid, msg, a.CircleID)
	case "task_manage":
		return true, b.card(uid, msg, "Изменить задание", "Автор или владелец может изменить правила будущих выполнений.", a.CircleID, fmt.Sprintf("edit_task:%d", id), "Название и баллы", fmt.Sprintf("assign:%d:0", id), "Исполнитель", fmt.Sprintf("due:%d", id), "Срок", fmt.Sprintf("reviewers:%d:0", id), "Проверяющий", fmt.Sprintf("task_state:%d", id), "Пауза / отмена", "tasks", "Назад")
	case "task_state":
		return true, b.card(uid, msg, "Состояние задания", "Пауза остановит выполнение и напоминания.", a.CircleID, fmt.Sprintf("pause:%d", id), "Приостановить", fmt.Sprintf("resume:%d", id), "Продолжить", fmt.Sprintf("cancel_confirm:%d", id), "Отменить задание", "tasks", "Назад")
	case "pause", "resume":
		if e := s.Manage(uid, a.CircleID, id, parts[0], 0); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "cancel_confirm":
		return true, b.card(uid, msg, "Отменить задание?", "Общая история сохранится.", a.CircleID, fmt.Sprintf("cancel_task:%d", id), "Подтвердить отмену", "tasks", "Назад")
	case "cancel_task":
		if e := s.Manage(uid, a.CircleID, id, "cancel", 0); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "edit_task", "due":
		step := "task_edit"
		prompt := "Введите баллы и название: 10 | Вынести мусор | описание (необязательно). /cancel — отмена"
		if parts[0] == "due" {
			step = "task_due"
			prompt = "Введите срок в часовом поясе дома: 2026-11-20 19:00, либо без срока. /cancel — отмена"
		}
		if e := b.Family.SetStep(uid, step, strconv.Itoa(id)); e != nil {
			return true, e
		}
		return true, b.say(uid, prompt)

	case "reviewers":
		page := 0
		if len(parts) == 3 {
			page, _ = strconv.Atoi(parts[2])
		}
		return true, b.reviewers(uid, msg, a.CircleID, id, page)
	case "reviewer":
		if len(parts) != 3 {
			return true, b.home(uid, msg)
		}
		target, e := strconv.Atoi(parts[2])
		if e != nil {
			return true, e
		}
		if e = s.SetReviewer(uid, a.CircleID, id, target); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "assign":
		page := 0
		if len(parts) == 3 {
			page, _ = strconv.Atoi(parts[2])
		}
		return true, b.assignees(uid, msg, a.CircleID, id, page)
	case "assignee":
		if len(parts) != 3 {
			return true, b.home(uid, msg)
		}
		target, e := strconv.Atoi(parts[2])
		if e != nil {
			return true, e
		}
		if e = s.Manage(uid, a.CircleID, id, "assign", target); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "snooze":
		if e := b.Snooze(uid, uint(id), 30*time.Minute); e != nil {
			return true, b.say(uid, "Это напоминание уже неактуально")
		}
		return true, b.say(uid, "Напомню через 30 минут. Срок задания не изменён.")
	case "notifications":
		return true, b.notifications(uid, msg, a.CircleID)
	case "rest":
		if e := b.Family.SetStep(uid, "rest", ""); e != nil {
			return true, e
		}
		return true, b.say(uid, "Введите: вкл 22 8 — напоминания с тихими часами; выкл — отключить; отпуск 7 — пауза на 7 дней. /cancel — отмена")
	case "schedule":
		if e := b.Family.SetStep(uid, "schedule", strconv.Itoa(id)); e != nil {
			return true, e
		}
		return true, b.say(uid, "Введите расписание в часовом поясе дома:\nкаждый день 19:00\nвт,чт 19:00\nкаждые 3 дня 19:00\nпосле 3 дней — после принятия\nбез повтора\n/cancel — отмена")
	case "timezone":
		if e := b.Family.SetStep(uid, "change_timezone", ""); e != nil {
			return true, e
		}
		return true, b.say(uid, "Введите IANA timezone, например Europe/Berlin. /cancel — отмена.")
	case "member_page":
		return true, b.members(uid, msg, a.CircleID, id)
	case "member_manage":
		return true, b.card(uid, msg, "Участник", "Действия доступны только владельцу дома.", a.CircleID, fmt.Sprintf("transfer_confirm:%d", id), "Передать управление", fmt.Sprintf("remove_confirm:%d", id), "Исключить", "members", "Назад")
	case "transfer_confirm", "remove_confirm":
		return true, b.card(uid, msg, "Подтверждение", "Изменение прав вступит в силу сразу.", a.CircleID, strings.TrimSuffix(parts[0], "_confirm")+":"+strconv.Itoa(id), "Подтвердить", "members", "Отмена")
	case "transfer":
		if e := b.Family.Transfer(uid, id); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.home(uid, msg)
	case "remove":
		if e := b.Family.Remove(uid, id); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.members(uid, msg, a.CircleID, 0)
	case "task_details":
		text, e := s.Details(uid, a.CircleID, id)
		if e != nil {
			return true, b.say(uid, "Задание недоступно")
		}
		return true, b.say(uid, text)
	case "options":
		return true, b.card(uid, msg, "Параметры задания", "Правила может менять автор или владелец. Фото и подтверждение — независимые требования.", a.CircleID, fmt.Sprintf("rules:%d", id), "Правила", fmt.Sprintf("photo_submit:%d", id), "Выполнить с фото", fmt.Sprintf("schedule:%d", id), "Повторение", fmt.Sprintf("task_manage:%d", id), "Изменить", fmt.Sprintf("task_details:%d", id), "Описание", "tasks", "Назад")
	case "rules":
		return true, b.card(uid, msg, "Правила выполнения", "Выберите требования. Для подтверждения нужен ещё один участник.", a.CircleID, fmt.Sprintf("configure:%d:none", id), "Без требований", fmt.Sprintf("configure:%d:photo", id), "Только фото", fmt.Sprintf("configure:%d:approval", id), "Подтверждение", fmt.Sprintf("configure:%d:both", id), "Фото и подтверждение")
	case "configure":
		if len(parts) != 3 {
			return true, b.home(uid, msg)
		}
		if e := s.Configure(uid, a.CircleID, id, parts[2] == "approval" || parts[2] == "both", parts[2] == "photo" || parts[2] == "both"); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "photo_submit", "photo_attach":
		if e := b.Family.SetStep(uid, parts[0], strconv.Itoa(id)); e != nil {
			return true, e
		}
		return true, b.say(uid, "Пришлите фото обычным сообщением. /cancel — отмена диалога.")
	case "undo":
		if e := s.Undo(uid, a.CircleID, id, b.Now()); e != nil {
			return true, b.say(uid, e.Error())
		}
		return true, b.tasks(uid, msg, 0)
	case "approve", "reject":
		if e := s.Review(uid, a.CircleID, id, parts[0] == "approve", b.Now()); e != nil {
			return true, b.say(uid, "Этот отчёт уже обработан или недоступен для проверки")
		}
		return true, b.reports(uid, msg, a.CircleID)
	case "view_photo":
		file, e := s.Photo(uid, a.CircleID, id)
		if e != nil {
			return true, b.say(uid, "Фото недоступно")
		}
		return true, b.queue("sendPhoto", map[string]any{"chat_id": uid, "photo": file, "caption": "Фотоотчёт"})
	case "reports":
		return true, b.reports(uid, msg, a.CircleID)
	}
	return false, nil
}
func (b *Bot) reports(uid, msg int64, circle int) error {
	user, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	var rows []task.Attempt
	e = b.DB.Table("hq_attempts a").Select("a.*").Joins("JOIN hq_task_instances i ON i.id = a.instance_id").Where("i.circle_id = ? AND ((a.status = ? AND a.user_id = ?) OR (a.status = ? AND a.user_id != ?))", circle, "photo", user.ID, "review", user.ID).Order("a.id").Find(&rows).Error
	var a task.Attempt
	for _, row := range rows {
		if row.Status == "photo" || task.CanReview(b.DB, row, circle, user.ID) {
			a = row
			break
		}
	}
	if e != nil || a.ID == 0 {
		return b.card(uid, msg, "Отчёты", "Сейчас нет отчётов, требующих вашего действия.", circle, "home", "Дом")
	}
	if a.Status == "photo" {
		return b.card(uid, msg, "Ожидает фотографии", "Ваше выполнение ещё не отправлено. Прикрепите фото.", circle, fmt.Sprintf("photo_attach:%d", a.ID), "Добавить фото", "home", "Дом")
	}
	opts := []string{fmt.Sprintf("approve:%d", a.ID), "Принять", fmt.Sprintf("reject:%d", a.ID), "Отклонить"}
	if a.PhotoID != "" {
		opts = append(opts, fmt.Sprintf("view_photo:%d", a.ID), "Посмотреть фото")
	}
	opts = append(opts, "home", "Дом")
	var subject struct {
		Name        string
		DisplayName string
		Points      int
	}
	if e = b.DB.Table("chore_histories h").Select("chores.name, users.display_name, h.points").Joins("JOIN chores ON chores.id = h.chore_id").Joins("JOIN users ON users.id = h.completed_by").Where("h.id = ?", a.HistoryID).Scan(&subject).Error; e != nil {
		return e
	}
	name := []rune(subject.Name)
	if len(name) > 50 {
		name = name[:50]
	}
	person := []rune(subject.DisplayName)
	if len(person) > 24 {
		person = person[:24]
	}
	return b.card(uid, msg, string(name), fmt.Sprintf("%s · %d баллов\nОжидает вашего подтверждения", string(person), subject.Points), circle, opts...)
}

func (b *Bot) members(uid, msg int64, circle, page int) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	if len(h.Members) == 0 {
		return b.home(uid, msg)
	}
	if page < 0 || page >= len(h.Members) {
		page = 0
	}
	m := h.Members[page]
	name := []rune(m.Name)
	if len(name) > 40 {
		name = append(name[:39], '…')
	}
	role := "Участник"
	if m.Owner {
		role = "Владелец настроек"
	}
	opts := []string{fmt.Sprintf("member_page:%d", (page+1)%len(h.Members)), "Следующий", "home", "Дом"}
	if h.Owner {
		opts = append(opts, fmt.Sprintf("member_manage:%d", m.ID), "Управление")
	}
	return b.card(uid, msg, string(name), fmt.Sprintf("%s · %d из %d", role, page+1, len(h.Members)), circle, opts...)
}

func (b *Bot) reviewers(uid, msg int64, circle, id, page int) error {
	h, e := b.Family.View(uid, circle)
	if e != nil {
		return e
	}
	if page < 0 || page >= len(h.Members) {
		page = 0
	}
	m := h.Members[page]
	return b.card(uid, msg, "Кто проверяет?", short(m.Name, 40)+"\nЕсли проверяющий выйдет, принять сможет другой взрослый. Самопроверка запрещена.", circle, fmt.Sprintf("reviewer:%d:%d", id, m.ID), "Выбрать участника", fmt.Sprintf("reviewers:%d:%d", id, (page+1)%len(h.Members)), "Следующий", fmt.Sprintf("reviewer:%d:0", id), "Любой другой участник", "tasks", "Назад")
}
