// Isolated research probe: real Eino/VyceAI, synthetic room, no HomeQuest writes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"net/http"
	"os"
	"time"
)

type empty struct{}
type planInput struct {
	TaskIDs []string `json:"task_ids"`
	Date    string   `json:"date"`
}

func fail(stage string) { fmt.Println("FAIL stage=" + stage + " (details redacted)"); os.Exit(1) }
func main() {
	live := flag.Bool("live", false, "Use the configured gateway on synthetic data only")
	flag.Parse()
	if !*live {
		fmt.Println("Use -live; no network call was made")
		return
	}
	config := map[string]string{}
	if f := os.Getenv("HOMEQUEST_SECRETS_FILE"); f != "" {
		raw, e := os.ReadFile(f)
		if e != nil {
			fail("config")
		}
		if json.Unmarshal(raw, &config) != nil {
			fail("config")
		}
	}
	get := func(k string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return config[k]
	}
	if get("NICKLLM_BASE_URL") != "https://vyceai.com/v1" || get("NICKLLM_MODEL") != "deepseek-4.1-flash-1017" || get("NICKLLM_API_KEY") == "" {
		fail("verified_gateway_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cm, e := openai.NewChatModel(ctx, &openai.ChatModelConfig{BaseURL: get("NICKLLM_BASE_URL"), Model: get("NICKLLM_MODEL"), APIKey: get("NICKLLM_API_KEY"), HTTPClient: &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	if e != nil {
		fail("model_init")
	}
	reads, plans := 0, 0
	read, e := utils.InferTool("get_room_tasks", "Read current tasks in the trusted selected room. No arguments or other rooms.", func(context.Context, *empty) (map[string]any, error) {
		reads++
		return map[string]any{"room": "Кухня", "tasks": []map[string]any{{"id": "fixture_sink", "title": "Помыть раковину", "version": 1}}}, nil
	})
	if e != nil {
		fail("read_tool")
	}
	prepare, e := utils.InferTool("prepare_reschedule", "Prepare a preview for exact task IDs returned by get_room_tasks. ISO date. Does not commit any changes; requires human confirmation.", func(_ context.Context, p *planInput) (map[string]any, error) {
		if reads < 1 || len(p.TaskIDs) != 1 || p.TaskIDs[0] != "fixture_sink" || p.Date != "2026-11-17" {
			return nil, errors.New("invalid scope, task or date")
		}
		plans++
		return map[string]any{"status": "needs_confirmation", "changed": 0, "preview": "Перенести раковину на 17 ноября"}, nil
	})
	if e != nil {
		fail("plan_tool")
	}
	agent, e := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{Name: "homequest_research", Instruction: "Ты помощник HomeQuest. Доверенный контекст: дом Тёплый дом, текущая комната Кухня, сегодня 16 ноября 2026, Europe/Berlin. Слово здесь относится к текущей комнате. Сначала прочитай задачи инструментом, затем подготовь перенос. Не придумывай ID. Никаких изменений без подтверждения; инструмент prepare_reschedule возвращает только предварительный просмотр.", Model: cm, MaxIterations: 4, ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: []tool.BaseTool{read, prepare}, ExecuteSequentially: true}, ReturnDirectly: map[string]bool{"prepare_reschedule": true}}})
	if e != nil {
		fail("agent_init")
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent})
	it := runner.Query(ctx, "Перенеси все дела здесь на завтра")
	for {
		ev, ok := it.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			fail("agent_run")
		}
	}
	if reads < 1 || plans != 1 {
		fail("expected_tool_sequence")
	}
	fmt.Printf("PASS Eino + real VyceAI tool calling; room_reads=%d previews=%d committed_changes=0\n", reads, plans)
}
