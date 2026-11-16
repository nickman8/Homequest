package bot

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFeatureCardBudgets(t *testing.T) {
	j := newJourney(t)
	j.create(900001, strings.Repeat("Дом ", 16), "America/Argentina/Buenos_Aires")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Добавить желание")
	j.send(900001, strings.Repeat("Я", 100), "")
	j.send(900001, "https://example.org/"+strings.Repeat("a", 900), "")
	j.send(900001, strings.Repeat("З", 500), "")
	j.send(900001, strings.Repeat("Ц", 40), "")
	j.click(900001, "Сохранить")
	j.send(900002, "/start", "")
	j.click(900002, "Подарки")
	j.click(900002, "Забронировать тайно")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Править")
	j.send(900001, strings.Repeat("Н", 100), "")
	for i := 0; i < 3; i++ {
		j.click(900001, "Оставить")
	}
	j.click(900001, "Сохранить")
	j.send(900002, "/start", "")
	j.click(900002, "Подарки")
	j.send(900001, "/start", "")
	j.click(900001, "Настройки")
	j.click(900001, "Напоминания")
	j.click(900001, "Для всего дома")
	j.click(900001, "Другие интервалы")
	j.send(900001, "за 10080, за 1440, за 60, в срок, через 120", "")
	j.click(900001, "Назад")
	if path := os.Getenv("HQ_FEATURE_CARDS_OUT"); path != "" {
		var rows []Delivery
		if e := j.db.Order("id").Find(&rows).Error; e != nil {
			t.Fatal(e)
		}
		cards := map[string]Card{}
		for _, r := range rows {
			var p map[string]any
			json.Unmarshal([]byte(r.Payload), &p)
			if raw, ok := p["chat_data"].(string); ok {
				var c Card
				json.Unmarshal([]byte(raw), &c)
				cards[c.Title] = c
			}
		}
		raw, e := json.Marshal(cards)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
