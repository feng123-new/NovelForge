package chapterversion

import (
	"errors"
	"fmt"
)

const (
	CodeNotFound              = "CHAPTER_VERSION_NOT_FOUND"
	CodeConflict              = "CHAPTER_VERSION_CONFLICT"
	CodeImmutable             = "CHAPTER_VERSION_IMMUTABLE"
	CodeRejected              = "CHAPTER_VERSION_REJECTED"
	CodeActiveFinalConflict   = "ACTIVE_FINAL_CONFLICT"
	CodeFinalizeNotAllowed    = "FINALIZE_NOT_ALLOWED"
	CodeContinuityBlocked     = "CONTINUITY_BLOCKED"
	CodeTruthConflict         = "UNRESOLVED_TRUTH_CONFLICT"
	CodeSHAMismatch           = "CONTENT_SHA_MISMATCH"
	CodeExternalChange        = "EXTERNAL_CHANGE_DETECTED"
	CodeSyncRequired          = "SYNC_REQUIRED"
	CodeSyncContentChanged    = "SYNC_CONTENT_CHANGED"
	CodeRebuildInProgress     = "REBUILD_IN_PROGRESS"
	CodeRebuildFailed         = "REBUILD_FAILED"
	CodeStaleDerivedState     = "STALE_DERIVED_STATE"
	CodeDiffTooLarge          = "DIFF_TOO_LARGE"
	CodeDiffCursorInvalid     = "DIFF_CURSOR_INVALID"
	CodeIdempotencyConflict   = "IDEMPOTENCY_CONFLICT"
	CodeValidation            = "CHAPTER_VERSION_VALIDATION_FAILED"
	CodeStorage               = "CHAPTER_VERSION_STORAGE_FAILED"
	CodeUnsafePath            = "CHAPTER_PATH_UNSAFE"
	CodeExternalTooLarge      = "CHAPTER_EXTERNAL_FILE_TOO_LARGE"
	CodeExternalEncoding      = "CHAPTER_EXTERNAL_ENCODING_INVALID"
)

var ErrNotFound = errors.New("chapter version not found")

type Error struct {
	Code      string
	Message   string
	Retryable bool
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return "chapter version error"
	}
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func newError(code, message string, retryable bool, err error) error {
	return &Error{Code: code, Message: message, Retryable: retryable, Err: err}
}

func IsCode(err error, code string) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
