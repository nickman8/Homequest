package assistant

import (
	"context"
	"donetick.com/core/internal/homequest/shopping"
	"donetick.com/core/internal/homequest/task"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

type Candidate struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Version int    `json:"version"`
}
type Draft struct {
	Title    string        `json:"title"`
	Points   int           `json:"points"`
	Schedule task.Schedule `json:"schedule"`
}
type Command struct {
	Shopping []shopping.Input `json:"shopping,omitempty"`
	Action   string           `json:"action"`
	IDs      []int            `json:"ids,omitempty"`
	Draft    *Draft           `json:"draft,omitempty"`
}
type LLM interface {
	Parse(context.Context, string, []Candidate, string) (Command, error)
}
type STT interface {
	Transcribe(context.Context, []byte) (string, error)
}

var unsafeText = regexp.MustCompile(`(?i)(^|[^а-яё])(не|нет|завтра|вчера|надо|нужно|кажется|возможно|наверное|если|планирую|собираюсь|потом|позже|буду|и)([^а-яё]|$)|[?？]`)

func (c *Command) Validate(text string, candidates []Candidate) error {
	denied := errors.New("Нужно уточнение; действие не выполнено")
	if c.Action != "shopping_draft" && len(c.Shopping) > 0 {
		return denied
	}
	switch c.Action {
	case "shopping_draft":
		if len(c.IDs) > 0 || c.Draft != nil || len(c.Shopping) < 1 || len(c.Shopping) > 10 {
			return denied
		}
		for _, v := range c.Shopping {
			if shopping.Validate(v) != nil {
				return denied
			}
		}
		return nil
	case "none", "clarify":
		if len(c.IDs) > 0 || c.Draft != nil {
			return denied
		}
		return nil
	case "complete", "confirm_multiple":
		if c.Draft != nil || len(c.IDs) == 0 || len(c.IDs) > 5 {
			return denied
		}
		if c.Action == "complete" && (len(c.IDs) != 1 || unsafeText.MatchString(text)) {
			return denied
		}
		seen := map[int]bool{}
		for _, id := range c.IDs {
			if seen[id] {
				return denied
			}
			seen[id] = true
			found := false
			for _, v := range candidates {
				if v.ID == id {
					found = true
				}
			}
			if !found {
				return denied
			}
		}
		return nil
	case "draft":
		if c.Draft == nil || len(c.IDs) > 0 {
			return denied
		}
		d := c.Draft
		if strings.TrimSpace(d.Title) == "" || len([]rune(d.Title)) > 100 || d.Points < 1 || d.Points > 10000 {
			return denied
		}
		r := d.Schedule
		if r.Hour < 0 || r.Hour > 23 || r.Minute < 0 || r.Minute > 59 {
			return denied
		}
		switch r.Mode {
		case "once", "daily":
		case "weekdays":
			if len(r.Days) == 0 || len(r.Days) > 7 {
				return denied
			}
			for _, day := range r.Days {
				switch day {
				case "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday":
				default:
					return denied
				}
			}
		case "interval", "after":
			if r.Every < 1 || r.Every > 365 {
				return denied
			}
		default:
			return denied
		}
		return nil
	default:
		return denied
	}
}
func Decode(raw, text string, candidates []Candidate) (Command, error) {
	var c Command
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		return c, errors.New("Некорректный ответ помощника")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Command{}, errors.New("Некорректный ответ помощника")
	}
	return c, c.Validate(text, candidates)
}
