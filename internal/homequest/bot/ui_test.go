package bot

import (
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
)

func TestSwitchUIWithoutChangingActions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ui.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(db, "<p>Дом</p>", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = b.ConfigureUI("unknown"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	for i, mode := range []string{"buttons", "auto", "widgets"} {
		if err = b.ConfigureUI(mode); err != nil {
			t.Fatal(err)
		}
		if err = b.Handle(Update{ID: int64(i + 1), Message: &Message{From: Person{ID: 900001, Name: "A"}, Chat: Chat{ID: 900001, Type: "private"}, Text: "/start"}}); err != nil {
			t.Fatal(err)
		}
		var d Delivery
		db.Order("id DESC").First(&d)
		want := "sendWidget"
		if mode == "buttons" {
			want = "sendMessage"
		}
		if d.Method != want {
			t.Fatalf("%s: %s", mode, d.Method)
		}
		var p struct {
			Markup struct {
				Buttons [][]struct {
					Data string `json:"callback_data"`
				} `json:"inline_keyboard"`
			} `json:"reply_markup"`
		}
		if err = json.Unmarshal([]byte(d.Payload), &p); err != nil {
			t.Fatal(err)
		}
		token := p.Markup.Buttons[0][0].Data
		var a Action
		if err = db.First(&a, "token = ? AND viewer = ?", token, 900001).Error; err != nil || a.Kind != "create" {
			t.Fatal("mode changed domain action", err)
		}
	}
}
