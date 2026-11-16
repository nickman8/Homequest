package task

import (
	ch "donetick.com/core/internal/chore/model"
	c "donetick.com/core/internal/circle/model"
	"donetick.com/core/internal/homequest/family"
	p "donetick.com/core/internal/points"
	"errors"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Instance struct {
	ID       int `gorm:"primaryKey"`
	ChoreID  int `gorm:"index"`
	CircleID int `gorm:"index"`
	Pending  bool
	Closed   bool
	Version  int
}

func (Instance) TableName() string { return "hq_task_instances" }

type Attempt struct {
	ID           int  `gorm:"primaryKey"`
	InstanceID   int  `gorm:"index"`
	HistoryID    int  `gorm:"uniqueIndex"`
	LedgerID     *int `gorm:"uniqueIndex"`
	AcceptedAt   time.Time
	UserID       int
	Status       string
	PhotoID      string
	Approval     bool
	UndoLedgerID *int
}

func (Attempt) TableName() string { return "hq_attempts" }

type Item struct {
	ID     int
	Name   string
	Points int
	Closed bool
}
type Service struct{ DB *gorm.DB }

func New(db *gorm.DB) (*Service, error) {
	// Upgrade the first HomeQuest schema before AutoMigrate: retries are separate attempts.
	if db.Migrator().HasTable(&Attempt{}) {
		var indexes []struct {
			Name   string
			Unique bool
		}
		if db.Dialector.Name() == "sqlite" {
			if e := db.Raw(`SELECT name, "unique" FROM pragma_index_list('hq_attempts')`).Scan(&indexes).Error; e != nil {
				return nil, e
			}
		} else {
			ix, e := db.Migrator().GetIndexes(&Attempt{})
			if e != nil {
				return nil, e
			}
			for _, i := range ix {
				u, _ := i.Unique()
				indexes = append(indexes, struct {
					Name   string
					Unique bool
				}{i.Name(), u})
			}
		}
		for _, i := range indexes {
			if i.Name == "idx_hq_attempts_instance_id" && i.Unique {
				if e := db.Migrator().DropIndex(&Attempt{}, i.Name); e != nil {
					return nil, e
				}
			}
		}
	}
	e := db.AutoMigrate(&ch.Chore{}, &ch.ChoreHistory{}, &p.PointsHistory{}, &Instance{}, &Attempt{}, &Rule{})
	if e != nil {
		return nil, e
	}
	e = db.Exec("UPDATE hq_attempts SET status = 'accepted', user_id = (SELECT completed_by FROM chore_histories WHERE id = hq_attempts.history_id) WHERE (status = '' OR status IS NULL) AND ledger_id IS NOT NULL").Error
	return &Service{DB: db}, e
}

func (s *Service) Create(tg int64, circle int, name string, points int, assigned *int) (Item, error) {
	var out Item
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || points < 1 || points > 10000 {
		return out, errors.New("Название до 100 символов; баллы от 1 до 10000")
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		if assigned != nil {
			var n int64
			if e = tx.Model(&c.UserCircle{}).Where("circle_id = ? AND user_id = ? AND is_active = ?", circle, *assigned, true).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return family.ErrDenied
			}
		}
		chore := ch.Chore{Name: name, Points: &points, FrequencyType: ch.FrequencyTypeOnce, IsActive: true, CircleID: circle, CreatedBy: user.ID, AssignedTo: assigned}
		if e = tx.Create(&chore).Error; e != nil {
			return e
		}
		instance := Instance{ChoreID: chore.ID, CircleID: circle, Version: 1}
		if e = tx.Create(&instance).Error; e != nil {
			return e
		}
		out = Item{instance.ID, name, points, false}
		return nil
	})
	return out, e
}
func (s *Service) List(tg int64, circle int) ([]Item, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var rows []Item
	e := s.DB.Table("hq_task_instances i").Select("i.id, chores.name, chores.points, i.closed").Joins("JOIN chores ON chores.id = i.chore_id").Where("i.circle_id = ? AND chores.circle_id = ? AND i.closed = ? AND i.pending = ? AND chores.is_active = ?", circle, circle, false, false, true).Order("i.id").Limit(100).Scan(&rows).Error
	return rows, e
}
func (s *Service) Complete(tg int64, circle, id int, now time.Time) (Item, error) {
	a, e := s.Submit(tg, circle, id, "", now)
	if e != nil {
		return Item{}, e
	}
	var instance Instance
	var chore ch.Chore
	_ = s.DB.First(&instance, id).Error
	_ = s.DB.First(&chore, instance.ChoreID).Error
	points := 0
	if chore.Points != nil {
		points = *chore.Points
	}
	return Item{id, chore.Name, points, a.Status == "accepted"}, nil
}

func (s *Service) Balance(tg int64, circle int) (int, error) {
	user, e := (&family.Service{DB: s.DB}).Authorize(tg, circle)
	if e != nil {
		return 0, e
	}
	var sum int
	e = s.DB.Model(&p.PointsHistory{}).Select("COALESCE(SUM(CASE WHEN action = 0 THEN points ELSE -points END),0)").Where("user_id = ? AND circle_id = ?", user.ID, circle).Scan(&sum).Error
	return sum, e
}
func (s *Service) Edit(tg int64, circle, id int, name string, points int) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 || points < 1 || points > 10000 {
		return errors.New("Некорректные параметры")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		user, e := f.Authorize(tg, circle)
		if e != nil {
			return e
		}
		var instance Instance
		if tx.First(&instance, "id = ? AND circle_id = ? AND closed = ?", id, circle, false).Error != nil {
			return family.ErrDenied
		}
		var chore ch.Chore
		if tx.First(&chore, "id = ? AND circle_id = ?", instance.ChoreID, circle).Error != nil {
			return family.ErrDenied
		}
		home, e := f.View(tg, circle)
		if e != nil {
			return e
		}
		if chore.CreatedBy != user.ID && !home.Owner {
			return family.ErrDenied
		}
		if e = tx.Model(&chore).Updates(map[string]any{"name": name, "points": points}).Error; e != nil {
			return e
		}
		return tx.Model(&instance).Update("version", gorm.Expr("version + 1")).Error
	})
}
