package bot

import (
	"donetick.com/core/internal/homequest/stats"
	"fmt"
	"strings"
	"time"
)

func (b *Bot) points(uid, msg int64, circle int) error {
	b = b.screenFor("points")
	s := &stats.Service{DB: b.DB}
	v, e := s.Summary(uid, circle, b.Now())
	if e != nil {
		return e
	}
	return b.card(uid, msg, "Мой вклад", fmt.Sprintf("Баланс: %d — можно тратить на награды\nЗа неделю: %d — принятые дела\nОпыт за всё время: %d", v.Balance, v.Week, v.XP), circle, "ranking:0", "Рейтинг", "badges", "Достижения", "house", "Дом и история", "rewards:0", "Награды", "premium", "Premium", "home", "Назад")
}
func (b *Bot) ranking(uid, msg int64, circle, page int) error {
	b = b.screenFor(fmt.Sprintf("ranking:%d", page))
	s := &stats.Service{DB: b.DB}
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
	b = b.screenFor("badges")
	names, e := (&stats.Service{DB: b.DB}).Badges(uid, circle, b.Now())
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
	b = b.screenFor("house")
	v, e := (&stats.Service{DB: b.DB}).Summary(uid, circle, b.Now())
	if e != nil {
		return e
	}
	return b.card(uid, msg, "Наш уютный дом", fmt.Sprintf("Уровень дома: %d\nОбщий опыт: %d\nКаждые 100 баллов принятого вклада — новый уровень. Покупки наград опыт не уменьшают.", 1+v.HouseXP/100, v.HouseXP), circle, "history:0", "История баллов", "points", "Назад")
}

func (b *Bot) history(uid, msg int64, circle, page int) error {
	user, e := b.Family.Authorize(uid, circle)
	if e != nil {
		return e
	}
	h, e := b.Family.Current(uid)
	if e != nil {
		return e
	}
	loc, e := time.LoadLocation(h.Timezone)
	if e != nil {
		return e
	}
	var rows []struct {
		ID          int
		Action      int
		Points      int
		CreatedAt   time.Time
		AcceptedAt  *time.Time
		PerformedAt *time.Time
	}
	if e = b.DB.Table("points_histories p").Select("p.id,p.action,p.points,p.created_at,a.accepted_at,h.performed_at").Joins("LEFT JOIN hq_attempts a ON a.ledger_id = p.id").Joins("LEFT JOIN chore_histories h ON h.id = a.history_id").Where("p.circle_id = ? AND p.user_id = ?", circle, user.ID).Order("p.id DESC").Scan(&rows).Error; e != nil {
		return e
	}
	if len(rows) == 0 {
		return b.card(uid, msg, "История баллов", "Начисления появятся после принятого задания.", circle, "points", "Назад")
	}
	if page < 0 || page >= len(rows) {
		page = 0
	}
	r := rows[page]
	kind := map[int]string{0: "Начисление / возврат", 1: "Компенсация отмены", 2: "Запрос награды"}[r.Action]
	text := fmt.Sprintf("%s: %d баллов\nУчёт: %s", kind, r.Points, r.CreatedAt.In(loc).Format("02.01.2006 15:04"))
	if r.AcceptedAt != nil {
		text = fmt.Sprintf("Принято: %s\n%d баллов", r.AcceptedAt.In(loc).Format("02.01.2006 15:04"), r.Points)
	}
	if r.PerformedAt != nil {
		text += "\nЗаявлено выполнение: " + r.PerformedAt.In(loc).Format("02.01.2006 15:04")
	}
	return b.card(uid, msg, "История баллов", text, circle, fmt.Sprintf("history:%d", (page+1)%len(rows)), "Далее", "points", "Назад")
}

func (b *Bot) awardBadges(uid int64) error {
	h, e := b.Family.Current(uid)
	if e != nil {
		return nil
	}
	var ids []int64
	if e = b.DB.Table("hq_identities").Select("telegram_id").Joins("JOIN user_circles ON user_circles.user_id = hq_identities.user_id").Where("circle_id = ? AND is_active = ?", h.ID, true).Scan(&ids).Error; e != nil {
		return e
	}
	for _, tg := range ids {
		if _, e = (&stats.Service{DB: b.DB}).Badges(tg, h.ID, b.Now()); e != nil {
			return e
		}
	}
	return b.refreshFamilyViews(uid, h.ID)
}
