package server

import (
	"errors"
	"net/http"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
)

func chapterVersionFailure(err error) *apiFailure {
	var versionErr *chapterversion.Error
	if errors.As(err, &versionErr) {
		status := http.StatusInternalServerError
		switch versionErr.Code {
		case chapterversion.CodeValidation, chapterversion.CodeDiffCursorInvalid:
			status = http.StatusBadRequest
		case chapterversion.CodeNotFound:
			status = http.StatusNotFound
		case chapterversion.CodeDiffTooLarge, chapterversion.CodeExternalTooLarge:
			status = http.StatusRequestEntityTooLarge
		case chapterversion.CodeUnsafePath, chapterversion.CodeExternalEncoding:
			status = http.StatusUnprocessableEntity
		case chapterversion.CodeConflict,
			chapterversion.CodeImmutable,
			chapterversion.CodeRejected,
			chapterversion.CodeActiveFinalConflict,
			chapterversion.CodeFinalizeNotAllowed,
			chapterversion.CodeContinuityBlocked,
			chapterversion.CodeTruthConflict,
			chapterversion.CodeSHAMismatch,
			chapterversion.CodeExternalChange,
			chapterversion.CodeSyncRequired,
			chapterversion.CodeSyncContentChanged,
			chapterversion.CodeRebuildInProgress,
			chapterversion.CodeRebuildFailed,
			chapterversion.CodeStaleDerivedState,
			chapterversion.CodeIdempotencyConflict:
			status = http.StatusConflict
		}
		return &apiFailure{
			Status:    status,
			Code:      versionErr.Code,
			Message:   versionErr.Message,
			Retryable: versionErr.Retryable,
		}
	}
	return ptrFailure(internalFailure())
}

type apiFailureAsError struct {
	Failure apiFailure
}

func (e *apiFailureAsError) Error() string {
	return e.Failure.Message
}

func apiFailureError(failure apiFailure) error {
	return &apiFailureAsError{Failure: failure}
}
