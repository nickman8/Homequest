package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ch "donetick.com/core/internal/chore/model"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/task"
	"gorm.io/gorm"
)

type Change struct {
	Operation   string           `json:"operation" jsonschema:"description=One operation from the provided catalog"`
	ID          int              `json:"id,omitempty"`
	Version     int              `json:"version,omitempty"`
	Name        string           `json:"name,omitempty"`
	Points      int              `json:"points,omitempty"`
	Due         string           `json:"due,omitempty" jsonschema:"description=Local family time YYYY-MM-DD HH:MM or без срока"`
	Description string           `json:"description,omitempty"`
	Target      int              `json:"target,omitempty" jsonschema:"description=Member ID for assignment or room ID for task_room; zero means shared/unset"`
	Schedule    *task.Schedule   `json:"schedule,omitempty"`
	Approval    bool             `json:"approval,omitempty"`
	Photo       bool             `json:"photo,omitempty"`
	Members     []int            `json:"members,omitempty"`
	Items       []shopping.Input `json:"items,omitempty"`
}
type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Target      string `json:"target"`
}

// Catalog is shared by validation, discovery and the model. No arbitrary code/SQL tool.
var Catalog = []Capability{
	{"task_create", "Создать задание: name, points; target=комната (0 — выбранная), schedule/due необязательны", "new"},
	{"task_edit", "Изменить название/баллы (пустое name или 0 points сохраняет поле)", "task"},
	{"task_due", "Перенести срок: due в локальном времени семьи, расписание сохраняется", "task"},
	{"task_schedule", "Изменить повторение: schedule", "task"},
	{"task_assign", "Назначить участника: target; 0 — общий пул", "task"},
	{"task_room", "Переместить в комнату: target; 0 — без комнаты", "task"},
	{"task_description", "Описание: description", "task"},
	{"task_configure", "Правила: approval и photo, оба значения задаются явно", "task"},
	{"task_rotation", "Чередование: members, пустой список — общий пул", "task"},
	{"task_pause", "Приостановить повторение", "task"},
	{"task_resume", "Возобновить повторение", "task"},
	{"task_cancel", "Отменить задание, сохранив историю", "task"},
	{"task_complete", "Отправить выполнение; фото/проверка остаются обязательными", "task"},
	{"room_create", "Добавить комнату: name", "new"},
	{"shopping_add", "Добавить покупки: items", "new"},
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
	ID                         string `gorm:"primaryKey"`
	Viewer                     int64  `gorm:"uniqueIndex:hq_agent_request"`
	Request                    string `gorm:"uniqueIndex:hq_agent_request"`
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
	if request == "" || len(request) > 100 || len(changes) < 1 || len(changes) > 10 {
		return p, ErrPlan
	}
	raw, e := json.Marshal(changes)
	if e != nil || len(raw) > 12000 {
		return p, ErrPlan
	}
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		child := &Service{tx}
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
		if e = child.validateTargets(p, changes); e != nil {
			return e
		}
		// Execute the SAME domain services in a rolled-back savepoint. These services
		// only write this DB, never call Telegram/LLM. This validates permissions and
		// the entire chain without copying rules or committing ledger/jobs/attempts.
		e = tx.Transaction(func(dry *gorm.DB) error {
			_, e := (&Service{dry}).execute(p, changes, now)
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
		summary, e := child.describe(p, changes)
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
func (s *Service) validateTargets(p Plan, changes []Change) error {
	seen := map[int]int{}
	for _, v := range changes {
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
		if cap.Target == "task" {
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
		child := &Service{tx}
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
		if e = child.validateTargets(p, changes); e != nil {
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
			var assigned *int
			var item task.Item
			item, e = ts.Create(p.Viewer, p.CircleID, v.Name, v.Points, assigned)
			if e == nil {
				r.ID = item.ID
				r.Status = "created"
				room := v.Target
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
		case "task_rotation":
			e = ts.SetRotation(p.Viewer, p.CircleID, v.ID, v.Members)
		case "task_pause", "task_resume", "task_cancel":
			e = ts.Manage(p.Viewer, p.CircleID, v.ID, strings.TrimPrefix(v.Operation, "task_"), 0)
		case "task_complete":
			var a task.Attempt
			a, e = ts.Submit(p.Viewer, p.CircleID, v.ID, "", now)
			r.AttemptID = a.ID
			r.Status = a.Status
		case "room_create":
			var room task.Room
			room, e = ts.CreateRoom(p.Viewer, p.CircleID, v.Name)
			r.ID = room.ID
			r.Status = "created"
		case "shopping_add":
			e = (&shopping.Service{DB: s.DB}).CreateBatch(p.Viewer, p.CircleID, fmt.Sprintf("agent:%s:%d", p.ID, index), v.Items, now)
			r.Status = "created"
		default:
			return nil, ErrPlan
		}
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *Service) describe(p Plan, changes []Change) (string, error) {
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
			text = fmt.Sprintf("Создать «%s» · %d баллов · %s", name, v.Points, room(target))
			if v.Schedule != nil {
				text += " · " + scheduleLabel(*v.Schedule)
			}
			if v.Due != "" {
				text += " · " + v.Due + " (" + h.Timezone + ")"
			}
		case "task_complete":
			text = "Отправить выполнение: " + text + " (фото и проверка сохраняются)"
		case "task_schedule":
			if v.Schedule != nil {
				text += " · " + scheduleLabel(*v.Schedule) + " (" + h.Timezone + ")"
			}
		case "task_assign":
			text += " → " + member(v.Target)
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
func validateFields(v Change) error {
	allowed := map[string]bool{"operation": true, "id": true, "version": true}
	var fields []string
	switch v.Operation {
	case "task_create":
		fields = []string{"name", "points", "target", "schedule", "due"}
	case "task_edit":
		fields = []string{"name", "points"}
	case "task_due":
		fields = []string{"due"}
	case "task_schedule":
		fields = []string{"schedule"}
	case "task_assign", "task_room":
		fields = []string{"target"}
	case "task_description":
		fields = []string{"description"}
	case "task_configure":
		fields = []string{"approval", "photo"}
	case "task_rotation":
		fields = []string{"members"}
	case "task_pause", "task_resume", "task_cancel", "task_complete":
	case "room_create":
		fields = []string{"name"}
	case "shopping_add":
		fields = []string{"items"}
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
			return ErrPlan
		}
	}
	return nil
}
