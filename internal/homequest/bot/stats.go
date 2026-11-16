package bot

import (
	"donetick.com/core/internal/homequest/stats"
	"fmt"
	"strings"
)

func (b *Bot) points(uid, msg int64, circle int) error {
	s := &stats.Service{DB: b.DB, WeeklyGoal: 100}
	v, e := s.Summary(uid, circle, b.Now())
	if e != nil {
		return e
	}
	return b.card(uid, msg, "Мой вклад", fmt.Sprintf("Баланс: %d — можно тратить на награды\nЗа неделю: %d — принятые дела\nОпыт за всё время: %d", v.Balance, v.Week, v.XP), circle, "ranking:0", "Рейтинг", "badges", "Достижения", "house", "Общий дом", "rewards:0", "Награды", "premium", "Premium", "home", "Назад")
}
func (b *Bot) ranking(uid, msg int64, circle, page int) error {
	s := &stats.Service{DB: b.DB, WeeklyGoal: 100}
	rows, e := s.Ranking(uid, circle, b.Now())
	if e != nil {
		return e
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	var lines []string
	for i := page; i < len(rows) && i < page+3; i++ {
		r := rows[i]
		name := []rune(r.Name)
		if len(name) > 24 {
			name = name[:24]
		}
		lines = append(lines, fmt.Sprintf("%d. %s — %d", r.Rank, string(name), r.Points))
	}
	home, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	next := page + 3
	if next >= len(rows) {
		next = 0
	}
	return b.card(uid, msg, "Вклад за неделю", strings.Join(lines, "\n")+"\nС понедельника 00:00 · "+home.Timezone, circle, fmt.Sprintf("ranking:%d", next), "Далее", "points", "Назад")
}
func (b *Bot) badges(uid, msg int64, circle int) error {
	names, e := (&stats.Service{DB: b.DB, WeeklyGoal: 100}).Badges(uid, circle, b.Now())
	if e != nil {
		return e
	}
	text := strings.Join(names, "\n")
	if text == "" {
		text = "Первое достижение появится после принятого задания."
	}
	return b.card(uid, msg, "Достижения", text, circle, "points", "Назад")
}
func (b *Bot) house(uid, msg int64, circle int) error {
	v, e := (&stats.Service{DB: b.DB, WeeklyGoal: 100}).Summary(uid, circle, b.Now())
	if e != nil {
		return e
	}
	return b.card(uid, msg, "Наш уютный дом", fmt.Sprintf("Уровень дома: %d\nОбщий опыт: %d\nКаждые 100 баллов принятого вклада — новый уровень. Покупки наград опыт не уменьшают.", 1+v.HouseXP/100, v.HouseXP), circle, "points", "Назад")
}
