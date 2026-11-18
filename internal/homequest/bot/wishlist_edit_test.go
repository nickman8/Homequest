package bot

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/wishlist"
	"encoding/json"
	"strings"
	"testing"
)

func TestWishWizardEditArchiveAndStaleButtons(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Тёплый дом", "Europe/Berlin")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Добавить желание")
	j.send(900001, "Книга", "")
	j.send(900001, "javascript:alert(1)", "")
	step, _ := j.b.Family.Step(900001)
	if step.Step != "wish_link" {
		t.Fatal("invalid link advanced wizard", step.Step)
	}
	j.send(900001, "https://example.org/book", "")
	j.restart()
	j.click(900001, "Назад")
	j.click(900001, "Пропустить")
	j.send(900001, "Бумажная", "")
	j.send(900001, "1000 ₽", "")
	var count int64
	j.db.Model(&wishlist.Item{}).Count(&count)
	if count != 0 {
		t.Fatal("unconfirmed wish created")
	}
	save := j.token(900001, "Сохранить")
	j.send(900001, "", save)
	j.send(900001, "", save)
	j.db.Model(&wishlist.Item{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate wish", count)
	}
	j.click(900001, "Подарки")
	before := projection(j.card(900001))
	j.click(900002, "Подарки")
	j.click(900002, "Забронировать тайно")
	oldPurchase := j.token(900002, "Уже куплено")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	if projection(j.card(900001)) != before {
		t.Fatal("claim disclosed to recipient")
	}
	j.click(900001, "Править")
	j.send(900001, "Другая книга", "")
	j.click(900001, "Оставить")
	j.click(900001, "Оставить")
	j.click(900001, "Оставить")
	j.click(900001, "Сохранить")
	j.send(900002, "", oldPurchase)
	var claim wishlist.ItemClaim
	j.db.First(&claim)
	if claim.Purchased {
		t.Fatal("old purchase action accepted changed wish")
	}
	j.send(900002, "/start", "")
	j.click(900002, "Подарки")
	if !strings.Contains(j.card(900002).Text, "обновлено") {
		t.Fatal("donor not told wish changed")
	}
	j.click(900002, "Уже куплено")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Убрать")
	j.click(900001, "Назад")
	j.click(900001, "Убрать")
	remove := j.token(900001, "Да, убрать")
	j.send(900001, "", remove)
	j.send(900001, "", remove)
	j.send(900002, "/start", "")
	j.click(900002, "Подарки")
	if !strings.Contains(j.card(900002).Text, "Добавьте желание") {
		t.Fatal("archived wish remains")
	}
	j.db.Model(&wishlist.Item{}).Where("archived=?", true).Count(&count)
	if count != 1 {
		t.Fatal("missing archive")
	}
	// A save button from a cancelled draft must not confirm the next draft.
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Добавить желание")
	j.send(900001, "Первое | | |", "")
	oldSave := j.token(900001, "Сохранить")
	j.click(900001, "Отмена")
	j.click(900001, "Добавить желание")
	j.send(900001, "Второе | | |", "")
	j.send(900001, "", oldSave)
	j.db.Model(&wishlist.Item{}).Count(&count)
	if count != 1 {
		t.Fatal("stale draft saved newer wish")
	}
	j.click(900001, "Сохранить")
	j.db.Model(&wishlist.Item{}).Count(&count)
	if count != 2 {
		t.Fatal("current draft missing")
	}
}
func projection(c Card) string {
	for i := range c.Buttons {
		c.Buttons[i].Action = ""
	}
	raw, _ := json.Marshal(c)
	return string(raw)
}

func TestGiftSecrecyAcrossProductPayloads(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Тёплый дом", "UTC")
	invite := j.invite(900001)
	for _, uid := range []int64{900002, 900003} {
		j.send(uid, "/start join_"+invite, "")
	}
	j.create(900004, "Другой дом", "UTC")
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	j.click(900001, "Добавить желание")
	j.send(900001, "Сюрприз-только-для-подарка | https://example.org/gift | Заметка | 1500", "")
	j.click(900001, "Сохранить")
	before := projection(j.card(900001))
	j.click(900001, "Подробнее")
	var infoBefore Delivery
	j.db.Where("viewer=?", 900001).Order("id DESC").First(&infoBefore)
	j.send(900002, "/start", "")
	j.click(900002, "Подарки")
	bToken := j.token(900002, "Забронировать тайно")
	j.send(900003, "/start", "")
	j.click(900003, "Подарки")
	cToken := j.token(900003, "Забронировать тайно")
	// Both buttons were issued before either reservation. Requests enter real handlers.
	errs := make(chan error, 2)
	j.seq += 2
	for i, who := range []struct {
		uid   int64
		token string
	}{{900002, bToken}, {900003, cToken}} {
		go func(id int64, uid int64, token string) {
			errs <- j.b.Handle(Update{ID: id, Query: &Query{ID: "race", From: Person{ID: uid}, Chat: Chat{ID: uid, Type: "private"}, MessageID: 1, Data: token}})
		}(j.seq-int64(i), who.uid, who.token)
	}
	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
	var count int64
	j.db.Model(&wishlist.ItemClaim{}).Count(&count)
	if count != 1 {
		t.Fatal("claim race", count)
	}
	j.send(900001, "/start", "")
	j.click(900001, "Подарки")
	if before != projection(j.card(900001)) {
		t.Fatal("widget owner response changed with claim")
	}
	j.click(900001, "Подробнее")
	var infoAfter Delivery
	j.db.Where("viewer=?", 900001).Order("id DESC").First(&infoAfter)
	if infoBefore.Payload != infoAfter.Payload {
		t.Fatal("ordinary message disclosed claim")
	}
	j.send(900004, "", bToken)
	var foreign Delivery
	j.db.Where("viewer=?", 900004).Order("id DESC").First(&foreign)
	if strings.Contains(foreign.Payload, "Сюрприз") {
		t.Fatal("foreign response disclosed item")
	}
	j.send(900001, "/task", "")
	j.send(900001, "Вынести мусор", "")
	j.send(900001, "10", "")
	llm := &fixtureLLM{command: assistant.Command{Action: "clarify"}}
	j.b.LLM = llm
	j.send(900001, "Что мне сделать?", "")
	var consent Action
	j.db.Where("viewer=? AND kind LIKE ?", 900001, "ai_consent:%").First(&consent)
	j.send(900001, "", consent.Token)
	if e := j.b.ProcessAI(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(llm.ids) != 1 || llm.ids[0].Title != "Вынести мусор" {
		t.Fatal("unexpected AI context", llm.ids)
	}
	raw, _ := json.Marshal(llm.ids)
	if strings.Contains(string(raw), "Сюрприз") || strings.Contains(string(raw), "claim") {
		t.Fatal("gift details in AI context")
	}
}
