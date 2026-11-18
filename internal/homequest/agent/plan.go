package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ch "donetick.com/core/internal/chore/model"
	"donetick.com/core/internal/homequest/billing"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"donetick.com/core/internal/homequest/wishlist"
	"gorm.io/gorm"
)

type Change struct {
	Timezone   string            `json:"timezone,omitempty"`
	QuietStart *int              `json:"quiet_start,omitempty"`
	QuietEnd   *int              `json:"quiet_end,omitempty"`
	Days       int               `json:"days,omitempty"`
	Minutes    int               `json:"minutes,omitempty"`
	Choices    []task.PresetTask `json:"choices,omitempty" jsonschema:"description=For preset_apply only. Omit for defaults; read_overview presets supplies editable task keys and schedules."`
	Offsets    *[]int            `json:"offsets,omitempty"`

	RoomFromStep int              `json:"room_from_step,omitempty" jsonschema:"description=For task_create, one-based index of earlier room_create in this plan; mutually exclusive with target"`
	Link         string           `json:"link,omitempty"`
	Price        string           `json:"price,omitempty" jsonschema:"description=Informational gift price only"`
	AssignedTo   *int             `json:"assigned_to,omitempty" jsonschema:"description=Member ID for task_create; omit for shared task"`
	Reviewer     *int             `json:"reviewer,omitempty" jsonschema:"description=Member ID for task_create with approval; omit for any other adult"`
	Snapshot     string           `json:"snapshot,omitempty" jsonschema:"description=Optional; the server attaches the observed snapshot automatically"`
	Operation    string           `json:"operation" jsonschema:"description=One operation from the provided catalog"`
	ID           int              `json:"id,omitempty"`
	Version      int              `json:"version,omitempty" jsonschema:"description=Task instance version from read_tasks only; omit for all other kinds"`
	Name         string           `json:"name,omitempty"`
	Points       int              `json:"points,omitempty"`
	Due          string           `json:"due,omitempty" jsonschema:"description=Local family time YYYY-MM-DD HH:MM or без срока"`
	Description  string           `json:"description,omitempty"`
	Target       int              `json:"target,omitempty" jsonschema:"description=Member ID for assignment or room ID for task_room; zero means shared/unset"`
	Schedule     *task.Schedule   `json:"schedule,omitempty"`
	Approval     bool             `json:"approval,omitempty"`
	Photo        bool             `json:"photo,omitempty"`
	Members      []int            `json:"members,omitempty"`
	Items        []shopping.Input `json:"items,omitempty"`
}
type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Target      string `json:"target"`
}

// Catalog is shared by validation, discovery and the model. No arbitrary code/SQL tool.
var Catalog = []Capability{
	{"task_create", "Создать задание: name, points; target=комната (0 — выбранная); schedule/due, assigned_to, description, approval, photo, reviewer необязательны", "new"},
	{"task_edit", "Изменить название/баллы (пустое name или 0 points сохраняет поле)", "task"},
	{"task_due", "Перенести срок: due в локальном времени семьи, расписание сохраняется", "task"},
	{"task_schedule", "Изменить повторение: schedule", "task"},
	{"task_assign", "Назначить участника: target; 0 — общий пул", "task"},
	{"task_room", "Переместить в комнату: target; 0 — без комнаты", "task"},
	{"task_description", "Описание: description", "task"},
	{"task_configure", "Правила: approval и photo, оба значения задаются явно", "task"},
	{"task_reviewer", "Назначить проверяющего: target; 0 — любой другой взрослый", "task"},
	{"task_rotation", "Чередование: members, пустой список — общий пул", "task"},
	{"task_pause", "Приостановить повторение", "task"},
	{"task_resume", "Возобновить повторение", "task"},
	{"task_cancel", "Отменить задание, сохранив историю", "task"},
	{"task_complete", "Отправить выполнение; фото/проверка остаются обязательными", "task"},
	{"room_create", "Добавить комнату: name", "new"},
	{"premium_on", "Включить семейный Premium для теста без оплаты", "setting"},
	{"premium_off", "Выключить тестовый Premium", "setting"},
	{"weekly_goal", "Цель со следующей недели: points", "setting"},
	{"digest_on", "Включить семейный недельный итог", "setting"},
	{"digest_off", "Выключить семейный недельный итог", "setting"},
	{"family_timezone", "Изменить часовой пояс дома: name в формате IANA", "setting"},
	{"family_create", "Создать первый дом: name и явно выбранный пользователем IANA timezone", "setting"},
	{"family_switch", "Выбрать другой свой дом с точным названием name", "setting"},
	{"family_leave", "Выйти из текущего дома, сохраняя историю", "setting"},
	{"invite_revoke", "Отозвать приглашения дома", "setting"},
	{"invite_create", "Создать приглашение на семь дней и прислать ссылку в личку", "setting"},
	{"reminder_mute", "Выключить личные напоминания", "setting"},
	{"reminder_unmute", "Включить личные напоминания", "setting"},
	{"quiet_hours", "Изменить тихие часы: quiet_start, quiet_end от 0 до 23", "setting"},
	{"vacation", "Приостановить личные уведомления: days до 365, 0 для завершения отпуска", "setting"},
	{"reminder_offsets", "Изменить семейные напоминания: offsets (минуты относительно срока)", "setting"},
	{"task_snooze", "Отложить своё последнее полученное напоминание по задаче: minutes", "task"},
	{"task_handoff", "Предложить другому участнику взять задание: target", "task"},
	{"handoff_accept", "Принять предложение взять задание", "handoff"},
	{"handoff_decline", "Отклонить предложение взять задание", "handoff"},
	{"handoff_cancel", "Отозвать предложение передать задание", "handoff"},
	{"wish_cleanup", "Освободить брони ушедших участников", "setting"},
	{"preset_apply", "Добавить готовый набор: name=kitchen, bathroom или daily", "setting"},

	{"member_remove", "Исключить участника: target", "setting"},
	{"owner_transfer", "Передать права владельца участнику: target", "setting"},
	{"report_approve", "Принять чужой отчёт", "report"},
	{"report_reject", "Отклонить чужой отчёт", "report"},
	{"report_undo", "Отменить своё прямое выполнение в течение пяти минут", "report"},
	{"report_photo", "Получить прикреплённое фотоотчёта отдельным сообщением", "report"},
	{"room_rename", "Переименовать комнату: name", "room"},
	{"room_delete", "Удалить группировку по комнате; задания сохранятся", "room"},
	{"wish_create", "Добавить своё желание: name, description, link, price (справочная цена)", "new"},
	{"wish_edit", "Изменить своё желание, передать все поля name, description, link, price", "wish"},
	{"wish_archive", "Убрать своё желание", "wish"},
	{"wish_claim", "Тайно забронировать подарок другого участника", "wish"},
	{"wish_purchase", "Отметить покупку забронированного вами подарка", "wish"},
	{"wish_release", "Снять свою тайную бронь", "wish"},
	{"shopping_add", "Добавить покупки: items", "new"},
	{"shopping_edit", "Изменить покупку: items с одной полной позицией name/quantity/note", "shopping"},
	{"shopping_claim", "Взять покупку на себя", "shopping"},
	{"shopping_release", "Освободить свою покупку", "shopping"},
	{"shopping_buy", "Отметить купленным", "shopping"},
	{"shopping_reopen", "Вернуть купленное в список", "shopping"},
	{"shopping_archive", "Убрать покупку из списка", "shopping"},
	{"reward_create", "Добавить награду: name, points (цена), description", "new"},
	{"reward_edit", "Изменить награду: полные name, points, description", "reward"},
	{"reward_archive", "Архивировать награду, сохранив заявки", "reward"},
	{"reward_request", "Запросить награду для себя; списать её текущую цену один раз", "reward"},
	{"redemption_approve", "Принять чужую заявку на награду", "redemption"},
	{"redemption_deliver", "Отметить выдачу принятой награды другому участнику", "redemption"},
	{"redemption_reject", "Отклонить чужую заявку с возвратом исходной суммы", "redemption"},
	{"redemption_cancel", "Отменить свою заявку до выдачи с возвратом исходной суммы", "redemption"},
}

func capability(name string) (Capability, bool) {
	for _, c := range Catalog {
		if c.Name == name {
			return c, true
		}
	}
	return Capability{}, false
}

type Plan struct {
	ReplyTarget                ReplyTarget `gorm:"embedded;embeddedPrefix:reply_target_"`
	ID                         string      `gorm:"primaryKey"`
	Viewer                     int64       `gorm:"uniqueIndex:hq_agent_request"`
	Request                    string      `gorm:"uniqueIndex:hq_agent_request"`
	CircleID, Revision, RoomID int
	Timezone                   string
	State                      string
	Changes, Summary, Result   string
	Expires                    time.Time
	CreatedAt                  time.Time
}

func (Plan) TableName() string { return "hq_agent_plans" }

type Effect struct {
	Operation string `json:"operation"`
	ID        int    `json:"id"`
	AttemptID int    `json:"attempt_id,omitempty"`
	Status    string `json:"status"`
}

var errPreview = errors.New("rollback preview")

func (s *Service) Prepare(c Context, request string, changes []Change, now time.Time) (Plan, error) {
	var p Plan
	if e := CheckReplyChanges(c, changes); e != nil {
		return p, e
	}
	if request == "" || len(request) > 100 || len(changes) < 1 || len(changes) > 10 {
		return p, ErrPlan
	}
	raw, e := json.Marshal(changes)
	if e != nil || len(raw) > 12000 {
		return p, ErrPlan
	}
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		child := s.withDB(tx)
		if e := child.Check(c, now); e != nil {
			return e
		}
		e := tx.First(&p, "viewer=? AND request=?", c.Viewer, request).Error
		if e == nil {
			if p.Changes != string(raw) || p.Revision != c.Revision || p.State != "pending" || !now.Before(p.Expires) {
				return ErrPlan
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		id, e := token()
		if e != nil {
			return e
		}
		p = Plan{ID: id, Viewer: c.Viewer, Request: request, CircleID: c.CircleID, RoomID: c.RoomID, Revision: c.Revision, Timezone: c.Timezone, State: "pending", Changes: string(raw), Expires: now.Add(15 * time.Minute), CreatedAt: now}
		if c.ReplyTarget != nil {
			p.ReplyTarget = *c.ReplyTarget
		}
		if e = child.validateTargets(p, changes, now); e != nil {
			return e
		}
		// Execute the SAME domain services in a rolled-back savepoint. These services
		// only write this DB, never call Telegram/LLM. This validates permissions and
		// the entire chain without copying rules or committing ledger/jobs/attempts.
		e = tx.Transaction(func(dry *gorm.DB) error {
			_, e := s.withDB(dry).execute(p, changes, now)
			if e != nil {
				return e
			}
			return errPreview
		})
		if !errors.Is(e, errPreview) {
			if e == nil {
				return ErrPlan
			}
			return e
		}
		summary, e := child.describe(p, changes, now)
		if e != nil {
			return e
		}
		p.Summary = summary
		// One pending intent per user. A newer preview supersedes the previous one.
		if e = tx.Model(&Plan{}).Where("viewer=? AND state=?", c.Viewer, "pending").Updates(map[string]any{"state": "superseded", "changes": "", "summary": ""}).Error; e != nil {
			return e
		}
		return tx.Create(&p).Error
	})
	return p, e
}
func (s *Service) validateTargets(p Plan, changes []Change, now time.Time) error {
	seen := map[int]int{}
	resources := map[string]bool{}
	for index, v := range changes {
		if e := p.ReplyTarget.checkChange(v); e != nil {
			return e
		}
		if v.RoomFromStep != 0 && (v.RoomFromStep < 1 || v.RoomFromStep > index || v.Target != 0 || changes[v.RoomFromStep-1].Operation != "room_create") {
			return ErrPlan
		}
		if p.CircleID == 0 && v.Operation != "family_create" && v.Operation != "family_switch" {
			return ErrPlan
		}
		if (v.Operation == "family_switch" || v.Operation == "family_leave" || v.Operation == "family_create") && len(changes) != 1 {
			return ErrPlan
		}
		if e := validateFields(v); e != nil {
			return e
		}
		cap, ok := capability(v.Operation)
		if !ok {
			return ErrPlan
		}
		if len([]rune(v.Name)) > 100 || len([]rune(v.Description)) > 1000 || len(v.Due) > 32 || len(v.Members) > 20 || len(v.Items) > 10 {
			return ErrPlan
		}
		if resourceTarget(cap.Target) {
			if v.ID <= 0 || v.Version != 0 || len(v.Snapshot) != 64 {
				return ErrPlan
			}
			key := fmt.Sprintf("%s:%d", cap.Target, v.ID)
			// One change per resource prevents a preview from charging a price edited
			// earlier in the same plan. Task chains retain their existing semantics.
			if resources[key] {
				return ErrPlan
			}
			resources[key] = true
			r, e := s.resource(p, v, now)
			if e != nil {
				return e
			}
			if r.Snapshot != v.Snapshot {
				return ErrChanged
			}
		} else if v.Snapshot != "" {
			return ErrPlan
		} else if cap.Target == "task" {
			var i task.Instance
			if v.ID <= 0 || v.Version <= 0 || s.DB.First(&i, "id=? AND circle_id=? AND version=? AND closed=? AND pending=?", v.ID, p.CircleID, v.Version, false, false).Error != nil {
				return ErrChanged
			}
			if ver, exists := seen[v.ID]; exists && ver != v.Version {
				return ErrChanged
			}
			seen[v.ID] = v.Version
		} else if v.ID != 0 || v.Version != 0 {
			return ErrPlan
		}
	}
	return nil
}
func (s *Service) Apply(uid int64, id string, now time.Time) (out []Effect, err error) {
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var p Plan
		if tx.First(&p, "id=? AND viewer=?", id, uid).Error != nil {
			return ErrPlan
		}
		child := s.withDB(tx)
		c, e := child.Context(uid, now)
		if e != nil {
			return e
		}
		if c.CircleID != p.CircleID || c.RoomID != p.RoomID || c.Revision != p.Revision || c.Timezone != p.Timezone {
			return ErrChanged
		}
		if !now.Before(p.Expires) {
			return ErrPlan
		}
		if p.State == "applied" {
			return json.Unmarshal([]byte(p.Result), &out)
		}
		if p.State != "pending" {
			return ErrPlan
		}
		var changes []Change
		if json.Unmarshal([]byte(p.Changes), &changes) != nil {
			return ErrPlan
		}
		if e = child.validateTargets(p, changes, now); e != nil {
			return e
		}
		out, e = child.execute(p, changes, now)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(out)
		r := tx.Model(&p).Where("state=?", "pending").Updates(map[string]any{"state": "applied", "result": string(raw)})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrPlan
		}
		return nil
	})
	return
}
func (s *Service) Cancel(uid int64, id string, now time.Time) error {
	c, e := s.Context(uid, now)
	if e != nil {
		return e
	}
	return s.DB.Model(&Plan{}).Where("id=? AND viewer=? AND circle_id=? AND state=?", id, uid, c.CircleID, "pending").Updates(map[string]any{"state": "cancelled", "changes": "", "summary": ""}).Error
}
func (s *Service) execute(p Plan, changes []Change, now time.Time) ([]Effect, error) {
	ts := &task.Service{DB: s.DB}
	out := []Effect{}
	for index, v := range changes {
		r := Effect{Operation: v.Operation, ID: v.ID, Status: "updated"}
		var e error
		switch v.Operation {
		case "task_create":
			var item task.Item
			item, e = ts.Create(p.Viewer, p.CircleID, v.Name, v.Points, v.AssignedTo)
			if e == nil {
				r.ID = item.ID
				r.Status = "created"
				room := v.Target
				if v.RoomFromStep > 0 {
					room = out[v.RoomFromStep-1].ID
				}
				if room == 0 {
					room = p.RoomID
				}
				if room != 0 {
					e = ts.SetRoom(p.Viewer, p.CircleID, item.ID, room)
				}
			}
			if e == nil && v.Schedule != nil {
				e = ts.SetSchedule(p.Viewer, p.CircleID, item.ID, *v.Schedule, now)
			}
			if e == nil && v.Due != "" {
				e = ts.SetDue(p.Viewer, p.CircleID, item.ID, v.Due)
			}
			if e == nil && v.Description != "" {
				e = ts.SetDescription(p.Viewer, p.CircleID, item.ID, v.Description)
			}
			if e == nil && (v.Approval || v.Photo) {
				e = ts.Configure(p.Viewer, p.CircleID, item.ID, v.Approval, v.Photo)
			}
			if e == nil && v.Reviewer != nil {
				e = ts.SetReviewer(p.Viewer, p.CircleID, item.ID, *v.Reviewer)
			}
		case "task_edit":
			var i task.Instance
			var d ch.Chore
			if e = s.DB.First(&i, "id=? AND circle_id=?", v.ID, p.CircleID).Error; e == nil {
				e = s.DB.First(&d, "id=? AND circle_id=?", i.ChoreID, p.CircleID).Error
			}
			if e == nil {
				name := v.Name
				if name == "" {
					name = d.Name
				}
				points := v.Points
				if points == 0 && d.Points != nil {
					points = *d.Points
				}
				e = ts.Edit(p.Viewer, p.CircleID, v.ID, name, points)
			}
		case "task_due":
			e = ts.SetDue(p.Viewer, p.CircleID, v.ID, v.Due)
		case "task_schedule":
			if v.Schedule == nil {
				return nil, ErrPlan
			}
			e = ts.SetSchedule(p.Viewer, p.CircleID, v.ID, *v.Schedule, now)
		case "task_assign":
			e = ts.Manage(p.Viewer, p.CircleID, v.ID, "assign", v.Target)
		case "task_room":
			e = ts.SetRoom(p.Viewer, p.CircleID, v.ID, v.Target)
		case "task_description":
			e = ts.SetDescription(p.Viewer, p.CircleID, v.ID, v.Description)
		case "task_configure":
			e = ts.Configure(p.Viewer, p.CircleID, v.ID, v.Approval, v.Photo)
		case "task_reviewer":
			e = ts.SetReviewer(p.Viewer, p.CircleID, v.ID, v.Target)
		case "task_rotation":
			e = ts.SetRotation(p.Viewer, p.CircleID, v.ID, v.Members)
		case "task_pause", "task_resume", "task_cancel":
			e = ts.Manage(p.Viewer, p.CircleID, v.ID, strings.TrimPrefix(v.Operation, "task_"), 0)
		case "task_complete":
			var a task.Attempt
			a, e = ts.Submit(p.Viewer, p.CircleID, v.ID, "", now)
			r.AttemptID = a.ID
			r.Status = a.Status
		case "invite_create", "task_snooze", "task_handoff", "reminder_mute", "reminder_unmute", "quiet_hours", "vacation", "reminder_offsets":
			if s.LocalAction == nil {
				return nil, ErrPlan
			}
			e = s.LocalAction(s.DB, p, v, now)
		case "preset_apply":
			_, e = ts.ApplyPreset(p.Viewer, p.CircleID, v.Name, v.presetChoices(), now)
		case "wish_cleanup":
			e = (&wishlist.Service{DB: s.DB}).ReleaseDeparted(p.Viewer, p.CircleID)
		case "premium_on", "premium_off":
			e = (&billing.Service{DB: s.DB, Mode: s.BillingMode}).Manual(p.Viewer, p.CircleID, v.Operation == "premium_on", now)
		case "weekly_goal":
			e = (&stats.Service{DB: s.DB}).SetWeeklyGoal(p.Viewer, p.CircleID, v.Points, now)
		case "digest_on", "digest_off":
			e = (&stats.Service{DB: s.DB}).SetDigest(p.Viewer, p.CircleID, v.Operation == "digest_on")
		case "family_create":
			_, e = (&family.Service{DB: s.DB}).Create(p.Viewer, v.Name, v.Timezone)
		case "family_leave":
			e = (&family.Service{DB: s.DB}).Leave(p.Viewer)
		case "family_switch":
			f := &family.Service{DB: s.DB}
			homes, err := f.Homes(p.Viewer)
			if err != nil {
				return nil, err
			}
			target := 0
			for _, h := range homes {
				if strings.EqualFold(strings.TrimSpace(v.Name), h.Name) {
					if target != 0 {
						return nil, ErrPlan
					}
					target = h.ID
				}
			}
			if target == 0 {
				return nil, ErrPlan
			}
			e = f.Switch(p.Viewer, target)
		case "family_timezone":
			e = ts.ChangeTimezone(p.Viewer, v.Name)
		case "member_remove":
			e = (&family.Service{DB: s.DB}).Remove(p.Viewer, v.Target)
		case "owner_transfer":
			e = (&family.Service{DB: s.DB}).Transfer(p.Viewer, v.Target)
		case "invite_revoke":
			e = (&family.Service{DB: s.DB}).Revoke(p.Viewer)
		case "room_create":
			var room task.Room
			room, e = ts.CreateRoom(p.Viewer, p.CircleID, v.Name)
			r.ID = room.ID
			r.Status = "created"
		case "shopping_add":
			e = (&shopping.Service{DB: s.DB}).CreateBatch(p.Viewer, p.CircleID, fmt.Sprintf("agent:%s:%d", p.ID, index), v.Items, now)
			r.Status = "created"
		default:
			r, e = s.executeResource(p, v, index, now)
		}
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *Service) describe(p Plan, changes []Change, now time.Time) (string, error) {
	if len(changes) == 1 {
		switch changes[0].Operation {
		case "family_create":
			return "Создать дом «" + changes[0].Name + "» · " + changes[0].Timezone, nil
		case "family_switch":
			return "Перейти в дом «" + changes[0].Name + "»", nil
		}
	}
	h, e := (&family.Service{DB: s.DB}).View(p.Viewer, p.CircleID)
	if e != nil {
		return "", e
	}
	rooms, e := (&task.Service{DB: s.DB}).Rooms(p.Viewer, p.CircleID)
	if e != nil {
		return "", e
	}
	member := func(id int) string {
		if id == 0 {
			return "может выполнить любой"
		}
		for _, m := range h.Members {
			if m.ID == id {
				return m.Name
			}
		}
		return "участник недоступен"
	}
	room := func(id int) string {
		if id == 0 {
			return "без комнаты"
		}
		for _, r := range rooms {
			if r.ID == id {
				return r.Name
			}
		}
		return "комната недоступна"
	}
	yes := func(v bool) string {
		if v {
			return "нужно"
		}
		return "не нужно"
	}
	lines := []string{}
	for _, v := range changes {
		cap, _ := capability(v.Operation)
		if cap.Target == "setting" {
			label := strings.Split(cap.Description, ":")[0]
			switch v.Operation {
			case "quiet_hours":
				label += fmt.Sprintf(" · %02d–%02d", *v.QuietStart, *v.QuietEnd)
			case "vacation":
				label += fmt.Sprintf(" · дней: %d", v.Days)
			case "reminder_offsets":
				label += fmt.Sprintf(" · минуты: %v", *v.Offsets)
			case "preset_apply":
				label += " · " + task.PresetNames[v.Name]
				for _, choice := range v.presetChoices() {
					if !choice.Enabled {
						continue
					}
					label += fmt.Sprintf("\n%s · %d баллов · каждые %d дн. в %02d:%02d", choice.Name, choice.Points, choice.Every, choice.Hour, choice.Minute)
				}
			case "weekly_goal":
				label += fmt.Sprintf(" · %d баллов", v.Points)
			case "family_timezone":
				label += " · " + v.Name
			case "member_remove", "owner_transfer":
				label += fmt.Sprintf(" · %s (участник №%d)", member(v.Target), v.Target)
			}
			lines = append(lines, label)
			continue
		}
		if resourceTarget(cap.Target) || v.Operation == "reward_create" || v.Operation == "wish_create" {
			text, e := s.describeResource(p, v, now)
			if e != nil {
				return "", e
			}
			lines = append(lines, text)
			continue
		}
		name := v.Name
		var definition ch.Chore
		if v.ID > 0 {
			var i task.Instance
			if s.DB.First(&i, "id=? AND circle_id=?", v.ID, p.CircleID).Error != nil {
				return "", family.ErrDenied
			}
			if s.DB.First(&definition, i.ChoreID).Error != nil {
				return "", family.ErrDenied
			}
			name = definition.Name
		}
		text := name
		if v.ID > 0 {
			var ids []int
			if e = s.DB.Table("hq_rooms r").Select("r.id").Joins("JOIN chore_labels cl ON cl.label_id=r.label_id").Where("r.circle_id=? AND cl.chore_id=?", p.CircleID, definition.ID).Scan(&ids).Error; e != nil {
				return "", e
			}
			actual := 0
			if len(ids) > 0 {
				actual = ids[0]
			}
			text += " · " + room(actual)
		}
		switch v.Operation {
		case "task_due":
			text += " → " + v.Due + " (" + h.Timezone + ")"
		case "task_edit":
			if v.Name != "" {
				text += " → «" + v.Name + "»"
			}
			if v.Points != 0 {
				text += fmt.Sprintf(" · %d баллов", v.Points)
			}
		case "task_create":
			target := v.Target
			if target == 0 {
				target = p.RoomID
			}
			roomName := room(target)
			if v.RoomFromStep > 0 {
				roomName = changes[v.RoomFromStep-1].Name
			}
			text = fmt.Sprintf("Создать «%s» · %d баллов · %s", name, v.Points, roomName)
			if v.Schedule != nil {
				text += " · " + scheduleLabel(*v.Schedule)
			}
			if v.Due != "" {
				text += " · " + v.Due + " (" + h.Timezone + ")"
			}
			if v.AssignedTo != nil {
				text += " · исполнитель: " + member(*v.AssignedTo)
			}
			if v.Description != "" {
				text += " · описание: " + v.Description
			}
			if v.Approval || v.Photo {
				text += " · подтверждение: " + yes(v.Approval) + ", фото: " + yes(v.Photo)
			}
			if v.Reviewer != nil {
				text += " · проверяет: " + member(*v.Reviewer)
			}
		case "task_complete":
			text = "Отправить выполнение: " + text + " (фото и проверка сохраняются)"
		case "task_schedule":
			if v.Schedule != nil {
				text += " · " + scheduleLabel(*v.Schedule) + " (" + h.Timezone + ")"
			}
		case "task_snooze":
			text += fmt.Sprintf(" · напомнить через %d мин.", v.Minutes)
		case "task_handoff":
			text += " · предложить взять дело: " + member(v.Target)
		case "task_assign":
			text += " → " + member(v.Target)
		case "task_reviewer":
			if v.Target == 0 {
				text += " · проверяет любой другой взрослый"
			} else {
				text += " · проверяет: " + member(v.Target)
			}
		case "task_room":
			text += " → " + room(v.Target)
		case "task_configure":
			text += " · подтверждение: " + yes(v.Approval) + ", фото: " + yes(v.Photo)
		case "task_rotation":
			names := []string{}
			for _, id := range v.Members {
				names = append(names, member(id))
			}
			if len(names) == 0 {
				text += " · общий пул"
			} else {
				text += " · по очереди: " + strings.Join(names, " → ")
			}
		case "task_description":
			text += " · описание: " + v.Description
		case "task_pause":
			text = "Приостановить: " + text
		case "task_resume":
			text = "Возобновить: " + text
		case "task_cancel":
			text = "Отменить: " + text
		case "room_create":
			text = "Добавить комнату: " + text
		case "shopping_add":
			items := []string{}
			for _, item := range v.Items {
				items = append(items, strings.TrimSpace(item.Name+" "+item.Quantity+" "+item.Note))
			}
			text = "Добавить покупки: " + strings.Join(items, "; ")
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n"), nil
}
func (v Change) presetChoices() []task.PresetTask {
	if v.Choices != nil {
		return v.Choices
	}
	return task.PresetTasks(v.Name)
}
func scheduleLabel(s task.Schedule) string {
	at := fmt.Sprintf(" в %02d:%02d", s.Hour, s.Minute)
	switch s.Mode {
	case "once":
		return "без повторения"
	case "daily":
		return "каждый день" + at
	case "interval":
		return fmt.Sprintf("каждые %d дн.", s.Every) + at
	case "after":
		return fmt.Sprintf("через %d дн. после принятия", s.Every) + at
	case "weekdays":
		names := map[string]string{"monday": "пн", "tuesday": "вт", "wednesday": "среда", "thursday": "чт", "friday": "пт", "saturday": "сб", "sunday": "вс"}
		days := []string{}
		for _, d := range s.Days {
			days = append(days, names[d])
		}
		return strings.Join(days, ", ") + at
	}
	return "расписание недоступно"
}

// Reject fields this operation does not implement: an omitted photo requirement
// must never turn into a silently accepted ordinary task.
type unsupportedField struct{ name string }

func (e *unsupportedField) Error() string { return "unsupported field: " + e.name }
func (*unsupportedField) Unwrap() error   { return ErrPlan }

func validateFields(v Change) error {
	allowed := map[string]bool{"operation": true, "id": true, "version": true, "snapshot": true}
	var fields []string
	switch v.Operation {
	case "task_create":
		if v.AssignedTo != nil && *v.AssignedTo <= 0 {
			return ErrPlan
		}
		if v.Reviewer != nil && (!v.Approval || *v.Reviewer <= 0) {
			return ErrPlan
		}
		fields = []string{"name", "points", "target", "room_from_step", "schedule", "due", "description", "assigned_to", "reviewer", "approval", "photo"}
	case "premium_on", "premium_off", "invite_revoke", "digest_on", "digest_off", "report_approve", "report_reject", "report_photo", "report_undo":
	case "member_remove", "owner_transfer":
		fields = []string{"target"}
	case "weekly_goal":
		fields = []string{"points"}
	case "quiet_hours":
		if v.QuietStart == nil || v.QuietEnd == nil {
			return ErrPlan
		}
		fields = []string{"quiet_start", "quiet_end"}
	case "vacation":
		if v.Days < 0 || v.Days > 365 {
			return ErrPlan
		}
		fields = []string{"days"}
	case "task_snooze":
		if v.Minutes < 1 || v.Minutes > 1440 {
			return ErrPlan
		}
		fields = []string{"minutes"}
	case "reminder_offsets":
		if v.Offsets == nil {
			return ErrPlan
		}
		fields = []string{"offsets"}
	case "task_handoff":
		fields = []string{"target"}
	case "invite_create", "reminder_mute", "reminder_unmute", "handoff_accept", "handoff_decline", "handoff_cancel":
	case "preset_apply":
		if v.Choices != nil && len(v.Choices) == 0 {
			return ErrPlan
		}
		fields = []string{"name", "choices"}
	case "wish_cleanup":
	case "family_timezone":
		fields = []string{"name"}
	case "family_create":
		fields = []string{"name", "timezone"}
	case "family_switch":
		fields = []string{"name"}
	case "family_leave":
	case "task_edit":
		fields = []string{"name", "points"}
	case "task_due":
		fields = []string{"due"}
	case "task_schedule":
		fields = []string{"schedule"}
	case "task_assign", "task_room", "task_reviewer":
		fields = []string{"target"}
	case "task_description":
		fields = []string{"description"}
	case "task_configure":
		fields = []string{"approval", "photo"}
	case "task_rotation":
		fields = []string{"members"}
	case "task_pause", "task_resume", "task_cancel", "task_complete":
	case "wish_create", "wish_edit":
		fields = []string{"name", "description", "link", "price"}
	case "room_delete", "wish_archive", "wish_claim", "wish_release", "wish_purchase":
	case "room_rename", "room_create":
		fields = []string{"name"}
	case "shopping_add", "shopping_edit":
		fields = []string{"items"}
	case "reward_create", "reward_edit":
		fields = []string{"name", "points", "description"}
	case "shopping_claim", "shopping_release", "shopping_buy", "shopping_reopen", "shopping_archive", "reward_archive", "reward_request", "redemption_approve", "redemption_deliver", "redemption_reject", "redemption_cancel":
	default:
		return ErrPlan
	}
	for _, f := range fields {
		allowed[f] = true
	}
	raw, _ := json.Marshal(v)
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return ErrPlan
	}
	for key := range values {
		if !allowed[key] {
			return &unsupportedField{key}
		}
	}
	return nil
}
