package task

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("task not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation failed")
	ErrTaskBusy          = errors.New("task is queued or being processed")
	ErrNothingToAnalyze  = errors.New("description has no words to analyze")
)

type TransitionError struct {
	ID   ID
	From Status
	To   Status
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("task %d: %s -> %s: %v", e.ID, e.From, e.To, ErrInvalidTransition)
}

func (e *TransitionError) Unwrap() error {
	return ErrInvalidTransition
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%v: %s %s", ErrValidation, e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}
