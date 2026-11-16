package bot

import (
	"context"
	"donetick.com/core/internal/homequest/widget"
	"testing"
)

type contactAPI struct {
	blocked bool
	calls   map[int64]int
}

func (a *contactAPI) Call(_ context.Context, _ string, p any, _ any) error {
	uid := int64(p.(map[string]any)["chat_id"].(float64))
	a.calls[uid]++
	if uid == 900001 && a.blocked {
		return &widget.APIError{Code: 403}
	}
	return nil
}
func TestDeliveryUnavailableAndPrivateChatRecovery(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	api := &contactAPI{blocked: true, calls: map[int64]int{}}
	for i := 0; i < 3; i++ {
		if e := j.b.Dispatch(context.Background(), api); e != nil {
			t.Fatal(e)
		}
	}
	if api.calls[900001] != 1 || api.calls[900002] == 0 {
		t.Fatal("blocked delivery retried or affected other member", api.calls)
	}
	var c Contact
	j.db.First(&c, "viewer=?", 900001)
	if !c.Blocked {
		t.Fatal("unavailable state not persisted")
	}
	j.restart()
	// Replayed old update cannot reopen delivery.
	if e := j.b.Handle(Update{ID: 1, Message: &Message{From: Person{ID: 900001}, Chat: Chat{ID: 900001, Type: "private"}, Text: "/start"}}); e != nil {
		t.Fatal(e)
	}
	j.db.First(&c, "viewer=?", 900001)
	if !c.Blocked {
		t.Fatal("replay reopened delivery")
	}
	_ = j.b.say(900001, "Старое сообщение")
	_ = j.b.Dispatch(context.Background(), api)
	if api.calls[900001] != 1 {
		t.Fatal("unavailable contact called again")
	}
	api.blocked = false
	j.send(900001, "/start", "")
	_ = j.b.Dispatch(context.Background(), api)
	if api.calls[900001] != 2 {
		t.Fatal("recovery missing or replay avalanche", api.calls)
	}
	c = Contact{}
	j.db.First(&c, "viewer=?", 900001)
	if c.Blocked || c.LastDeliveredAt.IsZero() {
		t.Fatal("contact not recovered")
	}
}

type late403API struct {
	j     *journey
	fired bool
}

func (a *late403API) Call(_ context.Context, _ string, _ any, _ any) error {
	if !a.fired {
		a.fired = true
		a.j.send(900001, "/start", "")
		return &widget.APIError{Code: 403}
	}
	return nil
}
func TestOldDeliveryFailureCannotBlockNewPrivateUpdate(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	ok := &fakeAPI{}
	for i := 0; i < 3; i++ {
		_ = j.b.Dispatch(context.Background(), ok)
	}
	if e := j.b.say(900001, "Проверка"); e != nil {
		t.Fatal(e)
	}
	api := &late403API{j: j}
	if e := j.b.Dispatch(context.Background(), api); e != nil {
		t.Fatal(e)
	}
	var c Contact
	j.db.First(&c, "viewer=?", 900001)
	if c.Blocked {
		t.Fatal("old failure overrode new contact")
	}
	before := ok.calls
	if e := j.b.Dispatch(context.Background(), ok); e != nil {
		t.Fatal(e)
	}
	if ok.calls != before+1 {
		t.Fatal("new reply cancelled")
	}
}
