package bot

import (
	"context"
	"donetick.com/core/internal/homequest/task"
	"testing"
)

func TestHandoffNotificationAndClosedOfferInvalidation(t *testing.T) {
	j := newJourney(t)
	j.create(900001, "Дом", "UTC")
	invite := j.invite(900001)
	j.send(900002, "/start join_"+invite, "")
	h, _ := j.b.Family.Current(900001)
	a, _ := j.b.Family.Authorize(900001, h.ID)
	b, _ := j.b.Family.Authorize(900002, h.ID)
	s := &task.Service{DB: j.db}
	item, _ := s.Create(900001, h.ID, "Мусор", 10, &a.ID)
	j.send(900001, "/start", "")
	j.click(900001, "Сегодня")
	j.click(900001, "Параметры")
	j.click(900001, "Комната и очередь")
	j.click(900001, "Предложить передачу")
	j.click(900001, "Предложить")
	var row Delivery
	if e := j.db.Where("handoff_id<>?", 0).First(&row).Error; e != nil {
		t.Fatal("no recipient notification", e)
	}
	if row.Viewer != 900002 || row.CircleID != h.ID {
		t.Fatal("wrong recipient")
	}
	offers, _ := s.Handoffs(900002, h.ID, j.now)
	if len(offers) != 1 || offers[0].ToID != b.ID {
		t.Fatal(offers)
	}
	s.Submit(900001, h.ID, item.ID, "", j.now)
	j.b.Dispatch(context.Background(), &fakeAPI{})
	if ready, e := j.b.noticeReady(row); e != nil || ready {
		t.Fatal("closed offer notice", ready, e)
	}
}
