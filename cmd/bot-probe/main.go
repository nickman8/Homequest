// Controlled transport for E01/E02; all writes go through product handlers.
package main

import (
	"donetick.com/core/internal/homequest/bot"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
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
		Viewer   int64  `json:"viewer"`
		Text     string `json:"text"`
		Action   string `json:"action"`
		UpdateID int64  `json:"update_id"`
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil {
		os.Exit(2)
	}
	var last int64
	db.Model(&bot.Receipt{}).Select("COALESCE(MAX(id),0)").Scan(&last)
	if input.UpdateID == 0 {
		input.UpdateID = last + 1
	}
	update := bot.Update{ID: input.UpdateID}
	if input.Action != "" {
		update.Query = &bot.Query{ID: "fixture_query", From: bot.Person{ID: input.Viewer}, Chat: bot.Chat{ID: input.Viewer, Type: "private"}, MessageID: 1, Data: input.Action}
	} else {
		update.Message = &bot.Message{From: bot.Person{ID: input.Viewer, Name: "Участник"}, Chat: bot.Chat{ID: input.Viewer, Type: "private"}, Text: input.Text}
	}
	if e = app.Handle(update); e != nil {
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
	var familyID int
	if h, e := app.Family.Current(input.Viewer); e == nil {
		familyID = h.ID
		balance, _ = (&task.Service{DB: db}).Balance(input.Viewer, h.ID)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"state": state, "payload": payload, "balance": balance, "family_id": familyID, "update_id": input.UpdateID, "at": time.Now().UTC()})
}
