package narrativeledger

import (
	"errors"
	"fmt"
)

var (
	ErrValidation = errors.New("narrative ledger validation failed")
	ErrNotFound   = errors.New("narrative ledger item not found")
	ErrConflict   = errors.New("narrative ledger conflict")
	ErrAuthority  = errors.New("narrative ledger authority rejected")
)

// Error keeps transport-safe codes separate from internal causes.
type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func newError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func errorCode(err error) string {
	var domain *Error
	if errors.As(err, &domain) {
		return domain.Code
	}
	return "LEDGER_INTERNAL_ERROR"
}
