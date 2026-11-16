package widget

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"time"
)

// Spike is a compatibility probe, NOT a family/task service. It stores no
// family data and must not be reused as authorization for domain actions.
type Spike struct{ db *gorm.DB }
type Probe struct {
	Token   string `gorm:"primaryKey"`
	Viewer  int64
	Expires time.Time
	Checked bool
}
type State struct {
	Status string `json:"status"`
	Action string `json:"action"`
}

func NewSpike(db *gorm.DB) (*Spike, error) { return &Spike{db}, db.AutoMigrate(&Probe{}) }
func (s *Spike) Issue(viewer int64, now time.Time) (State, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return State{}, err
	}
	p := Probe{Token: hex.EncodeToString(b), Viewer: viewer, Expires: now.Add(24 * time.Hour)}
	if err := s.db.Create(&p).Error; err != nil {
		return State{}, err
	}
	return State{Status: "Проверка подключения", Action: p.Token}, nil
}
func (s *Spike) Act(viewer int64, token string, now time.Time) (State, error) {
	if len(token) != 32 {
		return State{}, errors.New("invalid action")
	}
	r := s.db.Model(&Probe{}).Where("token = ? AND viewer = ? AND expires > ?", token, viewer, now).Update("checked", true)
	if r.Error != nil {
		return State{}, r.Error
	}
	if r.RowsAffected != 1 {
		return State{}, errors.New("action unavailable")
	}
	return State{Status: "Подключение работает", Action: token}, nil
}
