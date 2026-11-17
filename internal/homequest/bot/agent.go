package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	"gorm.io/gorm"
)

type agentAccess struct {
	bot   *Bot
	job   *AIJob
	scope agent.Context
}

func (a agentAccess) Tasks(room int) ([]agent.Task, error) {
	var job AIJob
	if a.bot.DB.First(&job, "id=? AND state=?", a.job.ID, "processing").Error != nil {
		return nil, agent.ErrPlan
	}
	return (&agent.Service{DB: a.bot.DB}).Tasks(a.scope, room, a.bot.Now())
}
func (a agentAccess) Resources(kind string) ([]agent.Resource, error) {
	var job AIJob
	if a.bot.DB.First(&job, "id=? AND state=?", a.job.ID, "processing").Error != nil {
		return nil, agent.ErrPlan
	}
	return (&agent.Service{DB: a.bot.DB}).Resources(a.scope, kind, a.bot.Now())
}
func (a agentAccess) Account() (agent.Account, error) {
	var job AIJob
	if a.bot.DB.First(&job, "id=? AND state=?", a.job.ID, "processing").Error != nil {
		return agent.Account{}, agent.ErrPlan
	}
	return (&agent.Service{DB: a.bot.DB}).Account(a.scope, a.bot.Now())
}
func (b *Bot) agentHome(uid, msg int64) error {
	c, e := (&agent.Service{DB: b.DB}).Context(uid, b.Now())
	if e != nil {
		return b.home(uid, msg)
	}
	text := c.Label() + "\nЗадания, сроки, повторения, покупки и награды — текстом или голосом. Изменения сначала покажу для проверки."
	if b.Agent == nil {
		text = c.Label() + "\nПолный режим агента пока выключен. Доступен прежний помощник для выполнения и создания заданий."
	}
	return b.card(uid, msg, "Помощник по дому", text, c.CircleID, "rooms:0", "Выбрать комнату", "agent_reset", "Сбросить контекст", "agent_stop", "Остановить запрос", "home", "Дом")
}
func (b *Bot) stopAgent(uid int64) error {
	if e := b.DB.Model(&AIJob{}).Where("viewer=? AND agent=? AND state IN ?", uid, true, []string{"queued", "processing", "confirm", "consent"}).Updates(map[string]any{"state": "cancelled", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error; e != nil {
		return e
	}
	if e := b.DB.Model(&agent.Plan{}).Where("viewer=? AND state=?", uid, "pending").Updates(map[string]any{"state": "cancelled", "changes": "", "summary": ""}).Error; e != nil {
		return e
	}
	return b.say(uid, "Запрос остановлен. Уже подтверждённые изменения сохранены.")
}
func (b *Bot) agentAction(uid, msg int64, a Action) (bool, error) {
	switch a.Kind {
	case "agent":
		return true, b.agentHome(uid, msg)
	case "agent_reset":
		if e := b.stopAgent(uid); e != nil {
			return true, e
		}
		if e := (&agent.Service{DB: b.DB}).Reset(uid, b.Now()); e != nil {
			return true, e
		}
		return true, b.agentHome(uid, msg)
	case "agent_stop":
		return true, b.stopAgent(uid)
	}
	if strings.HasPrefix(a.Kind, "agent_page:") {
		p := strings.Split(a.Kind, ":")
		if len(p) != 3 {
			return true, agent.ErrPlan
		}
		id, e := strconv.Atoi(p[1])
		if e != nil {
			return true, e
		}
		page, e := strconv.Atoi(p[2])
		if e != nil {
			return true, e
		}
		var job AIJob
		if b.DB.First(&job, "id=? AND viewer=? AND circle_id=? AND state=?", id, uid, a.CircleID, "confirm").Error != nil {
			return true, b.say(uid, "План уже закрыт")
		}
		return true, b.agentPreview(&job, msg, page)
	}
	return false, nil
}
func (b *Bot) jobScope(job *AIJob) (agent.Context, error) {
	c, e := (&agent.Service{DB: b.DB}).Context(job.Viewer, b.Now())
	if e != nil {
		return c, e
	}
	if c.CircleID != job.CircleID || c.RoomID != job.RoomID || c.Revision != job.ContextRevision || c.Timezone != job.Timezone {
		return c, agent.ErrChanged
	}
	return c, nil
}
func (b *Bot) processAgent(ctx context.Context, job *AIJob) error {
	inputText := job.Text
	scope, e := b.jobScope(job)
	if e != nil {
		return e
	}
	// Confirmation is interpreted by the server and bound at enqueue time, never by the model.
	phrase := strings.Trim(strings.ToLower(strings.TrimSpace(job.Text)), ".! ")
	if phrase == "да" || phrase == "подтверждаю" || phrase == "нет" || phrase == "отмена" {
		return b.DB.Transaction(func(tx *gorm.DB) error {
			child := *b
			child.DB = tx
			child.Family = &family.Service{DB: tx}
			var current AIJob
			if tx.First(&current, "id=? AND state=?", job.ID, "processing").Error != nil {
				return nil
			}
			if _, e := child.jobScope(job); e != nil {
				return e
			}
			if e := tx.Model(job).Updates(map[string]any{"state": "done", "text": "", "voice_id": ""}).Error; e != nil {
				return e
			}
			var target AIJob
			if job.ConfirmPlan == "" || tx.First(&target, "viewer=? AND plan_id=? AND state=?", job.Viewer, job.ConfirmPlan, "confirm").Error != nil {
				return child.say(job.Viewer, "Нет актуального плана для подтверждения. Опишите действие или откройте его карточку.")
			}
			if phrase == "нет" || phrase == "отмена" {
				if e := (&agent.Service{DB: tx}).Cancel(job.Viewer, job.ConfirmPlan, b.Now()); e != nil {
					return e
				}
				if e := tx.Model(&target).Update("state", "cancelled").Error; e != nil {
					return e
				}
				return child.say(job.Viewer, "План отменён.")
			}
			return child.applyAgent(&target)
		})
	}
	if b.Agent == nil {
		return agent.ErrPlan
	}
	// A new substantive request ends the older pending intent. Otherwise a later
	// "yes" to a clarification could accidentally approve an unrelated old plan.
	active := false
	if e = b.DB.Transaction(func(tx *gorm.DB) error {
		var current AIJob
		if tx.First(&current, "id=? AND state=?", job.ID, "processing").Error != nil {
			return nil
		}
		if e := (&agent.Service{DB: tx}).Check(scope, b.Now()); e != nil {
			return e
		}
		if e := tx.Model(&agent.Plan{}).Where("viewer=? AND state=?", job.Viewer, "pending").Updates(map[string]any{"state": "superseded", "changes": "", "summary": ""}).Error; e != nil {
			return e
		}
		if e := tx.Model(&AIJob{}).Where("viewer=? AND agent=? AND state=?", job.Viewer, true, "confirm").Updates(map[string]any{"state": "cancelled", "text": "", "voice_id": ""}).Error; e != nil {
			return e
		}
		active = true
		return nil
	}); e != nil {
		return e
	}
	if !active {
		return nil
	}
	result, e := b.Agent.Run(ctx, scope, job.Text, agentAccess{b, job, scope})
	if e != nil {
		return e
	}
	return b.DB.Transaction(func(tx *gorm.DB) error {
		var current AIJob
		if tx.First(&current, "id=? AND state=?", job.ID, "processing").Error != nil {
			return nil
		}
		child := *b
		child.DB = tx
		child.Family = &family.Service{DB: tx}
		svc := &agent.Service{DB: tx}
		if e = svc.Check(scope, b.Now()); e != nil {
			return e
		}
		if result.Room != nil {
			if len(result.Changes) > 0 || result.Reply != "" {
				return agent.ErrPlan
			}
			if _, e = svc.SelectRoom(job.Viewer, *result.Room, b.Now()); e != nil {
				return e
			}
			if e = tx.Model(job).Updates(map[string]any{"state": "done", "text": "", "voice_id": ""}).Error; e != nil {
				return e
			}
			return child.agentHome(job.Viewer, 0)
		}
		if len(result.Changes) > 0 {
			if e = agent.ValidateIntent(inputText, result.Changes); e != nil {
				return e
			}
			p, e := svc.Prepare(scope, fmt.Sprintf("job:%d", job.ID), result.Changes, b.Now())
			if e != nil {
				return e
			}
			job.PlanID = p.ID
			if e = tx.Model(job).Updates(map[string]any{"plan_id": p.ID, "state": "confirm", "text": "", "voice_id": ""}).Error; e != nil {
				return e
			}
			if e = svc.Remember(scope, result.References, inputText, "Подготовлен план; ещё не применён: "+p.Summary, b.Now()); e != nil {
				return e
			}
			return child.agentPreview(job, 0, 0)
		}
		if strings.TrimSpace(result.Reply) == "" || len([]rune(result.Reply)) > 1600 {
			return agent.ErrPlan
		}
		if e = svc.Remember(scope, result.References, inputText, result.Reply, b.Now()); e != nil {
			return e
		}
		if e = tx.Model(job).Updates(map[string]any{"state": "done", "text": "", "voice_id": ""}).Error; e != nil {
			return e
		}
		return child.say(job.Viewer, scope.Label()+"\n"+result.Reply+"\nИзменений не внесено.")
	})
}
func (b *Bot) agentPreview(job *AIJob, msg int64, page int) error {
	var p agent.Plan
	if b.DB.First(&p, "id=? AND viewer=? AND state=? AND expires>?", job.PlanID, job.Viewer, "pending", b.Now()).Error != nil {
		return b.say(job.Viewer, "План уже закрыт или устарел")
	}
	c, e := b.jobScope(job)
	if e != nil {
		return b.say(job.Viewer, "Контекст изменился. Отправьте просьбу заново.")
	}
	// Small pages use the existing native-theme/Open Props/Lucide card template.
	parts := []string{}
	for _, line := range strings.Split(p.Summary, "\n") {
		chars := []rune(line)
		for len(chars) > 160 {
			end := 160
			for n := 159; n > 100; n-- {
				if chars[n] == ' ' {
					end = n
					break
				}
			}
			parts = append(parts, string(chars[:end]))
			chars = []rune(strings.TrimSpace(string(chars[end:])))
		}
		if len(chars) > 0 {
			parts = append(parts, string(chars))
		}
	}
	pages := len(parts)
	if page < 0 || page >= pages {
		return agent.ErrPlan
	}
	text := c.Label() + fmt.Sprintf("\nЧасть плана: %d из %d\n", page+1, pages) + parts[page]
	opts := []string{}
	if page > 0 {
		opts = append(opts, fmt.Sprintf("agent_page:%d:%d", job.ID, page-1), "Предыдущая часть")
	}
	if page+1 < pages {
		opts = append(opts, fmt.Sprintf("agent_page:%d:%d", job.ID, page+1), "Следующая часть")
	} else {
		opts = append(opts, fmt.Sprintf("ai_confirm:%d", job.ID), "Подтвердить план")
	}
	opts = append(opts, fmt.Sprintf("ai_cancel:%d", job.ID), "Отмена")
	return b.card(job.Viewer, msg, "Проверьте изменения", text, job.CircleID, opts...)
}
func (b *Bot) applyAgent(job *AIJob) error {
	// Called inside the update/confirmation transaction: effects, job and deliveries commit together.
	svc := &agent.Service{DB: b.DB}
	effects, e := svc.Apply(job.Viewer, job.PlanID, b.Now())
	if e != nil {
		return b.say(job.Viewer, "План больше нельзя применить: данные или права изменились. Отправьте просьбу заново.")
	}
	if e = b.DB.Model(job).Updates(map[string]any{"state": "done", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error; e != nil {
		return e
	}
	photos := 0
	for _, effect := range effects {
		if effect.Status == "photo" {
			photos++
		}
	}
	if photos > 1 {
		if e = b.Family.SetStep(job.Viewer, "", ""); e != nil {
			return e
		}
	}
	refs := []int{}
	for _, effect := range effects {
		if strings.HasPrefix(effect.Operation, "task_") {
			refs = append(refs, effect.ID)
		}
		if effect.AttemptID != 0 {
			var a task.Attempt
			if e = b.DB.First(&a, effect.AttemptID).Error; e != nil {
				return e
			}
			if a.Status == "photo" && photos > 1 {
				continue
			}
			if e = b.submitted(job.Viewer, 0, job.CircleID, a); e != nil {
				return e
			}
		}
	}
	c, e := svc.Context(job.Viewer, b.Now())
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(effects)
	if e = svc.Remember(c, refs, "Подтверждение плана", "Применён: "+string(raw), b.Now()); e != nil {
		return e
	}
	if e = b.refreshFamilyViews(job.Viewer, job.CircleID); e != nil {
		return e
	}
	if e = b.awardBadges(job.Viewer); e != nil {
		return e
	}
	if photos > 1 {
		return b.card(job.Viewer, 0, "План применён", fmt.Sprintf("Изменений: %d.\nОтчётов без фото: %d. Откройте «Отчёты» и выберите задание перед отправкой фотографии.", len(effects), photos), job.CircleID, "reports", "Отчёты", "tasks", "Сегодня", "agent", "Помощник")
	}
	return b.card(job.Viewer, 0, "План применён", fmt.Sprintf("Изменений: %d.\n%s\nДля выполнения с фото или проверкой следуйте сообщению отчёта.", len(effects), c.Label()), job.CircleID, "tasks", "Сегодня", "reports", "Отчёты", "agent", "Помощник")
}
