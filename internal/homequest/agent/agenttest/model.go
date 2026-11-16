// Package agenttest contains an offline model for bot/browser tests only.
// It emits actual Eino tool calls; tools still read the real test database.
package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"donetick.com/core/internal/homequest/agent"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Model struct {
	Operation string `json:"operation"`
	Due       string `json:"due"`
	Search    string `json:"search"`
	RoomID    int    `json:"room_id"`
	Target    int    `json:"target"`
	Name      string `json:"name"`
	Points    int    `json:"points"`
	ForeignID int    `json:"foreign_id"`
	Failure   bool   `json:"failure"`
	step      int
}

func (m *Model) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	copy := *m
	copy.step = 0
	return &copy, nil
}
func (m *Model) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream unused")
}
func (m *Model) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	step := 1
	for _, msg := range messages {
		if msg.Role == schema.Assistant {
			step++
		}
	}
	if m.Failure {
		return nil, errors.New("fixture model unavailable")
	}
	call := func(name string, args any) *schema.Message {
		raw, _ := json.Marshal(args)
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: fmt.Sprintf("fixture_%d", step), Type: "function", Function: schema.FunctionCall{Name: name, Arguments: string(raw)}}}}
	}
	if m.Operation == "select_room" {
		return call("select_room", map[string]any{"room_id": m.RoomID}), nil
	}
	if m.Operation == "task_create" {
		return call("prepare_changes", map[string]any{"changes": []agent.Change{{Operation: m.Operation, Name: m.Name, Points: m.Points}}}), nil
	}
	if step == 1 {
		return call("read_tasks", map[string]any{"room_id": m.RoomID, "offset": 0, "search": m.Search}), nil
	}
	var result struct {
		Tasks []agent.Task `json:"tasks"`
	}
	for _, v := range messages {
		if v.Role == schema.Tool {
			_ = json.Unmarshal([]byte(v.Content), &result)
		}
	}
	if m.Operation == "read" {
		return call("reply", map[string]any{"text": fmt.Sprintf("Найдено дел: %d", len(result.Tasks))}), nil
	}
	changes := []agent.Change{}
	for _, r := range result.Tasks {
		changes = append(changes, agent.Change{Operation: m.Operation, ID: r.ID, Version: r.Version, Due: m.Due, Target: m.Target})
	}
	if m.ForeignID != 0 {
		changes = []agent.Change{{Operation: m.Operation, ID: m.ForeignID, Version: 1, Due: m.Due}}
	}
	return call("prepare_changes", map[string]any{"changes": changes}), nil
}
