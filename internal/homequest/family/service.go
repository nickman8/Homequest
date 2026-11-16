// Package family adapts upstream users/circles. Telegram is the identity provider;
// no shared admin token and no password chosen by a family member.
package family

import (
	"crypto/rand"
	"crypto/sha256"
	c "donetick.com/core/internal/circle/model"
	u "donetick.com/core/internal/user/model"
	"encoding/hex"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrDenied = errors.New("Действие недоступно. Откройте /start и проверьте активный дом.")

type Identity struct {
	TelegramID int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID     int   `gorm:"uniqueIndex"`
	Step       string
	Draft      string
}

func (Identity) TableName() string { return "hq_identities" }

type Settings struct {
	CircleID int `gorm:"primaryKey;autoIncrement:false"`
	Timezone string
}

func (Settings) TableName() string { return "hq_family_settings" }

type Invitation struct {
	Hash     string `gorm:"primaryKey"`
	CircleID int    `gorm:"index"`
	Expires  time.Time
	Revoked  bool
}

func (Invitation) TableName() string { return "hq_invitations" }

type Service struct{ DB *gorm.DB }
type View struct {
	ID       int
	Name     string
	Timezone string
	Owner    bool
	Members  []Member
}
type Member struct {
	ID    int
	Name  string
	Owner bool
}

func New(db *gorm.DB) (*Service, error) {
	// Same upstream schema and GORM migration mechanism; no duplicate family DB.
	e := db.AutoMigrate(&u.User{}, &c.Circle{}, &c.UserCircle{}, &Identity{}, &Settings{}, &Invitation{})
	return &Service{DB: db}, e
}
func (s *Service) Start(tg int64, name string) error {
	if tg <= 0 {
		return ErrDenied
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var id Identity
		e := tx.First(&id, "telegram_id = ?", tg).Error
		if e == nil {
			return tx.Model(&u.User{}).Where("id = ?", id.UserID).Updates(map[string]any{"display_name": name, "chat_id": tg}).Error
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		user := u.User{Username: fmt.Sprintf("hq_tg_%d", tg), DisplayName: name, ChatID: tg, Provider: u.AuthProviderOAuth2}
		if e = tx.Create(&user).Error; e != nil {
			return e
		}
		return tx.Create(&Identity{TelegramID: tg, UserID: user.ID}).Error
	})
}
func actor(tx *gorm.DB, tg int64) (u.User, error) {
	var id Identity
	var user u.User
	if tx.First(&id, "telegram_id = ?", tg).Error != nil {
		return user, ErrDenied
	}
	if tx.First(&user, "id = ? AND disabled = ?", id.UserID, false).Error != nil {
		return user, ErrDenied
	}
	return user, nil
}
func membership(tx *gorm.DB, tg int64, circle int, owner bool) (u.User, error) {
	user, e := actor(tx, tg)
	if e != nil {
		return user, e
	}
	if circle == 0 || user.CircleID != circle {
		return user, ErrDenied
	}
	var m c.UserCircle
	if tx.First(&m, "user_id = ? AND circle_id = ? AND is_active = ?", user.ID, circle, true).Error != nil {
		return user, ErrDenied
	}
	var home c.Circle
	if tx.First(&home, "id = ? AND disabled = ?", circle, false).Error != nil {
		return user, ErrDenied
	}
	if owner && m.Role != c.UserRoleAdmin {
		return user, ErrDenied
	}
	return user, nil
}
func (s *Service) Create(tg int64, name, zone string) (View, error) {
	var out View
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return out, errors.New("Название: от 1 до 64 символов")
	}
	if zone == "Local" || zone == "" {
		return out, errors.New("Укажите IANA timezone, например Europe/Berlin")
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return out, errors.New("Неизвестная IANA timezone")
	}
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if user.CircleID != 0 {
			return ErrDenied
		}
		home := c.Circle{Name: name, CreatedBy: user.ID}
		if e = tx.Create(&home).Error; e != nil {
			return e
		}
		if e = tx.Create(&c.UserCircle{UserID: user.ID, CircleID: home.ID, Role: c.UserRoleAdmin, IsActive: true}).Error; e != nil {
			return e
		}
		if e = tx.Create(&Settings{CircleID: home.ID, Timezone: zone}).Error; e != nil {
			return e
		}
		if e = tx.Model(&user).Update("circle_id", home.ID).Error; e != nil {
			return e
		}
		out = View{ID: home.ID, Name: name, Timezone: zone, Owner: true}
		return nil
	})
	return out, e
}
func (s *Service) Current(tg int64) (View, error) {
	user, e := actor(s.DB, tg)
	if e != nil {
		return View{}, e
	}
	return s.View(tg, user.CircleID)
}
func (s *Service) View(tg int64, id int) (View, error) {
	var out View
	e := s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := membership(tx, tg, id, false)
		if e != nil {
			return e
		}
		var home c.Circle
		var settings Settings
		var rows []struct {
			UserID      int
			DisplayName string
			Role        c.UserRole
		}
		if e = tx.First(&home, id).Error; e != nil {
			return e
		}
		if e = tx.First(&settings, "circle_id = ?", id).Error; e != nil {
			return e
		}
		if e = tx.Table("user_circles").Select("user_circles.user_id, users.display_name, user_circles.role").Joins("JOIN users ON users.id = user_circles.user_id").Where("user_circles.circle_id = ? AND is_active = ?", id, true).Order("user_circles.user_id").Scan(&rows).Error; e != nil {
			return e
		}
		out = View{ID: id, Name: home.Name, Timezone: settings.Timezone}
		for _, r := range rows {
			out.Members = append(out.Members, Member{r.UserID, r.DisplayName, r.Role == c.UserRoleAdmin})
			if r.UserID == user.ID {
				out.Owner = r.Role == c.UserRoleAdmin
			}
		}
		return nil
	})
	return out, e
}
func randomToken() (string, error) {
	b := make([]byte, 24)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func digest(token string) string { b := sha256.Sum256([]byte(token)); return hex.EncodeToString(b[:]) }
func (s *Service) Invite(tg int64, now time.Time) (string, error) {
	token, e := randomToken()
	if e != nil {
		return "", e
	}
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, true); e != nil {
			return e
		}
		return tx.Create(&Invitation{Hash: digest(token), CircleID: user.CircleID, Expires: now.Add(7 * 24 * time.Hour)}).Error
	})
	return token, e
}
func (s *Service) Join(tg int64, token string, now time.Time) error {
	if len(token) != 48 {
		return ErrDenied
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		var invite Invitation
		if tx.First(&invite, "hash = ? AND revoked = ? AND expires > ?", digest(token), false, now).Error != nil {
			return ErrDenied
		}
		if user.CircleID != 0 && user.CircleID != invite.CircleID {
			return errors.New("Сначала явно выйдите из текущего дома; контекст не переключён")
		}
		var home c.Circle
		if tx.First(&home, "id = ? AND disabled = ?", invite.CircleID, false).Error != nil {
			return ErrDenied
		}
		row := c.UserCircle{UserID: user.ID, CircleID: invite.CircleID, Role: c.UserRoleMember, IsActive: true}
		if e = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "circle_id"}}, DoUpdates: clause.Assignments(map[string]any{"is_active": true})}).Create(&row).Error; e != nil {
			return e
		}
		return tx.Model(&user).Update("circle_id", invite.CircleID).Error
	})
}
func (s *Service) Revoke(tg int64) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, true); e != nil {
			return e
		}
		return tx.Model(&Invitation{}).Where("circle_id = ?", user.CircleID).Update("revoked", true).Error
	})
}
func (s *Service) Leave(tg int64) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, false); e != nil {
			return e
		}
		var m c.UserCircle
		if e = tx.First(&m, "user_id = ? AND circle_id = ?", user.ID, user.CircleID).Error; e != nil {
			return e
		}
		if m.Role == c.UserRoleAdmin {
			var n int64
			if e = tx.Model(&c.UserCircle{}).Where("circle_id = ? AND role = ? AND is_active = ?", user.CircleID, c.UserRoleAdmin, true).Count(&n).Error; e != nil {
				return e
			}
			if n < 2 {
				return errors.New("Перед выходом передайте управление другому участнику")
			}
		}
		if e = tx.Model(&m).Update("is_active", false).Error; e != nil {
			return e
		}
		return tx.Model(&user).Update("circle_id", 0).Error
	})
}
func (s *Service) SetStep(tg int64, step, draft string) error {
	return s.DB.Model(&Identity{}).Where("telegram_id = ?", tg).Updates(map[string]any{"step": step, "draft": draft}).Error
}
func (s *Service) Step(tg int64) (Identity, error) {
	var id Identity
	e := s.DB.First(&id, "telegram_id = ?", tg).Error
	return id, e
}

// Authorize rechecks membership on the caller's transaction, never a cached card.
func (s *Service) Authorize(tg int64, circle int) (u.User, error) {
	return membership(s.DB, tg, circle, false)
}

func (s *Service) Transfer(tg int64, target int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, true); e != nil {
			return e
		}
		if target == user.ID {
			return nil
		}
		var to c.UserCircle
		if tx.First(&to, "user_id = ? AND circle_id = ? AND is_active = ?", target, user.CircleID, true).Error != nil {
			return ErrDenied
		}
		if e = tx.Model(&to).Update("role", c.UserRoleAdmin).Error; e != nil {
			return e
		}
		return tx.Model(&c.UserCircle{}).Where("user_id = ? AND circle_id = ?", user.ID, user.CircleID).Update("role", c.UserRoleMember).Error
	})
}
func (s *Service) Remove(tg int64, target int) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, true); e != nil {
			return e
		}
		var member c.UserCircle
		if tx.First(&member, "user_id = ? AND circle_id = ? AND is_active = ?", target, user.CircleID, true).Error != nil {
			return ErrDenied
		}
		if member.Role == c.UserRoleAdmin {
			return errors.New("Сначала передайте управление")
		}
		if e = tx.Model(&member).Update("is_active", false).Error; e != nil {
			return e
		}
		return tx.Model(&u.User{}).Where("id = ? AND circle_id = ?", target, user.CircleID).Update("circle_id", 0).Error
	})
}
func (s *Service) SetTimezone(tg int64, zone string) error {
	if zone == "" || zone == "Local" {
		return errors.New("Укажите IANA timezone")
	}
	if _, e := time.LoadLocation(zone); e != nil {
		return errors.New("Неизвестная IANA timezone")
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		user, e := actor(tx, tg)
		if e != nil {
			return e
		}
		if _, e = membership(tx, tg, user.CircleID, true); e != nil {
			return e
		}
		return tx.Model(&Settings{}).Where("circle_id = ?", user.CircleID).Update("timezone", zone).Error
	})
}
