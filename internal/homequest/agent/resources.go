package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/shopping"
)

// Resource is an explicit model projection. No Telegram identity, receipt key,
// family ID, media URL or surprise-gift information enters tool output.
type Resource struct {
	ID          int    `json:"id"`
	Kind        string `json:"kind"`
	Snapshot    string `json:"snapshot"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Quantity    string `json:"quantity,omitempty"`
	Points      int    `json:"points,omitempty"`
	Status      string `json:"status"`
	CreatedBy   int    `json:"created_by,omitempty"`
	ClaimedBy   int    `json:"claimed_by,omitempty"`
	BoughtBy    int    `json:"bought_by,omitempty"`
	Requester   int    `json:"requester,omitempty"`
}

func snapshot(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func resourceTarget(kind string) bool {
	return kind == "shopping" || kind == "reward" || kind == "redemption"
}
func (s *Service) Resources(c Context, kind string, now time.Time) ([]Resource, error) {
	if e := s.Check(c, now); e != nil {
		return nil, e
	}
	return s.resources(c.Viewer, c.CircleID, kind)
}
func (s *Service) resources(uid int64, circle int, kind string) ([]Resource, error) {
	out := []Resource{}
	switch kind {
	case "shopping":
		sh := &shopping.Service{DB: s.DB}
		for _, bought := range []bool{false, true} {
			rows, e := sh.List(uid, circle, bought)
			if e != nil {
				return nil, e
			}
			for _, r := range rows {
				out = append(out, Resource{ID: r.ID, Kind: kind, Snapshot: snapshot(r), Name: r.Name, Description: r.Note, Quantity: r.Quantity, Status: r.Status, CreatedBy: r.CreatedBy, ClaimedBy: r.ClaimedBy, BoughtBy: r.BoughtBy})
			}
		}
	case "reward":
		rows, e := (&reward.Service{DB: s.DB}).List(uid, circle)
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			out = append(out, Resource{ID: r.ID, Kind: kind, Snapshot: snapshot(r), Name: r.Name, Description: r.Description, Points: r.PointsCost, Status: "active", CreatedBy: r.CreatedBy})
		}
	case "redemption":
		rows, e := (&reward.Service{DB: s.DB}).Requests(uid, circle)
		if e != nil {
			return nil, e
		}
		labels := map[int]string{0: "pending", 1: "approved", 2: "rejected", 3: "delivered", 4: "cancelled"}
		for _, r := range rows {
			out = append(out, Resource{ID: r.ID, Kind: kind, Snapshot: snapshot(r), Name: r.RewardName, Points: r.PointsCost, Status: labels[int(r.Status)], Requester: r.UserID})
		}
	default:
		return nil, ErrPlan
	}
	return out, nil
}
func (s *Service) resource(p Plan, v Change) (Resource, error) {
	cap, ok := capability(v.Operation)
	if !ok || !resourceTarget(cap.Target) {
		return Resource{}, ErrPlan
	}
	rows, e := s.resources(p.Viewer, p.CircleID, cap.Target)
	if e != nil {
		return Resource{}, e
	}
	for _, r := range rows {
		if r.ID == v.ID {
			return r, nil
		}
	}
	return Resource{}, ErrChanged
}
func (s *Service) executeResource(p Plan, v Change, index int, now time.Time) (Effect, error) {
	out := Effect{Operation: v.Operation, ID: v.ID, Status: "updated"}
	rw := &reward.Service{DB: s.DB}
	var e error
	switch v.Operation {
	case "reward_create":
		r, err := rw.Create(p.Viewer, p.CircleID, v.Name, v.Description, v.Points)
		e = err
		out.ID = r.ID
		out.Status = "created"
	case "reward_edit":
		e = rw.Edit(p.Viewer, p.CircleID, v.ID, v.Name, v.Points, v.Description)
	case "reward_archive":
		e = rw.Archive(p.Viewer, p.CircleID, v.ID)
	case "reward_request":
		r, err := rw.Request(p.Viewer, p.CircleID, v.ID, fmt.Sprintf("agent:%s:%d", p.ID, index), now)
		e = err
		out.ID = r.ID
		out.Status = "pending"
	case "redemption_approve", "redemption_deliver", "redemption_reject", "redemption_cancel":
		e = rw.Resolve(p.Viewer, p.CircleID, v.ID, strings.TrimPrefix(v.Operation, "redemption_"), now)
	case "shopping_edit", "shopping_claim", "shopping_release", "shopping_buy", "shopping_reopen", "shopping_archive":
		var row shopping.Item
		if e = s.DB.First(&row, "id=? AND circle_id=?", v.ID, p.CircleID).Error; e != nil {
			return out, e
		}
		var value *shopping.Input
		if v.Operation == "shopping_edit" {
			if len(v.Items) != 1 {
				return out, ErrPlan
			}
			value = &v.Items[0]
		}
		e = (&shopping.Service{DB: s.DB}).Change(p.Viewer, p.CircleID, v.ID, row.Version, strings.TrimPrefix(v.Operation, "shopping_"), value, now)
	default:
		return out, ErrPlan
	}
	return out, e
}
func (s *Service) describeResource(p Plan, v Change) (string, error) {
	if v.Operation == "reward_create" {
		return fmt.Sprintf("Добавить награду «%s» · %d баллов · %s", v.Name, v.Points, v.Description), nil
	}
	r, e := s.resource(p, v)
	if e != nil {
		return "", e
	}
	label := map[string]string{"shopping_claim": "Взять покупку на себя", "shopping_release": "Освободить покупку", "shopping_buy": "Отметить купленным", "shopping_reopen": "Вернуть в список покупок", "shopping_archive": "Убрать из списка покупок", "shopping_edit": "Изменить покупку", "reward_edit": "Изменить награду", "reward_archive": "Убрать награду из каталога", "reward_request": "Запросить награду", "redemption_approve": "Принять заявку", "redemption_deliver": "Отметить выдачу награды", "redemption_reject": "Отклонить заявку", "redemption_cancel": "Отменить заявку"}[v.Operation]
	text := label + ": «" + r.Name + "»"
	switch v.Operation {
	case "shopping_edit":
		if len(v.Items) != 1 {
			return "", ErrPlan
		}
		i := v.Items[0]
		text += " → " + strings.TrimSpace(i.Name+" "+i.Quantity+" "+i.Note)
	case "reward_edit":
		text += fmt.Sprintf(" → «%s» · %d баллов · %s", v.Name, v.Points, v.Description)
	case "reward_request":
		text += fmt.Sprintf(" · списать %d баллов с вашего баланса", r.Points)
	case "redemption_reject", "redemption_cancel":
		text += fmt.Sprintf(" · вернуть %d баллов заявителю", r.Points)
	case "redemption_approve", "redemption_deliver":
		text += fmt.Sprintf(" · %d баллов (уже списаны)", r.Points)
	}
	if r.Requester != 0 {
		// Different family members can request the same reward: name alone is ambiguous.
		c, e := (&family.Service{DB: s.DB}).View(p.Viewer, p.CircleID)
		if e != nil {
			return "", e
		}
		name := "участник вышел из дома"
		for _, m := range c.Members {
			if m.ID == r.Requester {
				name = m.Name
			}
		}
		text += fmt.Sprintf(" · заявка №%d · %s", r.ID, name)
	}
	return text, nil
}
