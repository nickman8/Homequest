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
	"donetick.com/core/internal/homequest/task"
	"donetick.com/core/internal/homequest/wishlist"
)

// Resource is an explicit model projection. No Telegram identity, receipt key,
// family ID, media URL or surprise-gift information enters tool output.
type Resource struct {
	HasPhoto    bool   `json:"has_photo,omitempty"`
	Link        string `json:"link,omitempty"`
	Price       string `json:"price,omitempty"`
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
	return kind == "handoff" || kind == "report" || kind == "room" || kind == "wish" || kind == "shopping" || kind == "reward" || kind == "redemption"
}
func (s *Service) Resources(c Context, kind string, now time.Time) ([]Resource, error) {
	if e := s.Check(c, now); e != nil {
		return nil, e
	}
	return s.resources(c.Viewer, c.CircleID, kind, now)
}
func (s *Service) resources(uid int64, circle int, kind string, now time.Time) ([]Resource, error) {
	out := []Resource{}
	switch kind {
	case "handoff":
		rows, e := (&task.Service{DB: s.DB}).Handoffs(uid, circle, now)
		if e != nil {
			return nil, e
		}
		for _, row := range rows {
			var name string
			if e = s.DB.Table("chores").Select("chores.name").Joins("JOIN hq_task_instances i ON i.chore_id=chores.id").Where("i.id=? AND i.circle_id=?", row.InstanceID, circle).Scan(&name).Error; e != nil {
				return nil, e
			}
			out = append(out, Resource{ID: row.ID, Kind: kind, Status: row.Status, Name: name, CreatedBy: row.FromID, Requester: row.ToID, Snapshot: snapshot(row)})
		}
	case "report":
		user, e := (&family.Service{DB: s.DB}).Authorize(uid, circle)
		if e != nil {
			return nil, e
		}
		var rows []struct {
			task.Attempt
			Award int
			Name  string
		}
		e = s.DB.Table("hq_attempts a").Select("a.*, chores.name, COALESCE(hist.points, 0) AS award").Joins("JOIN chore_histories hist ON hist.id=a.history_id").Joins("JOIN hq_task_instances i ON i.id=a.instance_id JOIN chores ON chores.id=i.chore_id").Where("i.circle_id=? AND (a.user_id=? OR a.status=?)", circle, user.ID, "review").Order("a.id DESC").Scan(&rows).Error
		if e != nil {
			return nil, e
		}
		for _, row := range rows {
			if row.UserID != user.ID && !task.CanReview(s.DB, row.Attempt, circle, user.ID) {
				continue
			}
			r := Resource{ID: row.ID, Kind: kind, Name: row.Name, Points: row.Award, Status: row.Status, CreatedBy: row.UserID, HasPhoto: row.PhotoID != ""}
			r.Snapshot = snapshot(r)
			out = append(out, r)
		}
	case "room":
		rooms, e := (&task.Service{DB: s.DB}).Rooms(uid, circle)
		if e != nil {
			return nil, e
		}
		for _, r := range rooms {
			out = append(out, Resource{ID: r.ID, Name: r.Name, Kind: kind, Status: "active", Snapshot: snapshot(r)})
		}
	case "wish":
		rows, e := (&wishlist.Service{DB: s.DB}).List(uid, circle)
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			// Reservations stay private even in the donor's model context.
			r.Claim = nil
			out = append(out, Resource{ID: r.ID, Kind: kind, Snapshot: snapshot(r), Status: "active", CreatedBy: r.UserID, Name: r.Name, Description: r.Note, Price: r.Price, Link: r.URL})
		}
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
func (s *Service) resource(p Plan, v Change, now time.Time) (Resource, error) {
	cap, ok := capability(v.Operation)
	if !ok || !resourceTarget(cap.Target) {
		return Resource{}, ErrPlan
	}
	rows, e := s.resources(p.Viewer, p.CircleID, cap.Target, now)
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
	case "handoff_accept", "handoff_decline", "handoff_cancel":
		e = (&task.Service{DB: s.DB}).ResolveHandoff(p.Viewer, p.CircleID, v.ID, strings.TrimPrefix(v.Operation, "handoff_"), now)
	case "report_approve", "report_reject":
		e = (&task.Service{DB: s.DB}).Review(p.Viewer, p.CircleID, v.ID, v.Operation == "report_approve", now)
	case "report_undo":
		e = (&task.Service{DB: s.DB}).Undo(p.Viewer, p.CircleID, v.ID, now)
	case "report_photo":
		_, e = (&task.Service{DB: s.DB}).Photo(p.Viewer, p.CircleID, v.ID)
	case "room_delete", "room_rename":
		rooms := &task.Service{DB: s.DB}
		if v.Operation == "room_delete" {
			e = rooms.DeleteRoom(p.Viewer, p.CircleID, v.ID)
		} else {
			e = rooms.RenameRoom(p.Viewer, p.CircleID, v.ID, v.Name)
		}
	case "wish_create":
		r, err := (&wishlist.Service{DB: s.DB}).Create(p.Viewer, p.CircleID, v.Name, v.Link, v.Description, v.Price)
		out.ID, out.Status, e = r.ID, "created", err
	case "wish_edit", "wish_archive":
		ws := &wishlist.Service{DB: s.DB}
		r, err := ws.Owned(p.Viewer, p.CircleID, v.ID)
		if err != nil {
			return out, err
		}
		if v.Operation == "wish_edit" {
			e = ws.Edit(p.Viewer, p.CircleID, v.ID, r.Version, v.Name, v.Link, v.Description, v.Price)
		} else {
			e = ws.Archive(p.Viewer, p.CircleID, v.ID, r.Version)
		}
	case "wish_claim":
		e = (&wishlist.Service{DB: s.DB}).Claim(p.Viewer, p.CircleID, v.ID)
	case "wish_purchase", "wish_release":
		e = (&wishlist.Service{DB: s.DB}).UpdateClaim(p.Viewer, p.CircleID, v.ID, strings.TrimPrefix(v.Operation, "wish_"))
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
func (s *Service) describeResource(p Plan, v Change, now time.Time) (string, error) {
	if v.Operation == "wish_create" {
		return "Добавить желание «" + v.Name + "» " + strings.Join([]string{v.Description, v.Link, v.Price}, " "), nil
	}
	if v.Operation == "reward_create" {
		return fmt.Sprintf("Добавить награду «%s» · %d баллов · %s", v.Name, v.Points, v.Description), nil
	}
	r, e := s.resource(p, v, now)
	if e != nil {
		return "", e
	}
	label := map[string]string{"room_rename": "Переименовать комнату", "room_delete": "Удалить комнату, сохранив задания", "wish_edit": "Изменить желание", "wish_archive": "Убрать желание", "wish_claim": "Тайно забронировать", "wish_release": "Снять свою бронь", "wish_purchase": "Отметить покупку подарка", "shopping_claim": "Взять покупку на себя", "shopping_release": "Освободить покупку", "shopping_buy": "Отметить купленным", "shopping_reopen": "Вернуть в список покупок", "shopping_archive": "Убрать из списка покупок", "shopping_edit": "Изменить покупку", "reward_edit": "Изменить награду", "reward_archive": "Убрать награду из каталога", "reward_request": "Запросить награду", "redemption_approve": "Принять заявку", "redemption_deliver": "Отметить выдачу награды", "redemption_reject": "Отклонить заявку", "redemption_cancel": "Отменить заявку"}[v.Operation]
	if strings.HasPrefix(v.Operation, "handoff_") {
		cap, _ := capability(v.Operation)
		return fmt.Sprintf("%s · «%s» · предложение №%d", cap.Description, r.Name, r.ID), nil
	}
	if strings.HasPrefix(v.Operation, "report_") {
		cap, _ := capability(v.Operation)
		return fmt.Sprintf("%s · «%s» · отчёт №%d · %d баллов", cap.Description, r.Name, r.ID, r.Points), nil
	}
	text := label + ": «" + r.Name + "»"
	switch v.Operation {
	case "room_rename":
		text += " → " + v.Name
	case "wish_edit":
		text += " → " + strings.Join([]string{v.Name, v.Description, v.Link, v.Price}, " ")
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
