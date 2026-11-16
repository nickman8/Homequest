// Package bot handles trusted Telegram updates with durable deduplication/outbox.
package bot

import (
	"crypto/rand"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/billing"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"donetick.com/core/internal/homequest/widget"
	"donetick.com/core/internal/homequest/wishlist"
	u "donetick.com/core/internal/user/model"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

type Person struct {
	ID   int64  `json:"id"`
	Name string `json:"first_name"`
}
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	Size     int    `json:"file_size"`
}
type Message struct {
	Voice  *Voice `json:"voice,omitempty"`
	From   Person `json:"from"`
	Chat   Chat   `json:"chat"`
	Text   string `json:"text"`
	Photos []struct {
		FileID string `json:"file_id"`
	} `json:"photo"`
}
type Query struct {
	ID        string `json:"id"`
	From      Person `json:"from"`
	Chat      Chat   `json:"chat"`
	MessageID int64  `json:"message_id"`
	Data      string `json:"data"`
}
type Callback struct {
	ID      string  `json:"id"`
	From    Person  `json:"from"`
	Message Message `json:"message"`
	Data    string  `json:"data"`
}
type Update struct {
	ID       int64     `json:"update_id"`
	Message  *Message  `json:"message,omitempty"`
	Query    *Query    `json:"widget_query,omitempty"`
	Callback *Callback `json:"callback_query,omitempty"`
}
type Cursor struct {
	ID   int `gorm:"primaryKey;autoIncrement:false"`
	Next int64
}

func (Cursor) TableName() string { return "hq_polling_cursor" }

type Receipt struct {
	ID int64 `gorm:"primaryKey;autoIncrement:false"`
}

func (Receipt) TableName() string { return "hq_update_receipts" }

type Delivery struct {
	AttemptID    int
	EventKey     *string `gorm:"uniqueIndex"`
	InstanceID   int
	Version      int
	Notice       bool
	EventType    string
	ScheduledFor time.Time
	CircleID     int
	Viewer       int64
	ID           uint `gorm:"primaryKey"`
	Method       string
	Payload      string
	Attempts     int
	Sent         bool
	Next         time.Time
	CreatedAt    time.Time
}

func (Delivery) TableName() string { return "hq_deliveries" }

type Action struct {
	InstanceID int
	Version    int
	Token      string `gorm:"primaryKey"`
	Viewer     int64  `gorm:"index"`
	CircleID   int
	Kind       string
	Expires    time.Time
	Used       bool
}

func (Action) TableName() string { return "hq_actions" }

type Button struct {
	Label  string `json:"label"`
	Action string `json:"action"`
}
type Card struct {
	Title   string   `json:"title"`
	Text    string   `json:"text"`
	Buttons []Button `json:"buttons"`
}
type Bot struct {
	LLM             assistant.LLM
	STT             assistant.STT
	VoiceSource     VoiceSource
	AIDailyLimit    int
	AIPremiumLimit  int
	BillingMode     string
	ReminderOffsets []int
	DB              *gorm.DB
	Family          *family.Service
	HTML            string
	Username        string
	Now             func() time.Time
}

func New(db *gorm.DB, html, username string) (*Bot, error) {
	f, e := family.New(db)
	if e != nil {
		return nil, e
	}
	if _, e = task.New(db); e != nil {
		return nil, e
	}
	if _, e = stats.New(db); e != nil {
		return nil, e
	}
	if _, e = reward.New(db); e != nil {
		return nil, e
	}
	if _, e = wishlist.New(db); e != nil {
		return nil, e
	}
	if _, e = billing.New(db, ""); e != nil {
		return nil, e
	}
	e = db.AutoMigrate(&Receipt{}, &Cursor{}, &Delivery{}, &Action{}, &u.UserNotificationPreference{}, &RestPreference{}, &AIJob{}, &AIConsent{})
	return &Bot{DB: db, Family: f, HTML: html, Username: username, Now: func() time.Time { return time.Now().UTC() }, ReminderOffsets: []int{-60, 0, 120}}, e
}
func (b *Bot) Handle(update Update) error {
	return b.DB.Transaction(func(tx *gorm.DB) error {
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Receipt{ID: update.ID})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		if e := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Cursor{ID: 1, Next: update.ID + 1}).Error; e != nil {
			return e
		}
		child := *b
		child.DB = tx
		child.Family = &family.Service{DB: tx}
		if q := update.Query; q != nil {
			if q.Chat.Type != "private" || q.Chat.ID != q.From.ID {
				return nil
			}
			if e := child.action(q.From.ID, q.Data, q.MessageID); e != nil {
				return e
			}
			return child.awardBadges(q.From.ID)
		}
		if q := update.Callback; q != nil {
			if q.Message.Chat.Type != "private" || q.Message.Chat.ID != q.From.ID {
				return nil
			}
			if e := child.action(q.From.ID, q.Data, 0); e != nil {
				return e
			}
			return child.awardBadges(q.From.ID)
		}
		if m := update.Message; m != nil {
			if m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
				return nil
			}
			if e := child.Family.Start(m.From.ID, m.From.Name); e != nil {
				return e
			}
			if m.Voice != nil {
				if m.Voice.Duration > 60 || m.Voice.Size > 4<<20 {
					return child.say(m.From.ID, "Пришлите голосовое до 60 секунд и 4 МБ.")
				}
				return child.enqueueAI(m.From.ID, "", m.Voice.FileID)
			}
			if len(m.Photos) > 0 {
				if e := child.photo(m.From.ID, m.Photos[len(m.Photos)-1].FileID); e != nil {
					return e
				}
				return child.awardBadges(m.From.ID)
			}
			if e := child.message(m.From.ID, m.Text); e != nil {
				return e
			}
			return child.awardBadges(m.From.ID)
		}
		return nil
	})
}
func (b *Bot) queue(method string, p any) error {
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	d := Delivery{Method: method, Payload: string(raw), Next: b.Now()}
	if p, ok := p.(map[string]any); ok {
		if uid, ok := p["chat_id"].(int64); ok {
			d.Viewer = uid
			if h, e := b.Family.Current(uid); e == nil {
				d.CircleID = h.ID
			}
		}
	}
	return b.DB.Create(&d).Error
}
func (b *Bot) say(uid int64, text string) error {
	payload := map[string]any{"chat_id": uid, "text": text}
	if state, e := b.Family.Step(uid); e == nil && state.Step != "" {
		circle := 0
		if h, e := b.Family.Current(uid); e == nil {
			circle = h.ID
		}
		back, e := b.button(uid, circle, "dialog_back", "Назад")
		if e != nil {
			return e
		}
		cancel, e := b.button(uid, circle, "dialog_cancel", "Отмена")
		if e != nil {
			return e
		}
		payload["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{{{"text": back.Label, "callback_data": back.Action}, {"text": cancel.Label, "callback_data": cancel.Action}}}}
	}
	return b.queue("sendMessage", payload)
}
func (b *Bot) button(uid int64, circle int, kind, label string) (Button, error) {
	buf := make([]byte, 16)
	if _, e := rand.Read(buf); e != nil {
		return Button{}, e
	}
	a := Action{Token: hex.EncodeToString(buf), Viewer: uid, CircleID: circle, Kind: kind, Expires: b.Now().Add(24 * time.Hour)}
	parts := strings.Split(kind, ":")
	if len(parts) > 1 {
		switch parts[0] {
		case "reviewer", "reviewers", "done", "photo_submit", "options", "rules", "configure", "schedule", "task_manage", "pause", "resume", "cancel_task", "cancel_confirm", "edit_task", "due", "assign", "assignee":
			id, e := strconv.Atoi(parts[1])
			if e != nil {
				return Button{}, e
			}
			var instance task.Instance
			if e = b.DB.First(&instance, "id = ? AND circle_id = ?", id, circle).Error; e != nil {
				return Button{}, e
			}
			a.InstanceID = id
			a.Version = instance.Version
		}
	}
	e := b.DB.Create(&a).Error
	return Button{label, a.Token}, e
}
func (b *Bot) card(uid, msg int64, title, text string, circle int, options ...string) error {
	state := Card{Title: title, Text: text, Buttons: []Button{}}
	for i := 0; i < len(options); i += 2 {
		button, e := b.button(uid, circle, options[i], options[i+1])
		if e != nil {
			return e
		}
		state.Buttons = append(state.Buttons, button)
	}
	raw, e := json.Marshal(state)
	if e != nil {
		return e
	}
	if e = widget.Validate([]byte(b.HTML), string(raw), ""); e != nil {
		return e
	}
	if msg != 0 {
		return b.queue("editMessageWidgetData", map[string]any{"chat_id": uid, "message_id": msg, "chat_data": string(raw)})
	}
	keyboard := [][]map[string]string{}
	for _, button := range state.Buttons {
		keyboard = append(keyboard, []map[string]string{{"text": button.Label, "callback_data": button.Action}})
	}
	return b.queue("sendWidget", map[string]any{"chat_id": uid, "html": b.HTML, "chat_data": string(raw), "allow_queries": true, "caption": "HomeQuest · Если карточка не отображается, используйте кнопки ниже или /start.", "reply_markup": map[string]any{"inline_keyboard": keyboard}})
}
func (b *Bot) home(uid, msg int64) error {
	h, e := b.Family.Current(uid)
	if e != nil {
		return b.card(uid, msg, "Добро пожаловать домой", "Создайте дом или откройте приглашение близкого человека.", 0, "create", "Создать дом")
	}
	opts := []string{"tasks", "Сегодня", "new_task", "Добавить задание", "points", "Мои баллы", "wishes:0", "Подарки", "settings", "Настройки"}

	return b.card(uid, msg, h.Name, fmt.Sprintf("Часовой пояс: %s\nУчастников: %d\nДела по дому — общий вклад в уют.", h.Timezone, len(h.Members)), h.ID, opts...)
}
func (b *Bot) message(uid int64, text string) error {
	if len(text) > 4096 {
		return b.say(uid, "Сообщение слишком длинное")
	}
	if text == "/back" {
		state, e := b.Family.Step(uid)
		if e != nil {
			return e
		}
		switch state.Step {
		case "timezone":
			if e = b.Family.SetStep(uid, "name", ""); e != nil {
				return e
			}
			return b.say(uid, "Как назвать ваш дом?")
		case "task_points":
			if e = b.Family.SetStep(uid, "task_name", ""); e != nil {
				return e
			}
			return b.say(uid, "Как называется задание?")
		}
	}
	if text == "/cancel" || text == "/back" {
		if e := b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.home(uid, 0)
	}
	if strings.HasPrefix(text, "/start join_") {
		if e := b.Family.Join(uid, strings.TrimPrefix(text, "/start join_"), b.Now()); e != nil {
			return b.say(uid, e.Error())
		}
		return b.home(uid, 0)
	}
	if text == "/start" || text == "/home" {
		if e := b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.home(uid, 0)
	}
	if text == "/create" {
		if _, e := b.Family.Current(uid); e == nil {
			return b.home(uid, 0)
		}
		if e := b.Family.SetStep(uid, "name", ""); e != nil {
			return e
		}
		return b.say(uid, "Как назвать ваш дом? До 64 символов. /cancel — отмена.")
	}
	if text == "/task" {
		if _, e := b.Family.Current(uid); e != nil {
			return b.home(uid, 0)
		}
		if e := b.Family.SetStep(uid, "task_name", ""); e != nil {
			return e
		}
		return b.say(uid, "Как называется задание? До 100 символов. /cancel — отмена.")
	}
	id, e := b.Family.Step(uid)
	if e != nil {
		return e
	}
	switch id.Step {
	case "wish_new":
		return b.wishMessage(uid, text)
	case "reward_new", "reward_edit":
		return b.rewardMessage(uid, id.Step, id.Draft, text)
	case "task_edit", "task_due":
		return b.editTaskMessage(uid, id.Step, id.Draft, text)
	case "rest":
		return b.restMessage(uid, text)
	case "schedule":
		return b.scheduleMessage(uid, id.Draft, text)
	case "change_timezone":
		if e = (&task.Service{DB: b.DB}).ChangeTimezone(uid, strings.TrimSpace(text)); e != nil {
			return b.say(uid, e.Error())
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.home(uid, 0)
	case "task_name":
		if len([]rune(strings.TrimSpace(text))) < 1 || len([]rune(text)) > 100 {
			return b.say(uid, "Название: от 1 до 100 символов")
		}
		if e = b.Family.SetStep(uid, "task_points", text); e != nil {
			return e
		}
		return b.say(uid, "Сколько баллов за выполнение? Целое число от 1 до 10000. /cancel — отмена.")
	case "task_points":
		points, e := strconv.Atoi(text)
		if e != nil || points < 1 || points > 10000 {
			return b.say(uid, "Введите целое число от 1 до 10000")
		}
		home, e := b.Family.Current(uid)
		if e != nil {
			return b.home(uid, 0)
		}
		if _, e = (&task.Service{DB: b.DB}).Create(uid, home.ID, id.Draft, points, nil); e != nil {
			return e
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.tasks(uid, 0, 0)
	case "name":
		if len([]rune(strings.TrimSpace(text))) < 1 || len([]rune(text)) > 64 {
			return b.say(uid, "Введите название от 1 до 64 символов")
		}
		if e = b.Family.SetStep(uid, "timezone", strings.TrimSpace(text)); e != nil {
			return e
		}
		return b.say(uid, "Укажите часовой пояс IANA, например Europe/Moscow, Europe/Berlin или Asia/Almaty. Мы не определяем его по языку. /back — назад, /cancel — отмена.")
	case "timezone":
		if _, e = b.Family.Create(uid, id.Draft, strings.TrimSpace(text)); e != nil {
			return b.say(uid, e.Error())
		}
		if e = b.Family.SetStep(uid, "", ""); e != nil {
			return e
		}
		return b.home(uid, 0)
	default:
		return b.enqueueAI(uid, text, "")
	}
}
func (b *Bot) action(uid int64, token string, msg int64) error {
	var a Action
	if len(token) != 32 || b.DB.First(&a, "token = ? AND viewer = ? AND expires > ?", token, uid, b.Now()).Error != nil {
		return b.say(uid, "Эта кнопка недоступна или устарела. Откройте /start.")
	}
	if a.CircleID != 0 {
		if _, e := b.Family.View(uid, a.CircleID); e != nil {
			return b.say(uid, "Доступ к этому дому закончился. Откройте /start.")
		}
	}
	if a.InstanceID != 0 {
		var instance task.Instance
		if b.DB.First(&instance, "id = ? AND circle_id = ? AND version = ?", a.InstanceID, a.CircleID, a.Version).Error != nil {
			return b.say(uid, "Правила задания изменились. Откройте свежую карточку через /start.")
		}
	}
	if a.Used {
		return b.home(uid, msg)
	}
	if e := b.DB.Model(&a).Update("used", true).Error; e != nil {
		return e
	}
	if strings.HasPrefix(a.Kind, "done:") {
		id, e := strconv.Atoi(strings.TrimPrefix(a.Kind, "done:"))
		if e != nil {
			return e
		}
		attempt, e := (&task.Service{DB: b.DB}).Submit(uid, a.CircleID, id, "", b.Now())
		if e != nil {
			return b.say(uid, "Это задание сейчас нельзя выполнить")
		}
		return b.submitted(uid, msg, a.CircleID, attempt)
	}
	if handled, e := b.aiAction(uid, msg, a); handled {
		return e
	}
	if handled, e := b.wishAction(uid, msg, a); handled {
		return e
	}
	if handled, e := b.rewardAction(uid, msg, a); handled {
		return e
	}
	if handled, e := b.reportAction(uid, msg, a); handled {
		return e
	}

	if strings.HasPrefix(a.Kind, "page:") {
		page, e := strconv.Atoi(strings.TrimPrefix(a.Kind, "page:"))
		if e != nil {
			return e
		}
		return b.tasks(uid, msg, page)
	}
	switch a.Kind {
	case "dialog_back":
		return b.message(uid, "/back")
	case "dialog_cancel":
		return b.message(uid, "/cancel")
	case "new_task":
		return b.message(uid, "/task")
	case "tasks":
		return b.tasks(uid, msg, 0)
	case "points":
		return b.points(uid, msg, a.CircleID)
	case "home":
		return b.home(uid, msg)
	case "create":
		return b.message(uid, "/create")
	case "invite":
		token, e := b.Family.Invite(uid, b.Now())
		if e != nil {
			return b.say(uid, "Приглашения доступны владельцу дома")
		}
		return b.say(uid, "Приглашение действует 7 дней:\nhttps://t.me/"+b.Username+"?start=join_"+token)
	case "members":
		return b.members(uid, msg, a.CircleID, 0)
	case "premium":
		return b.premium(uid, msg, a.CircleID)
	case "premium_on", "premium_off":
		if e := (&billing.Service{DB: b.DB, Mode: b.BillingMode}).Manual(uid, a.CircleID, a.Kind == "premium_on", b.Now()); e != nil {
			return b.say(uid, "Тестовый Premium доступен только владельцу в режиме manual_test")
		}
		return b.premium(uid, msg, a.CircleID)
	case "premium_stats":
		return b.premiumStats(uid, msg, a.CircleID)
	case "settings":
		h, e := b.Family.Current(uid)
		if e != nil {
			return e
		}
		opts := []string{"members", "Участники", "leave", "Выйти из дома", "reports", "Отчёты", "notifications", "Напоминания", "home", "Назад"}
		if h.Owner {
			opts = []string{"invite", "Пригласить", "members", "Участники", "manage", "Управление", "reports", "Отчёты", "notifications", "Напоминания", "home", "Назад"}
		}
		return b.card(uid, msg, "Настройки", "Выход сохраняет общую историю дома.", h.ID, opts...)
	case "manage":
		return b.card(uid, msg, "Управление домом", "Приглашения можно отозвать в любой момент.", a.CircleID, "revoke", "Отозвать приглашения", "timezone", "Часовой пояс", "wish_cleanup", "Брони ушедших", "leave", "Выйти из дома", "home", "Назад")
	case "revoke":
		if e := b.Family.Revoke(uid); e != nil {
			return b.say(uid, "Только владелец управляет приглашениями")
		}
		return b.say(uid, "Приглашения отозваны")
	case "leave":
		return b.card(uid, msg, "Выйти из дома?", "Общая история останется у семьи.", a.CircleID, "confirm_leave", "Подтвердить выход", "home", "Отмена")
	case "confirm_leave":
		if e := b.Family.Leave(uid); e != nil {
			return b.say(uid, e.Error())
		}
		return b.home(uid, 0)
	default:
		return b.say(uid, "Карточка обновилась. Откройте /start для новой версии.")
	}
}

func (b *Bot) tasks(uid, msg int64, page int) error {
	h, e := b.Family.Current(uid)
	if e != nil {
		return b.home(uid, msg)
	}
	items, e := (&task.Service{DB: b.DB}).List(uid, h.ID)
	if e != nil {
		return e
	}
	if len(items) == 0 {
		return b.card(uid, msg, "Сегодня", "Пока нет доступных заданий. Можно добавить первое.", h.ID, "new_task", "Добавить задание", "home", "Дом")
	}
	if page < 0 || page >= len(items) {
		page = 0
	}
	item := items[page]
	name := []rune(item.Name)
	if len(name) > 60 {
		name = append(name[:59], '…')
	}
	assignment := "Может выполнить любой"
	if item.AssignedTo != nil {
		assignment = "Назначенное задание"
		for _, m := range h.Members {
			if m.ID == *item.AssignedTo {
				n := []rune(m.Name)
				if len(n) > 20 {
					n = n[:20]
				}
				assignment = "Исполнитель: " + string(n)
			}
		}
	}
	if item.Status == 2 {
		assignment = "На паузе"
	}
	return b.card(uid, msg, string(name), fmt.Sprintf("%d баллов · %s\n%s\nЗадание %d из %d", item.Points, assignment, dueText(item.DueAt, h.Timezone), page+1, len(items)), h.ID, fmt.Sprintf("done:%d", item.ID), "Выполнено", fmt.Sprintf("page:%d", (page+1)%len(items)), "Следующее", fmt.Sprintf("options:%d", item.ID), "Параметры", "home", "Дом")
}
