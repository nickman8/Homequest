package agent

import (
	"time"

	"donetick.com/core/internal/homequest/family"
	"donetick.com/core/internal/homequest/task"
)

// ReplyTarget is a server-captured reference, never inferred from message text.
// Resource snapshots deliberately exclude private gift reservations.
type ReplyTarget struct {
	Kind     string `json:"kind"`
	ID       int    `json:"id"`
	Version  int    `json:"version,omitempty"`
	Snapshot string `json:"snapshot,omitempty"`
}

func (s *Service) CaptureReply(uid int64, circle int, kind string, id int, now time.Time) (ReplyTarget, error) {
	out := ReplyTarget{Kind: kind, ID: id}
	if id <= 0 {
		return out, ErrChanged
	}
	if _, e := (&family.Service{DB: s.DB}).Authorize(uid, circle); e != nil {
		return out, e
	}
	if kind == "task" {
		var i task.Instance
		if e := s.DB.First(&i, "id=? AND circle_id=? AND closed=? AND pending=?", id, circle, false, false).Error; e != nil {
			return out, ErrChanged
		}
		out.Version = i.Version
		return out, nil
	}
	rows, e := s.resources(uid, circle, kind, now)
	if e != nil {
		return out, e
	}
	for _, r := range rows {
		if r.ID == id {
			out.Snapshot = r.Snapshot
			return out, nil
		}
	}
	return out, ErrChanged
}

func (s *Service) BindReply(c *Context, target ReplyTarget, now time.Time) error {
	fresh, e := s.CaptureReply(c.Viewer, c.CircleID, target.Kind, target.ID, now)
	if e != nil {
		return e
	}
	if fresh != target {
		return ErrChanged
	}
	c.ReplyTarget = &target
	if target.Kind == "task" {
		c.ReplyTask = target.ID
		return nil
	}
	rows, e := s.resources(c.Viewer, c.CircleID, target.Kind, now)
	if e != nil {
		return e
	}
	for _, r := range rows {
		if r.ID == target.ID {
			c.ReplyResource = &r
			return nil
		}
	}
	return ErrChanged
}

func (target ReplyTarget) checkChange(v Change) error {
	if target.Kind == "" {
		return nil
	}
	cap, ok := capability(v.Operation)
	if !ok || cap.Target != target.Kind || v.ID != target.ID {
		return ErrChanged
	}
	if target.Kind == "task" {
		if v.Version != target.Version {
			return ErrChanged
		}
	} else if v.Snapshot != target.Snapshot {
		return ErrChanged
	}
	return nil
}

// CheckReplyChanges is also enforced by Prepare/Apply; an LLM tool gate alone
// cannot protect against a stale job or an alternate planner adapter.
func CheckReplyChanges(c Context, changes []Change) error {
	for _, v := range changes {
		if c.ReplyTarget != nil {
			if e := c.ReplyTarget.checkChange(v); e != nil {
				return e
			}
		} else if c.ReplyTask != 0 {
			cap, ok := capability(v.Operation)
			if !ok || cap.Target != "task" || v.ID != c.ReplyTask {
				return ErrChanged
			}
		}
	}
	return nil
}
