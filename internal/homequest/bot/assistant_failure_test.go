package bot

import (
	"context"
	"donetick.com/core/internal/homequest/assistant"
	"donetick.com/core/internal/homequest/task"
	"errors"
	"testing"
)

type brokenAssistant struct{ kind string }

func (f brokenAssistant) Transcribe(context.Context, []byte) (string, error) {
	return "", errors.New("private-provider-error")
}
func (f brokenAssistant) Parse(_ context.Context, text string, ids []assistant.Candidate, _ string) (assistant.Command, error) {
	if f.kind == "json" {
		return assistant.Decode("not json", text, ids)
	}
	return assistant.Command{}, context.DeadlineExceeded
}
func TestAssistantFailureKeepsManualCompletion(t *testing.T) {
	for _, kind := range []string{"stt", "timeout", "json"} {
		t.Run(kind, func(t *testing.T) {
			j := newJourney(t)
			const A int64 = 900001
			j.create(A, "Дом", "UTC")
			j.click(A, "Добавить задание")
			j.send(A, "Вынести мусор", "")
			j.send(A, "10", "")
			j.b.LLM = brokenAssistant{kind}
			j.b.STT = brokenAssistant{kind}
			j.b.VoiceSource = fixtureSpeech{}
			if kind == "stt" {
				j.seq++
				if e := j.b.Handle(Update{ID: j.seq, Message: &Message{From: Person{ID: A}, Chat: Chat{ID: A, Type: "private"}, Voice: &Voice{FileID: "fixture", Duration: 2}}}); e != nil {
					t.Fatal(e)
				}
			} else {
				j.send(A, "Я выкинул мусор", "")
			}
			j.click(A, "Понятно, продолжить")
			if e := j.b.ProcessAI(context.Background()); e != nil {
				t.Fatal(e)
			}
			var job AIJob
			j.db.First(&job)
			if job.State != "failed" || job.Text != "" || job.VoiceID != "" || job.Candidates != "" {
				t.Fatal("failed input retained", job.State)
			}
			h, _ := j.b.Family.Current(A)
			balance, e := (&task.Service{DB: j.db}).Balance(A, h.ID)
			if e != nil || balance != 0 {
				t.Fatal("failed AI changed points")
			}
			j.send(A, "/start", "")
			j.click(A, "Сегодня")
			j.click(A, "Выполнено")
			balance, e = (&task.Service{DB: j.db}).Balance(A, h.ID)
			if e != nil || balance != 10 {
				t.Fatal("manual path blocked", e)
			}
		})
	}
}
