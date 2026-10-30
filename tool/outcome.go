package tool

import "errors"

type Outcome string

const (
	OutcomeRecoverable Outcome = "recoverable"
	OutcomeDenied      Outcome = "denied"
	OutcomeUncertain   Outcome = "uncertain"
)

type OutcomeError struct {
	Outcome Outcome
	Err     error
}

func (e *OutcomeError) Error() string {
	if e.Err != nil {
		return string(e.Outcome) + ": " + e.Err.Error()
	}
	return string(e.Outcome)
}

func (e *OutcomeError) Unwrap() error {
	return e.Err
}

func NewOutcomeError(outcome Outcome, err error) error {
	return &OutcomeError{Outcome: outcome, Err: err}
}

func NewDeniedError(err error) error {
	return NewOutcomeError(OutcomeDenied, err)
}

func NewUncertainError(err error) error {
	return NewOutcomeError(OutcomeUncertain, err)
}

func OutcomeOf(err error) Outcome {
	var outcomeErr *OutcomeError
	if errors.As(err, &outcomeErr) {
		return outcomeErr.Outcome
	}
	return OutcomeRecoverable
}

func IsTerminal(err error) bool {
	outcome := OutcomeOf(err)
	return outcome == OutcomeDenied || outcome == OutcomeUncertain
}
