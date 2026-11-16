package bot

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/billing"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

type AIConsent struct {
	Provider string
	UserID   int64 `gorm:"primaryKey;autoIncrement:false"`
	Accepted bool
}

func (AIConsent) TableName() string { return "hq_ai_consents" }

type AIJob struct {
	Provider   string
	ID         uint `gorm:"primaryKey"`
	Viewer     int64
	CircleID   int
	Text       string
	VoiceID    string
	State      string `gorm:"index"`
	Command    string
	Candidates string
	CreatedAt  time.Time
}

func (AIJob) TableName() string { return "hq_ai_jobs" }

type VoiceSource interface {
	Voice(context.Context, string) ([]byte, error)
}

func (b *Bot) enqueueAI(uid int64, text, voice string) error {
	if b.LLM == nil && voice == "" {
		return b.say(uid, "Помощник пока недоступен: доступ к выбранной модели не подтверждён. Задания можно выполнять кнопками.")
	}
	if voice != "" && (b.STT == nil || b.VoiceSource == nil) {
		return b.say(uid, "Распознавание голоса пока недоступно. Используйте текст или кнопки.")
	}
	h, e := b.Family.Current(uid)
	if e != nil {
		return b.home(uid, 0)
	}
	var pending int64
	if e = b.DB.Model(&AIJob{}).Where("state IN ?", []string{"consent", "queued", "processing", "confirm"}).Count(&pending).Error; e != nil {
		return e
	}
	if pending >= 16 {
		return b.say(uid, "Помощник занят. Попробуйте позже или воспользуйтесь кнопками.")
	}
	limit := b.AIDailyLimit
	if limit <= 0 {
		limit = 100
	}
	if active, _ := (&billing.Service{DB: b.DB}).Active(uid, h.ID, b.Now()); active {
		limit = b.AIPremiumLimit
		if limit <= 0 {
			limit = 500
		}
	}
	var used int64
	if e = b.DB.Model(&AIJob{}).Where("viewer = ? AND created_at > ?", uid, b.Now().Add(-24*time.Hour)).Count(&used).Error; e != nil {
		return e
	}
	if used >= int64(limit) {
		return b.say(uid, "Лимит помощника на сутки достигнут. Обычные задания доступны.")
	}
	var consent AIConsent
	_ = b.DB.First(&consent, "user_id = ?", uid).Error
	state := "queued"
	if !consent.Accepted || consent.Provider != b.AIProvider {
		state = "consent"
	}
	job := AIJob{Provider: b.AIProvider, Viewer: uid, CircleID: h.ID, Text: text, VoiceID: voice, State: state, CreatedAt: b.Now()}
	if e = b.DB.Create(&job).Error; e != nil {
		return e
	}
	if state == "consent" {
		return b.card(uid, 0, "Помощник по дому", "Голос распознаётся локально. Если VyceAI доступен, текст и названия заданий передаются ему для понимания. Продолжить?", h.ID, fmt.Sprintf("ai_consent:%d", job.ID), "Понятно, продолжить", fmt.Sprintf("ai_cancel:%d", job.ID), "Отмена")
	}
	return b.say(uid, "Обрабатываю сообщение. Обычные кнопки доступны.")
}
func (b *Bot) aiAction(uid, msg int64, a Action) (bool, error) {
	p := strings.Split(a.Kind, ":")
	if !strings.HasPrefix(p[0], "ai_") {
		return false, nil
	}
	if len(p) != 2 {
		return true, b.home(uid, msg)
	}
	id, e := strconv.Atoi(p[1])
	if e != nil {
		return true, b.home(uid, msg)
	}
	var job AIJob
	if b.DB.First(&job, "id = ? AND viewer = ? AND circle_id = ?", id, uid, a.CircleID).Error != nil {
		return true, b.say(uid, "Запрос недоступен")
	}
	if b.Now().Sub(job.CreatedAt) > 15*time.Minute {
		return true, b.say(uid, "Запрос устарел. Отправьте его заново.")
	}
	switch p[0] {
	case "ai_cancel":
		return true, b.DB.Model(&job).Updates(map[string]any{"state": "cancelled", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error
	case "ai_consent":
		if job.State != "consent" {
			return true, b.home(uid, msg)
		}
		if job.Provider != b.AIProvider {
			return true, b.say(uid, "Помощник обновился. Отправьте сообщение заново, чтобы подтвердить обработку текста.")
		}
		if e = b.DB.Save(&AIConsent{UserID: uid, Accepted: true, Provider: b.AIProvider}).Error; e != nil {
			return true, e
		}
		if e = b.DB.Model(&job).Update("state", "queued").Error; e != nil {
			return true, e
		}
		return true, b.say(uid, "Обрабатываю сообщение.")
	case "ai_confirm":
		if job.State != "confirm" {
			return true, b.home(uid, msg)
		}
		return true, b.applyAI(&job, true)
	}
	return true, b.home(uid, msg)
}
func (b *Bot) ProcessAI(ctx context.Context) error {
	// Text/file references are transient. Keep only audit state after expiration.
	if e := b.DB.Model(&AIJob{}).Where("created_at < ? AND state NOT IN ?", b.Now().Add(-15*time.Minute), []string{"done", "failed", "cancelled", "expired"}).Updates(map[string]any{"text": "", "voice_id": "", "command": "", "candidates": "", "state": "expired"}).Error; e != nil {
		return e
	}
	var job AIJob
	e := b.DB.Where("state = ?", "queued").Order("id").First(&job).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	result := b.DB.Model(&job).Where("state = ?", "queued").Update("state", "processing")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return nil
	}
	fail := func() error {
		return b.DB.Transaction(func(tx *gorm.DB) error {
			child := *b
			child.DB = tx
			child.Family = &family.Service{DB: tx}
			if e := tx.Model(&job).Updates(map[string]any{"state": "failed", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error; e != nil {
				return e
			}
			if _, e := child.Family.Authorize(job.Viewer, job.CircleID); e != nil {
				return nil
			}
			return child.say(job.Viewer, "Помощник не смог безопасно обработать сообщение. Ничего не изменено; используйте кнопки или уточните фразу.")
		})
	}
	if job.Provider != b.AIProvider {
		return fail()
	}
	home, e := b.Family.View(job.Viewer, job.CircleID)
	if e != nil {
		return fail()
	}
	if job.VoiceID != "" {
		if b.STT == nil || b.VoiceSource == nil {
			return fail()
		}
		audio, e := b.VoiceSource.Voice(ctx, job.VoiceID)
		if e != nil {
			return fail()
		}
		job.Text, e = b.STT.Transcribe(ctx, audio)
		if e != nil {
			return fail()
		}
	}

	if b.LLM == nil {
		return b.DB.Transaction(func(tx *gorm.DB) error {
			child := *b
			child.DB = tx
			child.Family = &family.Service{DB: tx}
			if e := tx.Model(&job).Updates(map[string]any{"state": "done", "text": "", "voice_id": ""}).Error; e != nil {
				return e
			}
			if _, e := child.Family.Authorize(job.Viewer, job.CircleID); e != nil {
				return nil
			}
			return child.say(job.Viewer, "Распознано локально: "+short(job.Text, 1000)+"\nVyceAI пока недоступен: задания не изменены. Проверьте текст и используйте кнопки.")
		})
	}
	if len(job.Text) > 4096 {
		return fail()
	}
	user, e := b.Family.Authorize(job.Viewer, job.CircleID)
	if e != nil {
		return fail()
	}
	items, e := (&task.Service{DB: b.DB}).List(job.Viewer, job.CircleID)
	if e != nil {
		return fail()
	}
	candidates := []assistant.Candidate{}
	for _, item := range items {
		if item.AssignedTo != nil && *item.AssignedTo != user.ID {
			continue
		}
		var instance task.Instance
		if b.DB.First(&instance, item.ID).Error != nil {
			return fail()
		}
		candidates = append(candidates, assistant.Candidate{ID: item.ID, Title: item.Name, Version: instance.Version})
	}
	command, e := b.LLM.Parse(ctx, job.Text, candidates, home.Timezone)
	if e != nil {
		return fail()
	}
	if e = command.Validate(job.Text, candidates); e != nil {
		return fail()
	}
	raw, _ := json.Marshal(command)
	source, _ := json.Marshal(candidates)
	job.Command = string(raw)
	job.Candidates = string(source)
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var current AIJob
		if e := tx.First(&current, job.ID).Error; e != nil {
			return e
		}
		if current.State != "processing" {
			return nil
		}
		child := *b
		child.DB = tx
		child.Family = &family.Service{DB: tx}
		if _, e = child.Family.Authorize(job.Viewer, job.CircleID); e != nil {
			return tx.Model(&job).Updates(map[string]any{"state": "failed", "text": "", "voice_id": ""}).Error
		}
		if e = tx.Model(&job).Updates(map[string]any{"text": job.Text, "command": job.Command, "candidates": job.Candidates, "voice_id": ""}).Error; e != nil {
			return e
		}
		if command.Action == "complete" {
			return child.applyAI(&job, false)
		}
		if command.Action == "draft" || command.Action == "confirm_multiple" {
			if e = tx.Model(&job).Update("state", "confirm").Error; e != nil {
				return e
			}
			text := ""
			if command.Draft != nil {
				d := command.Draft
				text = fmt.Sprintf("%s\n%d баллов\n%s · %02d:%02d · %s", short(d.Title, 45), d.Points, d.Schedule.Mode, d.Schedule.Hour, d.Schedule.Minute, home.Timezone)
				if len(d.Schedule.Days) > 0 {
					text += "\n" + strings.Join(d.Schedule.Days, ",")
				}
			} else {
				for _, id := range command.IDs {
					for _, c := range candidates {
						if c.ID == id {
							text += short(c.Title, 35) + "\n"
						}
					}
				}
			}
			return child.card(job.Viewer, 0, "Проверьте перед действием", text, job.CircleID, fmt.Sprintf("ai_confirm:%d", job.ID), "Подтвердить", fmt.Sprintf("ai_cancel:%d", job.ID), "Отмена")
		}
		if e = tx.Model(&job).Updates(map[string]any{"state": "done", "text": "", "command": "", "candidates": ""}).Error; e != nil {
			return e
		}
		return child.say(job.Viewer, "Действие не выполнено. Уточните, что уже сделано, или выберите задание кнопкой.")
	})
	if err != nil {
		return fail()
	}
	return nil
}
func (b *Bot) applyAI(job *AIJob, confirmed bool) error {
	var candidates []assistant.Candidate
	if json.Unmarshal([]byte(job.Candidates), &candidates) != nil {
		return errors.New("invalid AI snapshot")
	}
	command, e := assistant.Decode(job.Command, job.Text, candidates)
	if e != nil {
		return b.say(job.Viewer, "Нужно уточнение; действие не выполнено")
	}
	if (command.Action == "draft" || command.Action == "confirm_multiple") && !confirmed {
		return errors.New("confirmation required")
	}
	if _, e = b.Family.Authorize(job.Viewer, job.CircleID); e != nil {
		return e
	}
	// Validate every instance before any mutation, including the saved version.
	for _, id := range command.IDs {
		version := 0
		for _, c := range candidates {
			if c.ID == id {
				version = c.Version
			}
		}
		var i task.Instance
		if b.DB.First(&i, "id = ? AND circle_id = ? AND version = ? AND closed = ? AND pending = ?", id, job.CircleID, version, false, false).Error != nil {
			if e = b.DB.Model(job).Updates(map[string]any{"state": "failed", "text": "", "command": "", "candidates": ""}).Error; e != nil {
				return e
			}
			return b.say(job.Viewer, "Задание уже изменилось или выполнено. Откройте свежую карточку.")
		}
	}
	s := &task.Service{DB: b.DB}
	if command.Draft != nil {
		d := command.Draft
		item, e := s.Create(job.Viewer, job.CircleID, d.Title, d.Points, nil)
		if e != nil {
			return e
		}
		if d.Schedule.Mode != "once" {
			if e = s.SetSchedule(job.Viewer, job.CircleID, item.ID, d.Schedule, b.Now()); e != nil {
				return e
			}
		}
	} else {
		for _, id := range command.IDs {
			attempt, e := s.Submit(job.Viewer, job.CircleID, id, "", b.Now())
			if e != nil {
				return e
			}
			if e = b.submitted(job.Viewer, 0, job.CircleID, attempt); e != nil {
				return e
			}
		}
	}
	if e = b.DB.Model(job).Updates(map[string]any{"state": "done", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error; e != nil {
		return e
	}
	if command.Draft != nil {
		return b.tasks(job.Viewer, 0, 0)
	}
	return b.awardBadges(job.Viewer)
}
