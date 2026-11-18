package chore

import (
	"context"
	m "donetick.com/core/internal/chore/model"
	"testing"
	"time"
)

func TestHomeQuestLateAcceptanceAndDST(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	day := "tuesday"
	due := time.Date(2026, 3, 3, 19, 0, 0, 0, loc)
	c := m.Chore{FrequencyType: m.FrequencyTypeDayOfTheWeek, NextDueDate: &due, FrequencyMetadataV2: &m.FrequencyMetadata{Timezone: "Europe/Berlin", Time: due.Format(time.RFC3339), Days: []*string{&day}}}
	accepted := time.Date(2026, 4, 8, 23, 0, 0, 0, loc)
	next, e := NextAfterAcceptance(context.Background(), &c, accepted)
	if e != nil {
		t.Fatal(e)
	}
	want := time.Date(2026, 4, 14, 19, 0, 0, 0, loc)
	if next == nil || !next.Equal(want) {
		t.Fatal(next, want)
	}
	for _, tc := range []struct{ due, accepted, want time.Time }{
		{time.Date(2026, 3, 28, 2, 30, 0, 0, loc), time.Date(2026, 3, 28, 10, 0, 0, 0, loc), time.Date(2026, 3, 29, 3, 0, 0, 0, loc)},
		{time.Date(2026, 10, 24, 2, 30, 0, 0, loc), time.Date(2026, 10, 24, 10, 0, 0, 0, loc), time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC)},
	} {
		c := m.Chore{FrequencyType: m.FrequencyTypeDaily, NextDueDate: &tc.due, FrequencyMetadataV2: &m.FrequencyMetadata{Timezone: "Europe/Berlin"}}
		got, e := NextAfterAcceptance(context.Background(), &c, tc.accepted)
		if e != nil || got == nil || !got.Equal(tc.want) {
			t.Fatal(got, tc.want, e)
		}
	}
	unit := "days"
	c = m.Chore{FrequencyType: m.FrequencyTypeInterval, Frequency: 2, IsRolling: true, NextDueDate: &due, FrequencyMetadataV2: &m.FrequencyMetadata{Timezone: "Europe/Berlin", Time: due.Format(time.RFC3339), Unit: &unit}}
	next, e = NextAfterAcceptance(context.Background(), &c, accepted)
	want = accepted.AddDate(0, 0, 2)
	if e != nil || !next.Equal(want) {
		t.Fatal(next, want, e)
	}
}
