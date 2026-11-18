// Package agent shares the existing domain services; the model never supplies an actor.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	ch "donetick.com/core/internal/chore/model"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"gorm.io/gorm"
)

var ErrChanged = errors.New("Контекст или данные изменились. Отправьте просьбу заново.")
var ErrPlan = errors.New("План недоступен или устарел. Отправьте просьбу заново.")

const MemoryTTL = 24 * time.Hour

type Session struct {
	Viewer                     int64 `gorm:"primaryKey;autoIncrement:false"`
	CircleID, RoomID, Revision int
	References                 string
	History                    string
	UpdatedAt                  time.Time
}

func (Session) TableName() string { return "hq_agent_sessions" }

type Turn struct {
	User   string `json:"user"`
	Result string `json:"result"`
}
type Context struct {
	ReplyTarget   *ReplyTarget    `json:"reply_to,omitempty"`
	ReplyResource *Resource       `json:"reply_resource,omitempty"`
	ReplyTask     int             `json:"reply_to_task_id,omitempty"`
	Viewer        int64           `json:"-"`
	CircleID      int             `json:"-"`
	Revision      int             `json:"-"`
	RoomID        int             `json:"room_id"`
	Room          string          `json:"room"`
	Home          string          `json:"home"`
	Timezone      string          `json:"timezone"`
	LocalTime     string          `json:"local_time"`
	Actor         int             `json:"actor_member_id"`
	Owner         bool            `json:"owner"`
	Members       []family.Member `json:"members"`
	Rooms         []task.Room     `json:"rooms"`
	References    []int           `json:"previous_task_ids"`
	History       []Turn          `json:"recent_conversation"`
}
type Task struct {
	Author          int        `json:"author"`
	Room            int        `json:"room_id"`
	Paused          bool       `json:"paused"`
	Schedule        string     `json:"frequency"`
	Clock           string     `json:"schedule_time,omitempty"`
	Days            []string   `json:"weekdays,omitempty"`
	Every           int        `json:"interval_days,omitempty"`
	AfterAcceptance bool       `json:"after_acceptance,omitempty"`
	Rotation        []int      `json:"rotation,omitempty"`
	Description     *string    `json:"description,omitempty"`
	Reviewer        *int       `json:"reviewer,omitempty"`
	ID              int        `json:"id"`
	Version         int        `json:"version"`
	Name            string     `json:"name"`
	Points          int        `json:"points"`
	DueAt           *time.Time `json:"due_utc"`
	AssignedTo      *int       `json:"assigned_to"`
	Approval        bool       `json:"requires_approval"`
	Photo           bool       `json:"requires_photo"`
}
type Service struct {
	DB          *gorm.DB
	BillingMode string
	LocalAction func(*gorm.DB, Plan, Change, time.Time) error
}

func (s *Service) withDB(db *gorm.DB) *Service {
	child := *s
	child.DB = db
	return &child
}

func New(db *gorm.DB) (*Service, error) { return &Service{DB: db}, db.AutoMigrate(&Session{}, &Plan{}) }
func (s *Service) Context(uid int64, now time.Time) (out Context, err error) {
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		h, e := f.Current(uid)
		var actorID int
		var rooms []task.Room
		if e != nil {
			actorID, e = f.Unattached(uid)
			if e != nil {
				return e
			}
			h = family.View{Name: "Без дома"}
		} else {
			user, e := f.Authorize(uid, h.ID)
			if e != nil {
				return e
			}
			actorID = user.ID
			rooms, e = (&task.Service{DB: tx}).Rooms(uid, h.ID)
			if e != nil {
				return e
			}
		}
		var row Session
		e = tx.First(&row, "viewer=?", uid).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		reset := row.Viewer == 0 || row.CircleID != h.ID || now.Sub(row.UpdatedAt) > MemoryTTL
		name := "Весь дом"
		if row.RoomID != 0 && !reset {
			found := false
			for _, r := range rooms {
				if r.ID == row.RoomID {
					found = true
					name = r.Name
				}
			}
			if !found {
				reset = true
			}
		}
		if reset {
			row = Session{Viewer: uid, CircleID: h.ID, Revision: row.Revision + 1, UpdatedAt: now}
			if e = tx.Save(&row).Error; e != nil {
				return e
			}
		}
		loc, e := time.LoadLocation(h.Timezone)
		if e != nil {
			return e
		}
		out = Context{Viewer: uid, CircleID: h.ID, Revision: row.Revision, RoomID: row.RoomID, Room: name, Home: h.Name, Timezone: h.Timezone, LocalTime: now.In(loc).Format(time.RFC3339), Actor: actorID, Owner: h.Owner, Members: h.Members, Rooms: rooms}
		_ = json.Unmarshal([]byte(row.References), &out.References)
		_ = json.Unmarshal([]byte(row.History), &out.History)
		return nil
	})
	return
}
func (s *Service) SelectRoom(uid int64, room int, now time.Time) (Context, error) {
	var out Context
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		child := s.withDB(tx)
		c, e := child.Context(uid, now)
		if e != nil {
			return e
		}
		if room != 0 {
			found := false
			for _, r := range c.Rooms {
				if r.ID == room {
					found = true
				}
			}
			if !found {
				return family.ErrDenied
			}
		}
		if c.RoomID != room {
			if e = tx.Model(&Session{}).Where("viewer=?", uid).Updates(map[string]any{"room_id": room, "revision": gorm.Expr("revision+1"), "references": "", "history": "", "updated_at": now}).Error; e != nil {
				return e
			}
		}
		out, e = child.Context(uid, now)
		return e
	})
	return out, e
}
func (s *Service) Reset(uid int64, now time.Time) error {
	if _, e := s.Context(uid, now); e != nil {
		return e
	}
	return s.DB.Model(&Session{}).Where("viewer=?", uid).Updates(map[string]any{"room_id": 0, "revision": gorm.Expr("revision+1"), "references": "", "history": "", "updated_at": now}).Error
}
func (s *Service) Check(c Context, now time.Time) error {
	fresh, e := s.Context(c.Viewer, now)
	if e != nil {
		return e
	}
	if fresh.CircleID != c.CircleID || fresh.RoomID != c.RoomID || fresh.Revision != c.Revision || fresh.Timezone != c.Timezone {
		return ErrChanged
	}
	if c.ReplyTarget != nil {
		fresh, e := s.CaptureReply(c.Viewer, c.CircleID, c.ReplyTarget.Kind, c.ReplyTarget.ID, now)
		if e != nil {
			return e
		}
		if fresh != *c.ReplyTarget {
			return ErrChanged
		}
	}
	return nil
}
func (s *Service) Remember(c Context, refs []int, user, result string, now time.Time) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		child := s.withDB(tx)
		if e := child.Check(c, now); e != nil {
			return e
		}
		fresh, e := child.Context(c.Viewer, now)
		if e != nil {
			return e
		}
		if len(refs) > 50 {
			refs = refs[:50]
		}
		history := append(fresh.History, Turn{clip(user, 800), clip(result, 800)})
		if len(history) > 4 {
			history = history[len(history)-4:]
		}
		raw, _ := json.Marshal(history)
		ids, _ := json.Marshal(refs)
		return tx.Model(&Session{}).Where("viewer=? AND revision=?", c.Viewer, c.Revision).Updates(map[string]any{"references": string(ids), "history": string(raw), "updated_at": now}).Error
	})
}
func clip(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n])
	}
	return v
}
func (s *Service) Tasks(c Context, room int, now time.Time) ([]Task, error) {
	if room < -1 {
		return nil, ErrPlan
	}
	if e := s.Check(c, now); e != nil {
		return nil, e
	}
	svc := &task.Service{DB: s.DB}
	if room == 0 {
		room = c.RoomID
	}
	var rows []task.Item
	var e error
	if room > 0 {
		rows, e = svc.RoomTasks(c.Viewer, c.CircleID, room)
	} else {
		rows, e = svc.List(c.Viewer, c.CircleID)
	}
	if e != nil {
		return nil, e
	}
	out := []Task{}
	for _, r := range rows {
		var i task.Instance
		if e = s.DB.First(&i, "id=? AND circle_id=?", r.ID, c.CircleID).Error; e != nil {
			return nil, e
		}
		var rule task.Rule
		e = s.DB.First(&rule, "chore_id=?", i.ChoreID).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, e
		}
		var definition ch.Chore
		if e = s.DB.First(&definition, "id=? AND circle_id=?", i.ChoreID, c.CircleID).Error; e != nil {
			return nil, e
		}
		var roomID int
		if e = s.DB.Table("hq_rooms r").Select("r.id").Joins("JOIN chore_labels cl ON cl.label_id=r.label_id").Where("r.circle_id=? AND cl.chore_id=?", c.CircleID, i.ChoreID).Scan(&roomID).Error; e != nil {
			return nil, e
		}
		rotation, e := svc.Rotation(c.Viewer, c.CircleID, i.ID)
		if e != nil {
			return nil, e
		}
		taskDTO := Task{Room: roomID, Author: definition.CreatedBy, Paused: definition.Status == ch.ChoreStatusPaused, Rotation: rotation, Schedule: string(definition.FrequencyType), AfterAcceptance: definition.IsRolling, ID: r.ID, Version: i.Version, Name: r.Name, Points: r.Points, DueAt: r.DueAt, AssignedTo: r.AssignedTo, Approval: definition.RequireApproval, Photo: rule.PhotoRequired, Description: definition.Description, Reviewer: rule.ReviewerID}
		if meta := definition.FrequencyMetadataV2; meta != nil {
			if at, err := time.Parse(time.RFC3339, meta.Time); err == nil {
				taskDTO.Clock = at.Format("15:04")
			}
			for _, d := range meta.Days {
				if d != nil {
					taskDTO.Days = append(taskDTO.Days, *d)
				}
			}
			if definition.FrequencyType == ch.FrequencyTypeInterval {
				taskDTO.Every = definition.Frequency
			}
		}
		out = append(out, taskDTO)
	}
	return out, nil
}
func token() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func (s *Service) Purge(now time.Time) error {
	if e := s.DB.Model(&Session{}).Where("updated_at < ?", now.Add(-MemoryTTL)).Updates(map[string]any{"history": "", "references": "", "room_id": 0, "revision": gorm.Expr("revision+1"), "updated_at": now}).Error; e != nil {
		return e
	}
	return s.DB.Model(&Plan{}).Where("expires < ? AND state <> ?", now, "expired").Updates(map[string]any{"changes": "", "summary": "", "result": "", "state": "expired"}).Error
}
func (c Context) Label() string {
	return fmt.Sprintf("Контекст: %s → %s", clip(c.Home, 25), clip(c.Room, 25))
}
