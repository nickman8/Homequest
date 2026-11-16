package task

import (
	"donetick.com/core/internal/homequest/family"
	l "donetick.com/core/internal/label/model"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// Names and chore associations use upstream labels/chore_labels. Room adds a
// family-scoped canonical key and identifies which labels represent rooms.
type Room struct {
	ID       int    `gorm:"primaryKey"`
	CircleID int    `gorm:"uniqueIndex:hq_room_key"`
	Key      string `gorm:"uniqueIndex:hq_room_key"`
	LabelID  int    `gorm:"uniqueIndex"`
	Name     string `gorm:"->;-:migration"`
}

func (Room) TableName() string { return "hq_rooms" }

type PresetImport struct {
	CircleID  int    `gorm:"primaryKey;autoIncrement:false"`
	Key       string `gorm:"primaryKey"`
	CreatedAt time.Time
}

func (PresetImport) TableName() string { return "hq_preset_imports" }

type PresetTask struct {
	Key, Name                   string
	Points, Every, Hour, Minute int
	Enabled                     bool
}

var PresetNames = map[string]string{"kitchen": "Кухня", "bathroom": "Ванная", "daily": "Повседневные дела"}

func PresetTasks(key string) []PresetTask {
	switch key {
	case "kitchen":
		return []PresetTask{{"surfaces", "Протереть поверхности", 5, 1, 19, 0, true}, {"sink", "Помыть раковину", 5, 2, 19, 0, true}, {"fridge", "Проверить продукты в холодильнике", 10, 7, 19, 0, true}}
	case "bathroom":
		return []PresetTask{{"basin", "Помыть раковину в ванной", 5, 3, 19, 0, true}, {"mirror", "Протереть зеркало", 5, 7, 19, 0, true}, {"floor", "Помыть пол в ванной", 10, 7, 19, 0, true}}
	case "daily":
		return []PresetTask{{"trash", "Вынести мусор", 10, 2, 19, 0, true}, {"laundry", "Разобрать чистое бельё", 10, 3, 19, 0, true}, {"plants", "Проверить, нужен ли полив растениям", 5, 7, 19, 0, true}}
	}
	return nil
}
func (s *Service) Rooms(tg int64, circle int) ([]Room, error) {
	if _, e := (&family.Service{DB: s.DB}).Authorize(tg, circle); e != nil {
		return nil, e
	}
	var rooms []Room
	e := s.DB.Table("hq_rooms r").Select("r.*, labels.name").Joins("JOIN labels ON labels.id=r.label_id").Where("r.circle_id=?", circle).Order("r.id").Scan(&rooms).Error
	return rooms, e
}
func (s *Service) CreateRoom(tg int64, circle int, name string) (Room, error) {
	var out Room
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 40 {
		return out, errors.New("Название комнаты: 1–40 символов")
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		u, e := (&family.Service{DB: tx}).Authorize(tg, circle)
		if e != nil {
			return e
		}
		key := strings.ToLower(name)
		e = tx.First(&out, "circle_id=? AND key=?", circle, key).Error
		if e == nil {
			var label l.Label
			if e = tx.First(&label, out.LabelID).Error; e != nil {
				return e
			}
			out.Name = label.Name
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		label := l.Label{Name: name, IsShared: true, CircleID: &circle, CreatedBy: u.ID}
		if e = tx.Create(&label).Error; e != nil {
			return e
		}
		out = Room{CircleID: circle, Key: key, LabelID: label.ID, Name: name}
		return tx.Create(&out).Error
	})
	return out, e
}
func (s *Service) roomMutation(tg int64, circle, id int, name *string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		f := &family.Service{DB: tx}
		u, e := f.Authorize(tg, circle)
		if e != nil {
			return e
		}
		h, e := f.View(tg, circle)
		if e != nil {
			return e
		}
		var room Room
		if tx.First(&room, "id=? AND circle_id=?", id, circle).Error != nil {
			return family.ErrDenied
		}
		var label l.Label
		if e = tx.First(&label, room.LabelID).Error; e != nil {
			return e
		}
		if !h.Owner && label.CreatedBy != u.ID {
			return family.ErrDenied
		}
		if name == nil {
			if e = tx.Table("chore_labels").Where("label_id=?", label.ID).Delete(nil).Error; e != nil {
				return e
			}
			if e = tx.Delete(&label).Error; e != nil {
				return e
			}
			return tx.Delete(&room).Error
		}
		value := strings.TrimSpace(*name)
		if value == "" || len([]rune(value)) > 40 {
			return errors.New("Название комнаты: 1–40 символов")
		}
		if e = tx.Model(&room).Update("key", strings.ToLower(value)).Error; e != nil {
			return errors.New("Комната с таким названием уже есть")
		}
		return tx.Model(&label).Update("name", value).Error
	})
}
func (s *Service) RenameRoom(tg int64, circle, id int, name string) error {
	return s.roomMutation(tg, circle, id, &name)
}
func (s *Service) DeleteRoom(tg int64, circle, id int) error {
	return s.roomMutation(tg, circle, id, nil)
}
func (s *Service) SetRoom(tg int64, circle, instance, roomID int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		i, _, e := editable(tx, tg, circle, instance)
		if e != nil {
			return e
		}
		var room Room
		if roomID != 0 && tx.First(&room, "id=? AND circle_id=?", roomID, circle).Error != nil {
			return family.ErrDenied
		}
		if e = tx.Exec("DELETE FROM chore_labels WHERE chore_id=? AND label_id IN (SELECT label_id FROM hq_rooms WHERE circle_id=?)", i.ChoreID, circle).Error; e != nil {
			return e
		}
		if roomID != 0 {
			if e = tx.Exec("INSERT INTO chore_labels(chore_id,label_id) VALUES(?,?)", i.ChoreID, room.LabelID).Error; e != nil {
				return e
			}
		}
		return tx.Model(&i).Update("version", gorm.Expr("version+1")).Error
	})
}
func (s *Service) RoomTasks(tg int64, circle, roomID int) ([]Item, error) {
	rows, e := s.List(tg, circle)
	if e != nil {
		return nil, e
	}
	var room Room
	if s.DB.First(&room, "id=? AND circle_id=?", roomID, circle).Error != nil {
		return nil, family.ErrDenied
	}
	var ids []int
	if e = s.DB.Table("hq_task_instances i").Select("i.id").Joins("JOIN chore_labels l ON l.chore_id=i.chore_id").Where("i.circle_id=? AND l.label_id=?", circle, room.LabelID).Scan(&ids).Error; e != nil {
		return nil, e
	}
	found := map[int]bool{}
	for _, id := range ids {
		found[id] = true
	}
	out := []Item{}
	for _, r := range rows {
		if found[r.ID] {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *Service) ApplyPreset(tg int64, circle int, key string, choices []PresetTask, now time.Time) ([]int, error) {
	ids := []int{}
	name, ok := PresetNames[key]
	if !ok || len(choices) < 1 || len(choices) > 3 {
		return ids, errors.New("Набор недоступен")
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		if _, e := (&family.Service{DB: tx}).Authorize(tg, circle); e != nil {
			return e
		}
		marker := PresetImport{CircleID: circle, Key: key, CreatedAt: now}
		r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return nil
		}
		child := &Service{DB: tx}
		room, e := child.CreateRoom(tg, circle, name)
		if e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, v := range choices {
			valid := false
			for _, p := range PresetTasks(key) {
				if p.Key == v.Key {
					valid = true
				}
			}
			if !valid || seen[v.Key] {
				return errors.New("Некорректный состав набора")
			}
			seen[v.Key] = true
			if !v.Enabled {
				continue
			}
			item, e := child.Create(tg, circle, v.Name, v.Points, nil)
			if e != nil {
				return e
			}
			if e = child.SetSchedule(tg, circle, item.ID, Schedule{Mode: "interval", Every: v.Every, Hour: v.Hour, Minute: v.Minute}, now); e != nil {
				return e
			}
			if e = child.SetRoom(tg, circle, item.ID, room.ID); e != nil {
				return e
			}
			ids = append(ids, item.ID)
		}
		if len(ids) == 0 {
			return errors.New("Выберите хотя бы одно дело")
		}
		return nil
	})
	return ids, e
}
