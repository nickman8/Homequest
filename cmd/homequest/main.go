package main

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/bot"
	"donetick.com/core/internal/homequest/widget"
	"encoding/json"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		log.Print("HomeQuest stopped: ", e)
		os.Exit(1)
	}
}
func run() error {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		return errors.New("DATA_DIR required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return errors.New("data directory unavailable")
	}
	lock, e := os.OpenFile(filepath.Join(dir, "polling.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return errors.New("lock unavailable")
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errors.New("polling process already running")
	}
	secret := map[string]string{}
	if p := os.Getenv("HOMEQUEST_SECRETS_FILE"); p != "" {
		raw, e := os.ReadFile(p)
		if e != nil || json.Unmarshal(raw, &secret) != nil {
			return errors.New("secret configuration unavailable")
		}
	}
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		token = secret["TELEGRAM_BOT_TOKEN"]
	}
	if token == "" {
		return errors.New("Telegram token required")
	}
	api := widget.NewClient(token, &http.Client{Timeout: 10 * time.Second})
	var me struct {
		Username string `json:"username"`
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if e = api.Call(ctx, "getMe", map[string]any{}, &me); e != nil {
		return e
	}
	html, e := os.ReadFile("widgets/family.html")
	if e != nil {
		return errors.New("widget build missing")
	}
	db, e := gorm.Open(sqlite.Open(filepath.Join(dir, "donetick.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return errors.New("database unavailable")
	}
	sql, e := db.DB()
	if e != nil {
		return errors.New("database handle unavailable")
	}
	defer sql.Close()
	sql.SetMaxOpenConns(1)
	if e = db.Exec("PRAGMA busy_timeout=5000").Error; e != nil {
		return errors.New("database busy timeout unavailable")
	}
	app, e := bot.New(db, string(html), me.Username)
	if e != nil {
		return errors.New("migration failed")
	}

	config := func(name string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return secret[name]
	}
	llmStatus := "model_not_verified"
	if model := config("NICKLLM_MODEL"); model != "" && config("NICKLLM_API_KEY") != "" {
		app.LLM = &assistant.NickLLM{Key: config("NICKLLM_API_KEY"), Model: model}
		llmStatus = "configured_unverified"
	}
	sttStatus := "not_configured"
	if python := config("STT_PYTHON"); python != "" {
		if _, e := os.Stat(python); e != nil {
			return errors.New("STT Python unavailable")
		}
		app.STT = &assistant.FasterWhisper{Python: python, Script: "scripts/transcribe.py", TempDir: dir}
		app.VoiceSource = api
		sttStatus = "configured"
	}
	app.AIDailyLimit = 100
	app.AIPremiumLimit = 500
	for name, target := range map[string]*int{"AI_DAILY_LIMIT": &app.AIDailyLimit, "AI_PREMIUM_DAILY_LIMIT": &app.AIPremiumLimit} {
		if v := config(name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 {
				return errors.New("invalid AI limit")
			}
			*target = n
		}
	}
	app.BillingMode = os.Getenv("BILLING_MODE")
	if app.BillingMode == "" {
		app.BillingMode = "manual_test"
	}
	if value := os.Getenv("REMINDER_OFFSETS_MINUTES"); value != "" {
		var offsets []int
		for _, part := range strings.Split(value, ",") {
			n, e := strconv.Atoi(strings.TrimSpace(part))
			if e != nil || n < -10080 || n > 10080 {
				return errors.New("invalid reminder offsets")
			}
			offsets = append(offsets, n)
		}
		if len(offsets) > 5 {
			return errors.New("too many reminder offsets")
		}
		app.ReminderOffsets = offsets
	}
	var state atomic.Value
	state.Store("starting")
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if sql.PingContext(r.Context()) != nil {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"database":"unavailable"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"database": "ready", "telegram": state.Load(), "llm": llmStatus, "stt": sttStatus, "scheduler": "polling_worker"})
	})
	addr := os.Getenv("HOMEQUEST_HEALTH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:2022"
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		if e := server.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			state.Store("health_listener_failed")
			stop()
		}
	}()
	defer server.Close()
	log.Print("HomeQuest starting; bot @", me.Username)
	return app.Run(ctx, api, func(s string) { state.Store(s) })
}
