package task

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type ID int

const MaxTitleLen = 200

type Task struct {
	ID          ID
	Title       string
	Description string
	Priority    Priority
	Tags        []string
	Deadline    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Result     Result
	FailReason string
	Worker     int

	status Status
}

type Params struct {
	Title       string
	Description string
	Priority    Priority
	Deadline    time.Time
	Tags        []string
}

func New(p Params, now time.Time) (Task, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return Task{}, &ValidationError{Field: "title", Message: "must not be empty"}
	}
	if utf8.RuneCountInString(title) > MaxTitleLen {
		return Task{}, &ValidationError{
			Field:   "title",
			Message: fmt.Sprintf("must be at most %d characters", MaxTitleLen),
		}
	}

	priority := p.Priority
	if priority == 0 {
		priority = PriorityMedium
	}
	if !priority.IsValid() {
		return Task{}, &ValidationError{Field: "priority", Message: fmt.Sprintf("unknown value %d", int(priority))}
	}

	if !p.Deadline.IsZero() && p.Deadline.Before(now) {
		return Task{}, &ValidationError{Field: "deadline", Message: "must not be in the past"}
	}

	return Task{
		Title:       title,
		Description: strings.TrimSpace(p.Description),
		Priority:    priority,
		Tags:        normalizeTags(p.Tags),
		Deadline:    p.Deadline,
		CreatedAt:   now,
		UpdatedAt:   now,
		status:      StatusTodo,
	}, nil
}

func (t *Task) Status() Status {
	return t.status
}

func (t *Task) TransitionTo(to Status, now time.Time) error {
	if !to.IsValid() {
		return &ValidationError{Field: "status", Message: fmt.Sprintf("unknown value %d", int(to))}
	}
	if !t.status.CanTransitionTo(to) {
		return &TransitionError{ID: t.ID, From: t.status, To: to}
	}
	t.status = to
	t.UpdatedAt = now
	return nil
}

func (t *Task) Start(worker int, now time.Time) error {
	if err := t.TransitionTo(StatusInProgress, now); err != nil {
		return err
	}
	t.Worker = worker
	return nil
}

func (t *Task) Complete(r Result, now time.Time) error {
	if err := t.TransitionTo(StatusDone, now); err != nil {
		return err
	}
	t.Result = r
	t.FailReason = ""
	return nil
}

func (t *Task) Fail(reason string, now time.Time) error {
	if err := t.TransitionTo(StatusFailed, now); err != nil {
		return err
	}
	t.Result = Result{}
	t.FailReason = reason
	return nil
}

func (t *Task) Reopen(now time.Time) error {
	if t.status != StatusFailed && t.status != StatusCancelled {
		return &TransitionError{ID: t.ID, From: t.status, To: StatusTodo}
	}
	return t.TransitionTo(StatusTodo, now)
}

func (t *Task) IsOverdue(now time.Time) bool {
	return !t.Deadline.IsZero() && now.After(t.Deadline) && !t.status.IsClosed()
}

func (t *Task) HasTag(tag string) bool {
	return slices.Contains(t.Tags, normalizeTag(tag))
}

func (t *Task) Clone() Task {
	c := *t
	c.Tags = slices.Clone(t.Tags)
	c.Result.Keywords = slices.Clone(t.Result.Keywords)
	return c
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := normalizeTag(raw)
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeTag(tag string) string {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
	return strings.ToLower(strings.TrimSpace(tag))
}
