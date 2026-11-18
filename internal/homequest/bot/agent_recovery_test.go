package bot

import (
	"donetick.com/core/internal/homequest/agent"
	"donetick.com/core/internal/homequest/agent/agenttest"
	"donetick.com/core/internal/homequest/wishlist"
	"encoding/json"
	"testing"
	"time"
)

func TestInterruptedReasoningRetriesOnceWhileCurrent(t *testing.T) {
	j := newJourney(t)
	now := j.b.Now()
	jobs := []AIJob{
		{Viewer: 900001, Agent: true, ConsentScope: agentConsentScope, State: "processing", CreatedAt: now, Text: "Свежая просьба"},
		{Viewer: 900001, Agent: true, ConsentScope: agentConsentScope, State: "processing", CreatedAt: now.Add(-16 * time.Minute), VoiceID: "expired"},
		{Viewer: 900001, Agent: true, ConsentScope: "old", State: "processing", CreatedAt: now, VoiceID: "obsolete"},
		{Viewer: 900001, Agent: true, ConsentScope: agentConsentScope, State: "confirm", CreatedAt: now, PlanID: "accepted-preview"},
	}
	if err := j.db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	check := func(i int, state string) {
		t.Helper()
		var job AIJob
		if err := j.db.First(&job, jobs[i].ID).Error; err != nil || job.State != state {
			t.Fatal(i, job.State, err)
		}
		if state == "failed" && (job.VoiceID != "" || job.Text != "") {
			t.Fatal("abandoned inputs retained")
		}
	}
	if err := j.b.recoverInterruptedAI(); err != nil {
		t.Fatal(err)
	}
	check(0, "queued")
	check(1, "failed")
	check(2, "failed")
	check(3, "confirm")
	if err := j.db.Model(&jobs[0]).Update("state", "processing").Error; err != nil {
		t.Fatal(err)
	}
	if err := j.b.recoverInterruptedAI(); err != nil {
		t.Fatal(err)
	}
	check(0, "failed")
}

func TestSecretGiftConfirmationLeavesNoModelMemory(t *testing.T) {
	j, id, _ := agentJourney(t)
	const donor = 900001
	const recipient = 900002
	if err := j.b.Family.Start(recipient, "Участник"); err != nil {
		t.Fatal(err)
	}
	invite, err := j.b.Family.Invite(donor, j.b.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = j.b.Family.Join(recipient, invite, j.b.Now()); err != nil {
		t.Fatal(err)
	}
	ws := wishlist.Service{DB: j.db}
	if _, err = ws.Create(recipient, id, "Атлас", "", "", ""); err != nil {
		t.Fatal(err)
	}
	j.b.Agent = &agent.Eino{Model: &agenttest.Model{Operation: "wish_claim", Search: "Атлас"}}
	memory := func() string {
		t.Helper()
		c, err := (&agent.Service{DB: j.db}).Context(donor, j.b.Now())
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(c.History)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := memory()
	j.send(donor, "Тайно забронируй атлас", "")
	j.click(donor, "Понятно, продолжить")
	process(t, j)
	if memory() != before {
		t.Fatal("secret request leaked into future model context")
	}
	confirmAgent(t, j)
	rows, err := ws.List(donor, id)
	if err != nil || len(rows) != 1 || rows[0].Claim == nil || !rows[0].Claim.Mine {
		t.Fatal("claim did not succeed", err)
	}
	if memory() != before {
		t.Fatal("private confirmation changed model memory")
	}
}
