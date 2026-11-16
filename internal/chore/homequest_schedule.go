package chore

import (
	"context"
	m "donetick.com/core/internal/chore/model"
	"errors"
	"time"
)

// NextAfterAcceptance reuses Donetick's scheduler while making HomeQuest's
// acceptance anchor, missed-period policy and DST resolution explicit.
func NextAfterAcceptance(ctx context.Context, c *m.Chore, accepted time.Time) (*time.Time, error) {
	copy := *c
	switch copy.FrequencyType {
	case m.FrequencyTypeOnce, m.FrequencyTypeNoRepeat:
		return nil, nil
	case m.FrequencyTypeDaily, m.FrequencyTypeDayOfTheWeek, m.FrequencyTypeInterval:
	default:
		return nil, errors.New("unsupported HomeQuest recurrence")
	}
	if copy.FrequencyMetadataV2 == nil || copy.FrequencyMetadataV2.Timezone == "" {
		return nil, errors.New("schedule timezone required")
	}
	loc, e := time.LoadLocation(copy.FrequencyMetadataV2.Timezone)
	if e != nil {
		return nil, e
	}
	if copy.FrequencyType == m.FrequencyTypeInterval && (copy.Frequency < 1 || copy.FrequencyMetadataV2.Unit == nil || *copy.FrequencyMetadataV2.Unit != "days") {
		return nil, errors.New("positive calendar day interval required")
	}
	base := accepted.In(loc)
	if copy.NextDueDate != nil {
		base = copy.NextDueDate.In(loc)
	}
	h, min, sec := base.Clock()
	if copy.IsRolling {
		h, min, sec = accepted.In(loc).Clock()
	} else if copy.FrequencyMetadataV2.Time != "" {
		t, e := time.Parse(time.RFC3339, copy.FrequencyMetadataV2.Time)
		if e != nil {
			return nil, e
		}
		h, min, sec = t.In(loc).Clock()
	}
	for n := 0; n < 10000; n++ {
		next, e := scheduleNextDueDate(ctx, &copy, accepted)
		if e != nil || next == nil {
			return next, e
		}
		d := next.In(loc)
		resolved, e := resolveLocal(d.Year(), d.Month(), d.Day(), h, min, sec, loc)
		if e != nil {
			return nil, e
		}
		if resolved.After(accepted) {
			v := resolved.UTC()
			return &v, nil
		}
		copy.NextDueDate = &resolved
		copy.IsRolling = false
	}
	return nil, errors.New("recurrence catch-up exceeds safe bound")
}

// Nonexistent local time advances to the first real local minute after it.
// Ambiguous local time chooses the earlier UTC occurrence, once only.
func resolveLocal(year int, month time.Month, day, hour, minute, second int, loc *time.Location) (time.Time, error) {
	nominal := time.Date(year, month, day, 12, 0, 0, 0, loc).UTC()
	start := nominal.Add(-36 * time.Hour).Truncate(time.Minute).Add(time.Duration(second) * time.Second)
	target := hour*60 + minute
	var after time.Time
	best := 24*60 + 1
	for i := 0; i < 72*60; i++ {
		utc := start.Add(time.Duration(i) * time.Minute)
		t := utc.In(loc)
		if t.Year() != year || t.Month() != month || t.Day() != day {
			continue
		}
		value := t.Hour()*60 + t.Minute()
		if value == target {
			return utc, nil
		}
		if value > target && value < best {
			best = value
			after = utc
		}
	}
	if !after.IsZero() {
		return after, nil
	}
	return time.Time{}, errors.New("local calendar date has no valid future time")
}
