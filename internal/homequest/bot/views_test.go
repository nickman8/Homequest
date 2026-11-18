package bot

import (
	"context"
	"donetick.com/core/internal/homequest/widget"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type widgetMessageTransport struct {
	next          int
	failRefresh   bool
	staleAttempts int
}

func (a *widgetMessageTransport) Call(_ context.Context, method string, payload any, out any) error {
	if a.failRefresh && method == "editMessageWidgetData" {
		p := payload.(map[string]any)
		raw, _ := p["chat_data"].(string)
		if p["chat_id"] == float64(900001) && strings.Contains(raw, "Пока нет доступных заданий") {
			a.staleAttempts++
			if a.staleAttempts == 1 {
				return &widget.APIError{Code: 429, RetryAfter: 30}
			}
		}
	}
	a.next++
	if raw, ok := out.(*json.RawMessage); ok {
		v, _ := json.Marshal(map[string]int{"message_id": a.next})
		*raw = v
	}
	return nil
}
func TestOtherMembersTaskCardRefreshesThroughOutbox(t *testing.T) {
	j := newJourney(t)
	const A int64 = 900001
	const B int64 = 900002
	j.create(A, "Тёплый дом", "UTC")
	j.send(B, "/start join_"+j.invite(A), "")
	j.send(A, "/start", "")
	j.click(A, "Добавить задание")
	j.send(A, "Вынести мусор", "")
	j.send(A, "10", "")
	j.send(B, "/start", "")
	j.click(B, "Сегодня")
	api := &widgetMessageTransport{}
	for i := 0; i < 5; i++ {
		if e := j.b.Dispatch(context.Background(), api); e != nil {
			t.Fatal(e)
		}
	}
	var before Delivery
	j.db.Order("id DESC").First(&before)
	j.click(B, "Выполнено")
	var changed Delivery
	if e := j.db.Where("id>? AND viewer=? AND method=?", before.ID, A, "editMessageWidgetData").Order("id DESC").First(&changed).Error; e != nil {
		t.Fatal("other member card not refreshed", e)
	}
	var p map[string]any
	json.Unmarshal([]byte(changed.Payload), &p)
	if !strings.Contains(p["chat_data"].(string), "Пока нет доступных заданий") {
		t.Fatal("stale task state")
	}
	api.failRefresh = true
	j.send(A, "/start", "")
	j.click(A, "Настройки")
	for i := 0; i < 5; i++ {
		if e := j.b.Dispatch(context.Background(), api); e != nil {
			t.Fatal(e)
		}
	}
	j.now = j.now.Add(time.Minute)
	if e := j.b.Dispatch(context.Background(), api); e != nil {
		t.Fatal(e)
	}
	if api.staleAttempts != 1 {
		t.Fatal("late retry overwrote navigation", api.staleAttempts)
	}
	// An expired membership cannot receive a refresh of retained old messages.
	j.send(B, "/start", "")
	j.click(B, "Настройки")
	j.click(B, "Выйти из дома")
	j.click(B, "Подтвердить выход")
	before = Delivery{}
	j.db.Order("id DESC").First(&before)
	j.send(A, "/start", "")
	j.click(A, "Добавить задание")
	j.send(A, "Полить цветы", "")
	j.send(A, "5", "")
	var n int64
	j.db.Model(&Delivery{}).Where("id>? AND viewer=?", before.ID, B).Count(&n)
	if n != 0 {
		t.Fatal("former member got state")
	}
}
