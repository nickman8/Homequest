package widget

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestSpikeBoundActionAndReplay(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/test.db"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewSpike(db)
	if e != nil {
		t.Fatal(e)
	}
	card, e := s.Issue(900001, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Act(900002, card.Action, time.Now()); e == nil {
		t.Fatal("foreign viewer")
	}
	result, e := s.Act(900001, card.Action, time.Now())
	if e != nil || result.Status != "Подключение работает" {
		t.Fatal(result, e)
	}
	again, e := s.Act(900001, card.Action, time.Now())
	if e != nil || again != result {
		t.Fatal("replay", e)
	}
	if _, e = s.Act(900001, card.Action, time.Now().Add(25*time.Hour)); e == nil {
		t.Fatal("expired action")
	}
}
