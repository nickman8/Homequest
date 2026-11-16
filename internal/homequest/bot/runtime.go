package bot

import (
	"context"
	"donetick.com/core/internal/homequest/widget"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"sync"
	"time"
)

type API interface {
	Call(context.Context, string, any, any) error
}

var ErrPollingConflict = errors.New("another Telegram polling consumer is active")

func terminalPolling(e error) error {
	var api *widget.APIError
	if errors.As(e, &api) && api.Code == 409 {
		return ErrPollingConflict
	}
	return nil
}

// Dispatch runs in the single polling process. A lost response after Telegram
// accepted a message can cause a duplicate message; domain effects are deduped.
func (b *Bot) Dispatch(ctx context.Context, api API) error {
	var rows []Delivery
	if e := b.DB.Where("sent = ? AND attempts < ? AND next <= ?", false, 5, b.Now()).Order("id").Limit(20).Find(&rows).Error; e != nil {
		return e
	}
	for _, row := range rows {
		var payload map[string]any
		if json.Unmarshal([]byte(row.Payload), &payload) != nil {
			return errors.New("invalid persisted delivery")
		}
		// Never allow old family data to be sent after membership has ended.
		if row.CircleID != 0 {
			if _, e := b.Family.View(row.Viewer, row.CircleID); e != nil {
				if e = b.DB.Model(&row).Update("sent", true).Error; e != nil {
					return e
				}
				continue
			}
		}
		ready, e := b.noticeReady(row)
		if e != nil {
			return e
		}
		if !ready {
			continue
		}
		e = api.Call(ctx, row.Method, payload, nil)
		if e == nil {
			if e = b.DB.Model(&row).Update("sent", true).Error; e != nil {
				return e
			}
			continue
		}

		var rejection *widget.APIError
		if errors.As(e, &rejection) && rejection.Reason == "widget_not_allowed" && (row.Method == "sendWidget" || row.Method == "editMessageWidgetData") {
			var card Card
			raw, ok := payload["chat_data"].(string)
			if !ok || json.Unmarshal([]byte(raw), &card) != nil {
				return errors.New("invalid fallback projection")
			}
			buttons := [][]map[string]string{}
			for _, button := range card.Buttons {
				buttons = append(buttons, []map[string]string{{"text": button.Label, "callback_data": button.Action}})
			}
			fallback := map[string]any{"chat_id": payload["chat_id"], "text": card.Title + "\n\n" + card.Text, "reply_markup": map[string]any{"inline_keyboard": buttons}}
			encoded, err := json.Marshal(fallback)
			if err != nil {
				return err
			}
			if err = b.DB.Model(&row).Updates(map[string]any{"method": "sendMessage", "payload": string(encoded), "widget_fallback": true, "attempts": 0, "next": b.Now()}).Error; err != nil {
				return err
			}
			continue
		}
		delay := time.Duration(1<<uint(row.Attempts)) * time.Second
		var ae *widget.APIError
		if errors.As(e, &ae) && ae.RetryAfter > 0 {
			delay = time.Duration(ae.RetryAfter) * time.Second
		}
		if delay > time.Hour {
			delay = time.Hour
		}
		if e = b.DB.Model(&row).Updates(map[string]any{"attempts": row.Attempts + 1, "next": b.Now().Add(delay)}).Error; e != nil {
			return e
		}
	}
	return nil
}
func (b *Bot) Run(ctx context.Context, api API, status func(string)) error {
	var webhook struct {
		URL string `json:"url"`
	}
	if e := api.Call(ctx, "getWebhookInfo", map[string]any{}, &webhook); e != nil {
		return e
	}
	if webhook.URL != "" {
		return errors.New("webhook present; polling not started")
	}
	var offset int64
	if e := b.DB.Model(&Receipt{}).Select("COALESCE(MAX(id),0)").Scan(&offset).Error; e != nil {
		return e
	}
	if offset > 0 {
		offset++
	}
	var cursor Cursor
	if e := b.DB.First(&cursor, 1).Error; e == nil {
		offset = cursor.Next
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	// Polling and the single durable-outbox worker are independent: an outbound
	// timeout must never hold a widget query behind a queue of slow messages.
	ctx, cancel := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() {
		for ctx.Err() == nil {
			if e := b.PlanReminders(b.ReminderOffsets); e != nil {
				workerDone <- e
				cancel()
				return
			}
			if e := b.PlanReviews(); e != nil {
				workerDone <- e
				cancel()
				return
			}
			if e := b.Dispatch(ctx, api); e != nil {
				workerDone <- e
				cancel()
				return
			}
			select {
			case <-ctx.Done():
			case <-time.After(250 * time.Millisecond):
			}
		}
		workerDone <- nil
	}()
	defer func() { cancel(); <-workerDone }()

	if e := b.DB.Model(&AIJob{}).Where("state = ?", "processing").Updates(map[string]any{"state": "failed", "text": "", "voice_id": "", "command": "", "candidates": ""}).Error; e != nil {
		return e
	}
	aiDone := make(chan struct{})
	go func() {
		defer close(aiDone)
		for ctx.Err() == nil {
			if e := b.ProcessAI(ctx); e != nil {
				status("assistant_worker_error")
			}
			select {
			case <-ctx.Done():
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
	defer func() { cancel(); <-aiDone }()
	failures := 0
	for ctx.Err() == nil {
		var updates []Update
		e := api.Call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 5, "limit": 20, "allowed_updates": []string{"message", "widget_query", "callback_query"}}, &updates)
		if e != nil {
			if fatal := terminalPolling(e); fatal != nil {
				return fatal
			}
			status("telegram_unavailable")
			failures++
			delay := time.Duration(min(failures, 30)) * time.Second
			var ae *widget.APIError
			if errors.As(e, &ae) {
				if ae.Code == 401 || ae.Code == 403 {
					return errors.New("Telegram authorization unavailable")
				}
				if ae.RetryAfter > 0 {
					delay = time.Duration(ae.RetryAfter) * time.Second
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		failures = 0
		status("polling")

		// Acknowledge the whole batch concurrently within a short deadline; a slow
		// query must not consume the next query's 15-second response window.
		var acknowledgements sync.WaitGroup
		for _, update := range updates {
			method := ""
			payload := map[string]any{}
			if update.Query != nil {
				method = "answerWidgetQuery"
				payload["widget_query_id"] = update.Query.ID
			}
			if update.Callback != nil {
				method = "answerCallbackQuery"
				payload["callback_query_id"] = update.Callback.ID
			}
			if method != "" {
				acknowledgements.Add(1)
				go func(method string, payload map[string]any) {
					defer acknowledgements.Done()
					ackCtx, stop := context.WithTimeout(ctx, 2*time.Second)
					defer stop()
					_ = api.Call(ackCtx, method, payload, nil)
				}(method, payload)
			}
		}
		acknowledgements.Wait()
		for _, update := range updates {
			if e = b.Handle(update); e != nil {
				return errors.New("update transaction failed")
			}
			offset = update.ID + 1
		}
	}
	return ctx.Err()
}
