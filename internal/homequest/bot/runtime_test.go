package bot

import (
	"context"
	"donetick.com/core/internal/homequest/widget"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

type fakeAPI struct {
	calls int
	fail  bool
}

func (f *fakeAPI) Call(context.Context, string, any, any) error {
	f.calls++
	if f.fail {
		return &widget.APIError{Code: 429, RetryAfter: 60}
	}
	return nil
}
func TestOutboxBoundedRetryAndRestart(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(t.TempDir()+"/out.db"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := New(db, "<h3>Дом</h3>", "test")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	b.Now = func() time.Time { return now }
	if e = b.say(900001, "Привет"); e != nil {
		t.Fatal(e)
	}
	api := &fakeAPI{fail: true}
	if e = b.Dispatch(context.Background(), api); e != nil {
		t.Fatal(e)
	}
	if api.calls != 1 {
		t.Fatal(api.calls)
	}
	_ = b.Dispatch(context.Background(), api)
	if api.calls != 1 {
		t.Fatal("retry flood")
	}
	now = now.Add(time.Minute)
	api.fail = false
	_ = b.Dispatch(context.Background(), api)
	_ = b.Dispatch(context.Background(), api)
	if api.calls != 2 {
		t.Fatal("duplicate delivery")
	}
}
func TestConflictStopsPolling(t *testing.T) {
	if !errors.Is(terminalPolling(&widget.APIError{Code: 409}), ErrPollingConflict) {
		t.Fatal("409 must stop")
	}
}
