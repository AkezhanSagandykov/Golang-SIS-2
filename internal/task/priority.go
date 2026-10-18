package task

import (
	"fmt"
	"strings"
)

type Priority int

const (
	PriorityLow Priority = iota + 1
	PriorityMedium
	PriorityHigh
)

func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	default:
		return fmt.Sprintf("Priority(%d)", int(p))
	}
}

func (p Priority) IsValid() bool {
	return p >= PriorityLow && p <= PriorityHigh
}

func ParsePriority(str string) (Priority, error) {
	switch strings.ToLower(strings.TrimSpace(str)) {
	case "low":
		return PriorityLow, nil
	case "medium":
		return PriorityMedium, nil
	case "high":
		return PriorityHigh, nil
	default:
		return 0, &ValidationError{Field: "priority", Message: fmt.Sprintf("unknown value %q", str)}
	}
}
