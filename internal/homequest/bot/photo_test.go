package bot

import (
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
)

func TestE03PhotoApprovalThroughUpdates(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/e03.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<p>Дом</p>", "test_bot")
	if e != nil {
		t.Fatal(e)
	}
	var seq int64
	send := func(uid int64, text, action string) {
		t.Helper()
		seq++
		u := Update{ID: seq}
		if action != "" {
			u.Query = &Query{ID: "test", From: Person{ID: uid}, Chat: Chat{ID: uid, Type: "private"}, MessageID: 1, Data: action}
		} else {
			u.Message = &Message{From: Person{ID: uid, Name: "Участник"}, Chat: Chat{ID: uid, Type: "private"}, Text: text}
		}
		if e = b.Handle(u); e != nil {
			t.Fatal(e)
		}
	}
	card := func(uid int64) Card {
		t.Helper()
		var row Delivery
		db.Where("viewer = ? AND method IN ?", uid, []string{"sendWidget", "editMessageWidgetData"}).Order("id DESC").First(&row)
		var p map[string]any
		_ = json.Unmarshal([]byte(row.Payload), &p)
		var c Card
		if raw, ok := p["chat_data"].(string); ok {
			_ = json.Unmarshal([]byte(raw), &c)
		}
		return c
	}
	press := func(uid int64, label string) {
		t.Helper()
		for _, v := range card(uid).Buttons {
			if v.Label == label {
				send(uid, "", v.Action)
				return
			}
		}
		t.Fatalf("missing button %s in %#v", label, card(uid))
	}
	const A int64 = 900001
	const B int64 = 900002
	send(A, "/start", "")
	send(A, "/create", "")
	send(A, "Дом", "")
	send(A, "UTC", "")
	press(A, "Настройки")
	press(A, "Пригласить")
	var invite Delivery
	db.Where("viewer = ?", A).Order("id DESC").First(&invite)
	var p map[string]string
	_ = json.Unmarshal([]byte(invite.Payload), &p)
	var token string
	for i := 0; i+5 < len(p["text"]); i++ {
		if p["text"][i:i+5] == "join_" {
			token = p["text"][i+5:]
			break
		}
	}
	send(B, "/start join_"+token, "")
	send(A, "/task", "")
	send(A, "Мусор", "")
	send(A, "10", "")
	press(A, "Параметры")
	press(A, "Правила")
	press(A, "Фото и подтверждение")
	send(B, "/start", "")
	press(B, "Сегодня")
	press(B, "Выполнено")
	home, _ := b.Family.Current(B)
	if balance, _ := (&task.Service{DB: db}).Balance(B, home.ID); balance != 0 {
		t.Fatal("early points")
	}
	seq++
	var photo Update
	_ = json.Unmarshal([]byte(`{"message":{"from":{"id":900002,"first_name":"B"},"chat":{"id":900002,"type":"private"},"photo":[{"file_id":"fixture_photo_1"}]}}`), &photo)
	photo.ID = seq
	if e = b.Handle(photo); e != nil {
		t.Fatal(e)
	}
	send(A, "/start", "")
	press(A, "Настройки")
	press(A, "Отчёты")
	press(A, "Посмотреть фото")
	var media Delivery
	db.Where("viewer = ?", A).Order("id DESC").First(&media)
	if media.Method != "sendPhoto" {
		t.Fatal("photo must be separate media")
	}
	press(A, "Принять")
	if balance, _ := (&task.Service{DB: db}).Balance(B, home.ID); balance != 10 {
		t.Fatal("approval points", balance)
	}
}
