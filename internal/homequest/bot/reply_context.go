package bot

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"donetick.com/core/internal/homequest/agent"
)

var errReplyAmbiguous = errors.New("В этой карточке несколько объектов. Откройте карточку нужного объекта или назовите его в отдельном сообщении.")

// Navigation page indexes are not entity IDs. Keep this allowlist explicit.
func replyResourceKind(kind string) (string, int) {
	p := strings.Split(kind, ":")
	if len(p) < 2 {
		return "", 0
	}
	id, e := strconv.Atoi(p[1])
	if e != nil || id <= 0 {
		return "", 0
	}
	switch p[0] {
	case "wish_info", "wish_edit", "wish_archive", "wish_archive_confirm", "wish_claim", "wish_purchase", "wish_release":
		return "wish", id
	case "shop_details", "shop_manage", "shop_edit", "shop_archive", "shop_change":
		return "shopping", id
	case "reward_request", "reward_manage", "reward_edit", "reward_archive":
		return "reward", id
	case "redemption":
		return "redemption", id
	case "room_rename", "room_delete", "room_delete_yes":
		return "room", id
	case "approve", "reject", "view_photo", "undo", "photo_attach":
		return "report", id
	case "handoff":
		return "handoff", id
	}
	return "", 0
}

func (b *Bot) bindReplyButton(a *Action) {
	kind, id := replyResourceKind(a.Kind)
	if a.InstanceID != 0 {
		kind, id = "task", a.InstanceID
	}
	if kind == "" {
		return
	}
	// A button may legitimately lead to a no-longer-actionable report. It remains
	// usable by its normal handler, but cannot become AI reply context.
	target, e := (&agent.Service{DB: b.DB}).CaptureReply(a.Viewer, a.CircleID, kind, id, b.Now())
	if e == nil {
		a.ReplyTarget = target
	}
}

func (b *Bot) replyTarget(uid int64, circle int) (agent.ReplyTarget, error) {
	var out agent.ReplyTarget
	if b.reply == nil || !b.reply.From.IsBot || !strings.EqualFold(b.reply.From.Username, b.Username) {
		return out, agent.ErrChanged
	}
	svc := &agent.Service{DB: b.DB}
	tokens, e := b.replyTokens(uid)
	if e != nil {
		return out, e
	}
	for _, data := range tokens {
		if data == "" {
			continue
		}
		if len(data) != 32 {
			return out, agent.ErrChanged
		}
		var a Action
		if b.DB.First(&a, "token=? AND viewer=? AND circle_id=? AND expires>? AND used=?", data, uid, circle, b.Now(), false).Error != nil {
			return out, agent.ErrChanged
		}
		target := a.ReplyTarget
		// Preserve support for task cards sent before this schema migration.
		if target.Kind == "" && a.InstanceID != 0 {
			target = agent.ReplyTarget{Kind: "task", ID: a.InstanceID, Version: a.Version}
		}
		if target.Kind == "" {
			continue
		}
		fresh, e := svc.CaptureReply(uid, circle, target.Kind, target.ID, b.Now())
		if e != nil {
			return out, e
		}
		if fresh != target {
			return out, agent.ErrChanged
		}
		if out.Kind != "" && out != target {
			return out, errReplyAmbiguous
		}
		out = target
	}
	if out.Kind == "" {
		return out, agent.ErrChanged
	}
	return out, nil
}

// Compatibility for existing task-only callers; resource cards use replyTarget.
func (b *Bot) replyTask(uid int64, circle int) (int, int, error) {
	target, e := b.replyTarget(uid, circle)
	if e != nil {
		return 0, 0, e
	}
	if target.Kind != "task" {
		return 0, 0, agent.ErrChanged
	}
	return target.ID, target.Version, nil
}

// Telegram keeps the original inline keyboard when widget chat_data changes.
// Resolve an edited widget from the last *confirmed delivered* server state,
// not from that obsolete keyboard. Pending/failed/skipped edits are ambiguous.
func (b *Bot) replyTokens(uid int64) ([]string, error) {
	tokens := []string{}
	if b.reply.MessageID > 0 {
		var updates []Delivery
		if e := b.DB.Where("viewer=? AND method=? AND created_at>?", uid, "editMessageWidgetData", b.Now().Add(-24*time.Hour)).Order("id DESC").Find(&updates).Error; e != nil {
			return nil, e
		}
		for _, row := range updates {
			var payload struct {
				MessageID int64  `json:"message_id"`
				ChatData  string `json:"chat_data"`
			}
			if json.Unmarshal([]byte(row.Payload), &payload) != nil {
				return nil, agent.ErrChanged
			}
			if payload.MessageID != b.reply.MessageID {
				continue
			}
			var view CardView
			if !row.Sent || b.DB.First(&view, "viewer=? AND message_id=? AND delivery_id=?", uid, b.reply.MessageID, row.ID).Error != nil {
				return nil, agent.ErrChanged
			}
			var card Card
			if json.Unmarshal([]byte(payload.ChatData), &card) != nil {
				return nil, agent.ErrChanged
			}
			for _, button := range card.Buttons {
				tokens = append(tokens, button.Action)
			}
			return tokens, nil
		}
	}
	for _, row := range b.reply.Markup.Rows {
		for _, button := range row {
			tokens = append(tokens, button.Data)
		}
	}
	return tokens, nil
}
