// Controlled transport for E01/E02; all writes go through product handlers.
package main

import (
	"context"
	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/agent/agenttest"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/bot"
	"donetick.com/core/internal/homequest/reward"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/stats"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	db, e := gorm.Open(sqlite.Open(os.Args[1]), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		os.Exit(1)
	}
	html, e := os.ReadFile("widgets/family.html")
	if e != nil {
		os.Exit(1)
	}
	app, e := bot.New(db, string(html), "test_homequest_bot")
	if e != nil {
		os.Exit(1)
	}
	var input struct {
		ReplyActions  []string           `json:"reply_actions"`
		ReplySender   string             `json:"reply_sender"`
		VoiceDuration int                `json:"voice_duration"`
		VoiceSize     int                `json:"voice_size"`
		SpeechFailure bool               `json:"speech_failure"`
		Viewer        int64              `json:"viewer"`
		Text          string             `json:"text"`
		Action        string             `json:"action"`
		UpdateID      int64              `json:"update_id"`
		Photo         string             `json:"photo"`
		Now           string             `json:"now"`
		BillingMode   string             `json:"billing_mode"`
		Tick          bool               `json:"tick"`
		AgentFixture  *agenttest.Model   `json:"agent_fixture"`
		AICommand     *assistant.Command `json:"ai_command"`
		Speech        string             `json:"speech"`
		Voice         bool               `json:"voice"`
		ProcessAI     bool               `json:"process_ai"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	if input.AICommand != nil {
		fixture := &fixtureAI{Command: *input.AICommand, Speech: input.Speech}
		app.LLM = fixture
		app.STT = fixture
		app.VoiceSource = fixture
		app.AIProvider = "fixture"
	}
	if input.AgentFixture != nil {
		app.Agent = &agent.Eino{Model: input.AgentFixture}
		speech := &fixtureAI{Speech: input.Speech, Failure: input.SpeechFailure}
		app.STT = speech
		app.VoiceSource = speech
		app.AIProvider = "fixture"
	}
	app.BillingMode = "manual_test"
	if input.BillingMode != "" {
		app.BillingMode = input.BillingMode
	}
	if input.Now != "" {
		at, err := time.Parse(time.RFC3339, input.Now)
		if err != nil {
			os.Exit(2)
		}
		app.Now = func() time.Time { return at }
	}
	var before uint
	db.Model(&bot.Delivery{}).Select("COALESCE(MAX(id),0)").Scan(&before)
	var last int64
	db.Model(&bot.Receipt{}).Select("COALESCE(MAX(id),0)").Scan(&last)
	if input.UpdateID == 0 {
		input.UpdateID = last + 1
	}
	update := bot.Update{ID: input.UpdateID}
	if input.Action != "" {
		update.Query = &bot.Query{ID: "fixture_query", From: bot.Person{ID: input.Viewer}, Chat: bot.Chat{ID: input.Viewer, Type: "private"}, MessageID: 1, Data: input.Action}
	} else {
		update.Message = &bot.Message{From: bot.Person{ID: input.Viewer, Name: fmt.Sprintf("Участник %d", input.Viewer-900000)}, Chat: bot.Chat{ID: input.Viewer, Type: "private"}, Text: input.Text}
	}
	if input.Photo != "" {
		var m bot.Message
		raw, _ := json.Marshal(map[string]any{"from": map[string]any{"id": input.Viewer, "first_name": "Фото"}, "chat": map[string]any{"id": input.Viewer, "type": "private"}, "photo": []map[string]string{{"file_id": input.Photo}}})
		json.Unmarshal(raw, &m)
		update.Message = &m
	}
	if input.Voice {
		update.Message = &bot.Message{From: bot.Person{ID: input.Viewer, Name: "Участник"}, Chat: bot.Chat{ID: input.Viewer, Type: "private"}, Voice: &bot.Voice{FileID: "fixture_audio", Duration: input.VoiceDuration, Size: input.VoiceSize}}
	}
	if len(input.ReplyActions) > 0 && update.Message != nil {
		sender := input.ReplySender
		if sender == "" {
			sender = "test_homequest_bot"
		}
		rows := [][]map[string]string{{}}
		for _, token := range input.ReplyActions {
			rows[0] = append(rows[0], map[string]string{"callback_data": token})
		}
		raw, _ := json.Marshal(map[string]any{"reply_to_message": map[string]any{"from": map[string]any{"is_bot": true, "username": sender}, "reply_markup": map[string]any{"inline_keyboard": rows}}})
		var reply bot.Message
		_ = json.Unmarshal(raw, &reply)
		update.Message.Reply = reply.Reply
	}
	transport := &captureAPI{}
	if input.Tick {
		e = app.PlanWeekly()
		if e == nil {
			e = app.PlanReminders(app.ReminderOffsets)
		}
		if e == nil {
			e = app.PlanReviews()
		}
		for n := 0; e == nil && n < 100; n++ {
			before := len(transport.Sent)
			e = app.Dispatch(context.Background(), transport)
			if len(transport.Sent) == before {
				break
			}
		}
	} else {
		e = app.Handle(update)
	}
	if e == nil && input.ProcessAI {
		e = app.ProcessAI(context.Background())
	}
	if e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": "handler_failed"})
		os.Exit(1)
	}
	var delivery bot.Delivery
	db.Where("viewer = ?", input.Viewer).Order("id DESC").First(&delivery)
	var payload map[string]any
	_ = json.Unmarshal([]byte(delivery.Payload), &payload)
	var state any
	if raw, ok := payload["chat_data"].(string); ok {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	balance := 0
	var summary stats.Summary
	var familyID int
	needed, boughtCount := 0, 0
	var openTasks any
	var members any
	var purchasesView, catalog, redemptions any
	attempts := []map[string]any{}
	if h, e := app.Family.Current(input.Viewer); e == nil {
		familyID = h.ID
		members = h.Members
		openTasks, _ = (&task.Service{DB: db}).List(input.Viewer, h.ID)
		purchases, _ := (&shopping.Service{DB: db}).List(input.Viewer, h.ID, false)
		needed = len(purchases)
		bought, _ := (&shopping.Service{DB: db}).List(input.Viewer, h.ID, true)
		boughtCount = len(bought)
		purchasesView = append(purchases, bought...)
		catalog, _ = (&reward.Service{DB: db}).List(input.Viewer, h.ID)
		redemptions, _ = (&reward.Service{DB: db}).Requests(input.Viewer, h.ID)
		var rows []task.Attempt
		db.Table("hq_attempts a").Select("a.*").Joins("JOIN hq_task_instances i ON i.id=a.instance_id").Where("i.circle_id=?", h.ID).Order("a.id").Find(&rows)
		for _, a := range rows {
			attempts = append(attempts, map[string]any{"id": a.ID, "instance_id": a.InstanceID, "user_id": a.UserID, "status": a.Status, "has_photo": a.PhotoID != ""})
		}
		balance, _ = (&task.Service{DB: db}).Balance(input.Viewer, h.ID)
		summary, _ = (&stats.Service{DB: db}).Summary(input.Viewer, h.ID, app.Now())
	}
	// Read-only test oracles; no fixture writes of successful domain outcomes.
	var jobs []struct {
		ReplyTarget agent.ReplyTarget `gorm:"embedded;embeddedPrefix:reply_target_"`
		ID          uint
		State       string
		PlanID      string
	}
	db.Model(&bot.AIJob{}).Where("viewer=?", input.Viewer).Order("id").Find(&jobs)
	var deliveries []bot.Delivery
	db.Where("viewer=? AND id>?", input.Viewer, before).Order("id").Find(&deliveries)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ai_jobs": jobs, "attempts": attempts, "shopping_items": purchasesView, "reward_catalog": catalog, "redemptions": redemptions, "tasks_open": openTasks, "members": members, "shopping_needed": needed, "shopping_bought": boughtCount, "sent": transport.Sent, "deliveries": deliveries, "state": state, "payload": payload, "balance": balance, "week": summary.Week, "xp": summary.XP, "family_id": familyID, "update_id": input.UpdateID, "at": app.Now()})
}

type captureAPI struct{ Sent []map[string]any }

func (a *captureAPI) Call(_ context.Context, method string, payload any, out any) error {
	a.Sent = append(a.Sent, map[string]any{"method": method, "payload": payload})
	if r, ok := out.(*json.RawMessage); ok {
		*r = json.RawMessage(fmt.Sprintf(`{"message_id":%d}`, len(a.Sent)))
	}
	return nil
}

type fixtureAI struct {
	Failure bool
	Command assistant.Command
	Speech  string
}

func (f *fixtureAI) Parse(context.Context, string, []assistant.Candidate, string) (assistant.Command, error) {
	return f.Command, nil
}
func (f *fixtureAI) Transcribe(context.Context, []byte) (string, error) {
	if f.Failure {
		return "", errors.New("fixture speech unavailable")
	}
	return f.Speech, nil
}
func (f *fixtureAI) Voice(context.Context, string) ([]byte, error) {
	return []byte("controlled fixture audio"), nil
}
