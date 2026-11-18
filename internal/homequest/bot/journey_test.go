package bot

import (
	"context"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
	"time"
)

type journey struct {
	t   *testing.T
	b   *Bot
	db  *gorm.DB
	seq int64
	now time.Time
}

func newJourney(t *testing.T) *journey {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/journey.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	j := &journey{t: t, db: db, now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	j.restart()
	return j
}
func (j *journey) restart() {
	j.t.Helper()
	b, e := New(j.db, "<p>Дом</p>", "test")
	if e != nil {
		j.t.Fatal(e)
	}
	j.b = b
	b.Now = func() time.Time { return j.now }
}
func (j *journey) send(uid int64, text, action string) {
	j.t.Helper()
	j.seq++
	u := Update{ID: j.seq}
	if action != "" {
		u.Query = &Query{ID: "fixture", From: Person{ID: uid}, Chat: Chat{ID: uid, Type: "private"}, MessageID: 1, Data: action}
	} else {
		u.Message = &Message{From: Person{ID: uid, Name: "Участник"}, Chat: Chat{ID: uid, Type: "private"}, Text: text}
	}
	if e := j.b.Handle(u); e != nil {
		j.t.Fatal(e)
	}
}
func (j *journey) card(uid int64) Card {
	j.t.Helper()
	var d Delivery
	j.db.Where("viewer=? AND method IN ?", uid, []string{"sendWidget", "editMessageWidgetData"}).Order("id DESC").First(&d)
	var p map[string]any
	json.Unmarshal([]byte(d.Payload), &p)
	var c Card
	if raw, ok := p["chat_data"].(string); ok {
		json.Unmarshal([]byte(raw), &c)
	}
	return c
}
func (j *journey) token(uid int64, label string) string {
	j.t.Helper()
	for _, b := range j.card(uid).Buttons {
		if b.Label == label {
			return b.Action
		}
	}
	j.t.Fatalf("button %s missing: %+v", label, j.card(uid))
	return ""
}
func (j *journey) click(uid int64, label string) { j.t.Helper(); j.send(uid, "", j.token(uid, label)) }
func (j *journey) create(uid int64, name, zone string) {
	j.t.Helper()
	j.send(uid, "/start", "")
	j.click(uid, "Создать дом")
	j.send(uid, name, "")
	j.send(uid, zone, "")
}
func (j *journey) invite(uid int64) string {
	j.t.Helper()
	j.send(uid, "/start", "")
	j.click(uid, "Настройки")
	j.click(uid, "Пригласить")
	var d Delivery
	j.db.Where("viewer=?", uid).Order("id DESC").First(&d)
	var p map[string]any
	json.Unmarshal([]byte(d.Payload), &p)
	text, _ := p["text"].(string)
	parts := strings.Split(text, "join_")
	if len(parts) != 2 {
		j.t.Fatal("missing invitation")
	}
	return parts[1]
}

type noticeTransport struct{ notices []string }

func (n *noticeTransport) Call(_ context.Context, method string, payload any, _ any) error {
	if method == "sendMessage" {
		p := payload.(map[string]any)
		text, _ := p["text"].(string)
		if strings.HasPrefix(text, "Мягкое напоминание") || strings.Contains(text, "отчёт на проверку") {
			n.notices = append(n.notices, text)
		}
	}
	return nil
}

func TestE04LateApprovalThroughBotAndWorker(t *testing.T) {
	j := newJourney(t)
	const A int64 = 900001
	const B int64 = 900002
	j.create(A, "Тёплый дом", "Europe/Berlin")
	invite := j.invite(A)
	j.send(B, "/start join_"+invite, "")
	j.send(A, "/start", "")
	j.click(A, "Добавить задание")
	j.send(A, "Вынести мусор", "")
	j.send(A, "10", "")
	j.click(A, "Параметры")
	j.click(A, "Повторение")
	j.send(A, "вт 19:00", "")
	j.click(A, "Параметры")
	j.click(A, "Правила")
	j.click(A, "Подтверждение")
	if e := j.b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	transport := &noticeTransport{}
	j.now = time.Date(2026, 3, 3, 17, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		if e := j.b.Dispatch(context.Background(), transport); e != nil {
			t.Fatal(e)
		}
	}
	if len(transport.notices) != 2 {
		t.Fatal("pre-due delivery to both members", transport.notices)
	}
	j.now = time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	j.send(B, "/start", "")
	j.click(B, "Сегодня")
	old := j.token(B, "Выполнено")
	j.click(B, "Выполнено")
	var count int64
	j.db.Model(&task.Instance{}).Count(&count)
	if count != 1 {
		t.Fatal("new instance before acceptance")
	}
	j.now = time.Date(2026, 4, 8, 22, 0, 0, 0, time.UTC)
	j.restart()
	before := len(transport.notices)
	if e := j.b.PlanReminders([]int{-60, 0, 120}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		if e := j.b.Dispatch(context.Background(), transport); e != nil {
			t.Fatal(e)
		}
	}
	if len(transport.notices) != before {
		t.Fatal("restart sent stale notices")
	}
	j.send(A, "/start", "")
	j.click(A, "Настройки")
	j.click(A, "Отчёты")
	j.click(A, "Принять")
	var next task.Instance
	j.db.Where("closed=?", false).First(&next)
	expected := time.Date(2026, 4, 14, 17, 0, 0, 0, time.UTC)
	if next.DueAt == nil || !next.DueAt.Equal(expected) {
		t.Fatal("late acceptance/DST", next)
	}
	j.send(B, "", old)
	j.db.Model(&task.Instance{}).Count(&count)
	if count != 2 {
		t.Fatal("old action spawned instance")
	}
	h, _ := j.b.Family.Current(B)
	balance, e := (&task.Service{DB: j.db}).Balance(B, h.ID)
	if e != nil || balance != 10 {
		t.Fatal(balance, e)
	}
	var attempts []task.Attempt
	j.db.Find(&attempts)
	if len(attempts) != 1 || attempts[0].AcceptedAt.IsZero() || !attempts[0].AcceptedAt.Equal(j.now) {
		t.Fatal("acceptance audit", attempts)
	}
}

func TestMultipleHomesThroughButtons(t *testing.T) {
	j := newJourney(t)
	const A int64 = 900001
	const B int64 = 900002
	j.create(A, "Первый дом", "UTC")
	old := j.token(A, "Добавить задание")
	first, _ := j.b.Family.Current(A)
	j.create(B, "Второй дом", "UTC")
	invite := j.invite(B)
	j.send(A, "/start join_"+invite, "")
	current, _ := j.b.Family.Current(A)
	if current.ID != first.ID {
		t.Fatal("join switched active home")
	}
	j.click(A, "Следующий дом")
	j.click(A, "Открыть этот дом")
	current, _ = j.b.Family.Current(A)
	if current.Name != "Второй дом" {
		t.Fatal(current)
	}
	j.send(A, "", old)
	step, _ := j.b.Family.Step(A)
	if step.Step != "" {
		t.Fatal("old home action started draft")
	}
	j.send(A, "/start", "")
	j.click(A, "Мои дома")
	j.click(A, "Открыть этот дом")
	current, _ = j.b.Family.Current(A)
	if current.ID != first.ID {
		t.Fatal("cannot return")
	}
}
