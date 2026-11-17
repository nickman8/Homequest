// Package agenttest contains an offline model for bot/browser tests only.
// It emits actual Eino tool calls; tools still read the real test database.
package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/shopping"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Model struct {
	Changes        []agent.Change   `json:"changes"`
	ExtraChanges   []agent.Change   `json:"extra_changes"`
	Offset         int              `json:"offset"`
	ForgedSnapshot bool             `json:"forged_snapshot"`
	Malformed      bool             `json:"malformed"`
	Timeout        bool             `json:"timeout"`
	AssignedTo     *int             `json:"assigned_to"`
	Reviewer       *int             `json:"reviewer"`
	Approval       bool             `json:"approval"`
	Photo          bool             `json:"photo"`
	Status         string           `json:"status"`
	Operation      string           `json:"operation"`
	Due            string           `json:"due"`
	Search         string           `json:"search"`
	RoomID         int              `json:"room_id"`
	Target         int              `json:"target"`
	Name           string           `json:"name"`
	Points         int              `json:"points"`
	Description    string           `json:"description"`
	Items          []shopping.Input `json:"items"`
	ForeignID      int              `json:"foreign_id"`
	Failure        bool             `json:"failure"`
	step           int
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
	if m.Timeout {
		return nil, context.DeadlineExceeded
	}
	if m.Malformed {
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "invalid", Type: "function", Function: schema.FunctionCall{Name: "prepare_changes", Arguments: "{invalid"}}}}, nil
	}
	if m.Failure {
		return nil, errors.New("fixture model unavailable")
	}
	call := func(name string, args any) *schema.Message {
		raw, _ := json.Marshal(args)
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: fmt.Sprintf("fixture_%d", step), Type: "function", Function: schema.FunctionCall{Name: name, Arguments: string(raw)}}}}
	}
	if len(m.Changes) > 0 {
		return call("prepare_changes", map[string]any{"changes": m.Changes}), nil
	}
	if m.Operation == "account_read" {
		if step == 1 {
			return call("read_account", map[string]any{}), nil
		}
		var account agent.Account
		for _, v := range messages {
			if v.Role == schema.Tool {
				_ = json.Unmarshal([]byte(v.Content), &account)
			}
		}
		return call("reply", map[string]any{"text": fmt.Sprintf("Баланс: %d. Вклад недели: %d. Опыт: %d. Участников в рейтинге: %d.", account.Summary.Balance, account.Summary.Week, account.Summary.XP, len(account.Ranking))}), nil
	}
	if m.Operation == "select_room" {
		return call("select_room", map[string]any{"room_id": m.RoomID}), nil
	}
	if m.Operation == "task_create" || m.Operation == "reward_create" || m.Operation == "shopping_add" {
		return call("prepare_changes", map[string]any{"changes": []agent.Change{{Operation: m.Operation, Name: m.Name, Points: m.Points, Description: m.Description, Items: m.Items, AssignedTo: m.AssignedTo, Reviewer: m.Reviewer, Approval: m.Approval, Photo: m.Photo, Due: m.Due}}}), nil
	}
	kind := ""
	for _, k := range []string{"shopping", "reward", "redemption"} {
		if strings.HasPrefix(m.Operation, k+"_") {
			kind = k
		}
	}
	if kind != "" {
		if step == 1 {
			return call("read_resources", map[string]any{"kind": kind, "offset": m.Offset, "search": m.Search, "status": m.Status}), nil
		}
		var result struct {
			Resources []agent.Resource `json:"resources"`
		}
		for _, v := range messages {
			if v.Role == schema.Tool {
				_ = json.Unmarshal([]byte(v.Content), &result)
			}
		}
		if strings.HasSuffix(m.Operation, "_read") {
			return call("reply", map[string]any{"text": fmt.Sprintf("Найдено позиций: %d", len(result.Resources))}), nil
		}
		changes := []agent.Change{}
		for _, r := range result.Resources {
			change := agent.Change{Operation: m.Operation, ID: r.ID, Snapshot: r.Snapshot}
			if m.ForgedSnapshot {
				change.Snapshot = strings.Repeat("0", 64)
			}
			if m.Operation == "reward_edit" {
				change.Name = m.Name
				change.Points = m.Points
				change.Description = m.Description
			}
			if m.Operation == "shopping_edit" {
				change.Items = m.Items
			}
			if m.ForeignID != 0 {
				change.ID = m.ForeignID
			}
			changes = append(changes, change)
		}
		return call("prepare_changes", map[string]any{"changes": append(changes, m.ExtraChanges...)}), nil
	}
	if step == 1 {
		return call("read_tasks", map[string]any{"room_id": m.RoomID, "offset": m.Offset, "search": m.Search}), nil
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
	return call("prepare_changes", map[string]any{"changes": append(changes, m.ExtraChanges...)}), nil
}
