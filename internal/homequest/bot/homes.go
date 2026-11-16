package bot

import "fmt"

func (b *Bot) homes(uid, msg int64, page int) error {
	rows, e := b.Family.Homes(uid)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.home(uid, msg)
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	h := rows[page]
	text := "Переключение происходит только по вашей кнопке. История каждого дома сохраняется отдельно."
	if h.Current {
		text = "Это ваш активный дом.\n" + text
	}
	return b.card(uid, msg, short(h.Name, 40), text, 0, fmt.Sprintf("switch_home:%d", h.ID), "Открыть этот дом", fmt.Sprintf("homes:%d", (page+1)%len(rows)), "Следующий дом", "home", "Назад")
}
