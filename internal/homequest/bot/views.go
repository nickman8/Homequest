package bot

import (
	"encoding/json"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

// Only the most recently delivered widget is refreshed, so navigation is not
// replaced by an old list. No personal widget state is shared between viewers.
type CardView struct {
	Viewer     int64 `gorm:"primaryKey;autoIncrement:false"`
	CircleID   int
	MessageID  int64
	DeliveryID uint
	Screen     string
}

func (CardView) TableName() string          { return "hq_card_views" }
func (b *Bot) screenFor(screen string) *Bot { child := *b; child.screen = screen; return &child }
func (b *Bot) rememberView(tx *gorm.DB, row Delivery, payload map[string]any, reply json.RawMessage) error {
	if row.Method != "sendWidget" && row.Method != "editMessageWidgetData" {
		return nil
	}
	var message int64
	if row.Method == "sendWidget" {
		var result struct {
			ID int64 `json:"message_id"`
		}
		if json.Unmarshal(reply, &result) != nil {
			return nil
		}
		message = result.ID
	} else if n, ok := payload["message_id"].(float64); ok {
		message = int64(n)
	}
	if message == 0 {
		return nil
	}
	var previous CardView
	if e := tx.First(&previous, "viewer=?", row.Viewer).Error; e != nil && e != gorm.ErrRecordNotFound {
		return e
	}
	if previous.DeliveryID > row.ID {
		return nil
	}
	return tx.Save(&CardView{Viewer: row.Viewer, CircleID: row.CircleID, MessageID: message, DeliveryID: row.ID, Screen: row.Screen}).Error
}
func (b *Bot) refreshFamilyViews(uid int64, circle int) error {
	if b.UIMode == "buttons" {
		return nil
	}
	child := *b
	child.autoRefresh = true
	b = &child
	var rows []CardView
	if e := b.DB.Where("circle_id=? AND viewer<>? AND screen<>?", circle, uid, "").Find(&rows).Error; e != nil {
		return e
	}
	for _, v := range rows {
		if _, e := b.Family.Authorize(v.Viewer, circle); e != nil {
			continue
		}
		parts := strings.Split(v.Screen, ":")
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		var e error
		switch parts[0] {
		case "tasks":
			e = b.tasks(v.Viewer, v.MessageID, page)
		case "home":
			e = b.home(v.Viewer, v.MessageID)
		case "points":
			e = b.points(v.Viewer, v.MessageID, circle)
		case "ranking":
			e = b.ranking(v.Viewer, v.MessageID, circle, page)
		case "house":
			e = b.house(v.Viewer, v.MessageID, circle)
		case "badges":
			e = b.badges(v.Viewer, v.MessageID, circle)
		}
		if e != nil {
			return e
		}
	}
	return nil
}
