package task

import (
	"fmt"
	"slices"
	"strings"
)

type Status int

const (
	StatusTodo Status = iota + 1
	StatusQueued
	StatusInProgress
	StatusDone
	StatusFailed
	StatusCancelled
)

var transitions = map[Status][]Status{
	StatusTodo:       {StatusQueued, StatusCancelled},
	StatusQueued:     {StatusInProgress, StatusTodo},
	StatusInProgress: {StatusDone, StatusFailed},
	StatusDone:       {},
	StatusFailed:     {StatusTodo},
	StatusCancelled:  {StatusTodo},
}

func AllStatuses() []Status {
	return []Status{StatusTodo, StatusQueued, StatusInProgress, StatusDone, StatusFailed, StatusCancelled}
}

func (s Status) String() string {
	switch s {
	case StatusTodo:
		return "todo"
	case StatusQueued:
		return "queued"
	case StatusInProgress:
		return "in_progress"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("Status(%d)", int(s))
	}
}

func (s Status) IsValid() bool {
	_, ok := transitions[s]
	return ok
}

func (s Status) IsClosed() bool {
	return s == StatusDone || s == StatusCancelled
}

func (s Status) IsBusy() bool {
	return s == StatusQueued || s == StatusInProgress
}

func (s Status) CanTransitionTo(to Status) bool {
	return slices.Contains(transitions[s], to)
}

func ParseStatus(str string) (Status, error) {
	str = strings.ToLower(strings.TrimSpace(str))
	for _, s := range AllStatuses() {
		if s.String() == str {
			return s, nil
		}
	}
	return 0, &ValidationError{Field: "status", Message: fmt.Sprintf("unknown value %q", str)}
}
