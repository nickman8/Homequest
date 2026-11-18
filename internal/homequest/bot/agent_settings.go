package bot

import (
	"errors"
	"time"

	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
	u "donetick.com/core/internal/user/model"
	"gorm.io/gorm"
)

// Only database changes and durable deliveries are allowed inside the preview savepoint.
func (b *Bot) agentLocalAction(tx *gorm.DB, p agent.Plan, v agent.Change, now time.Time) error {
	child := *b
	child.DB = tx
	child.Family = &family.Service{DB: tx}
	uid := p.Viewer
	user, e := child.Family.Authorize(uid, p.CircleID)
	if e != nil {
		return e
	}
	switch v.Operation {
	case "invite_create":
		token, e := child.Family.Invite(uid, now)
		if e != nil {
			return e
		}
		return child.say(uid, "Приглашение действует 7 дней:\nhttps://t.me/"+child.Username+"?start=join_"+token)
	case "task_handoff":
		offer, e := (&task.Service{DB: tx}).OfferHandoff(uid, p.CircleID, v.ID, v.Target, now)
		if e != nil {
			return e
		}
		return child.notifyHandoff(offer)
	case "task_snooze":
		var row Delivery
		if e := tx.Where("viewer=? AND circle_id=? AND instance_id=? AND sent=? AND notice=? AND version=?", uid, p.CircleID, v.ID, true, true, v.Version).Order("id DESC").First(&row).Error; e != nil {
			return e
		}
		return child.Snooze(uid, row.ID, time.Duration(v.Minutes)*time.Minute)
	case "reminder_offsets":
		policy, _, e := child.reminderPlan(p.CircleID, child.effectiveDefaults())
		if e != nil {
			return e
		}
		return child.SetReminderPlan(uid, p.CircleID, policy.Version, *v.Offsets)
	}
	pref := u.DefaultNotificationPreference(user.ID)
	if e = tx.First(pref, "user_id=?", user.ID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	rest := RestPreference{UserID: user.ID, QuietStart: 22, QuietEnd: 8}
	if e = tx.First(&rest, "user_id=?", user.ID).Error; e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	mute := pref.Profile == u.NotificationProfileQuiet
	switch v.Operation {
	case "reminder_mute", "reminder_unmute":
		mute = v.Operation == "reminder_mute"
	case "quiet_hours":
		rest.QuietStart, rest.QuietEnd = *v.QuietStart, *v.QuietEnd
	case "vacation":
		rest.VacationUntil = time.Time{}
		if v.Days > 0 {
			loc, e := time.LoadLocation(p.Timezone)
			if e != nil {
				return e
			}
			rest.VacationUntil = now.In(loc).AddDate(0, 0, v.Days).UTC()
		}
	default:
		return agent.ErrPlan
	}
	return child.SetRest(uid, mute, rest.QuietStart, rest.QuietEnd, rest.VacationUntil)
}
