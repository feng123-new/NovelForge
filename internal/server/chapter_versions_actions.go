package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
)

func (s *Server) handleChapterVersionCheck(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.check", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, _ []byte) (int, any, error) {
		evaluation, err := coordinator.Check(r.Context(), chapter, key, versionID)
		return http.StatusOK, chapterVersionCheckView{Evaluation: evaluation}, err
	})
}

func (s *Server) handleChapterVersionRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()

	s.executeIdempotent(w, r, "chapter.version.restore", projectID, func(body []byte) (int, any, *apiFailure) {
		var input struct{}
		if bodyFailure := decodeJSONBody(body, &input, true); bodyFailure != nil {
			return bodyFailure.Status, nil, bodyFailure
		}
		service := chapterversion.Service{Store: store}
		version, err := service.Restore(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID)
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusCreated, chapterversion.RestoreResult{Version: version}, nil
	})
}

func (s *Server) handleChapterVersionAccept(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.accept", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, body []byte) (int, any, error) {
		var input struct {
			Reason string `json:"reason,omitempty"`
		}
		if failure := decodeJSONBody(body, &input, true); failure != nil {
			return failure.Status, nil, apiFailureError(*failure)
		}
		version, err := coordinator.Accept(r.Context(), chapter, key, versionID, strings.TrimSpace(input.Reason))
		return http.StatusOK, chapterversion.AcceptResult{Version: version}, err
	})
}

func (s *Server) handleChapterVersionReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	store, cleanup, failure := s.chapterVersionStore(r, projectID)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	defer cleanup()

	s.executeIdempotent(w, r, "chapter.version.reject", projectID, func(body []byte) (int, any, *apiFailure) {
		var input struct {
			Reason string `json:"reason"`
		}
		if bodyFailure := decodeJSONBody(body, &input, false); bodyFailure != nil {
			return bodyFailure.Status, nil, bodyFailure
		}
		service := chapterversion.Service{Store: store}
		version, err := service.Reject(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID, input.Reason)
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusOK, chapterversion.AcceptResult{Version: version}, nil
	})
}

func (s *Server) handleChapterVersionFinalize(w http.ResponseWriter, r *http.Request) {
	s.handleChapterCoordinatorAction(w, r, "chapter.version.finalize", func(coordinator *chapterversion.Coordinator, chapter int, key, versionID string, body []byte) (int, any, error) {
		var input struct{}
		if failure := decodeJSONBody(body, &input, true); failure != nil {
			return failure.Status, nil, apiFailureError(*failure)
		}
		result, err := coordinator.Finalize(r.Context(), chapter, key, versionID)
		return http.StatusOK, result, err
	})
}

func (s *Server) handleChapterSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	s.executeIdempotent(w, r, "chapter.version.external_sync", projectID, func(body []byte) (int, any, *apiFailure) {
		var input struct {
			ObservedSHA string `json:"observed_sha,omitempty"`
		}
		if bodyFailure := decodeJSONBody(body, &input, true); bodyFailure != nil {
			return bodyFailure.Status, nil, bodyFailure
		}
		coordinator, cleanup, failure := s.chapterVersionCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		result, err := coordinator.SyncExternal(r.Context(), chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), strings.TrimSpace(input.ObservedSHA))
		if err != nil {
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return http.StatusOK, result, nil
	})
}

type chapterCoordinatorAction func(*chapterversion.Coordinator, int, string, string, []byte) (int, any, error)

func (s *Server) handleChapterCoordinatorAction(w http.ResponseWriter, r *http.Request, operation string, action chapterCoordinatorAction) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r, http.MethodPost)
		return
	}
	projectID, chapter, failure := s.chapterVersionIdentity(r)
	if failure != nil {
		writeFailure(w, r, *failure)
		return
	}
	versionID := strings.TrimSpace(r.PathValue("version"))
	if versionID == "" {
		writeFailure(w, r, apiFailure{
			Status:  http.StatusBadRequest,
			Code:    "CHAPTER_VERSION_VALIDATION_FAILED",
			Message: "version is required",
		})
		return
	}

	s.executeIdempotent(w, r, operation, projectID, func(body []byte) (int, any, *apiFailure) {
		coordinator, cleanup, failure := s.chapterVersionCoordinator(r, projectID)
		if failure != nil {
			return failure.Status, nil, failure
		}
		defer cleanup()
		status, result, err := action(coordinator, chapter, strings.TrimSpace(r.Header.Get("Idempotency-Key")), versionID, body)
		if err != nil {
			var wrapped *apiFailureAsError
			if errors.As(err, &wrapped) {
				return wrapped.Failure.Status, nil, &wrapped.Failure
			}
			failure := chapterVersionFailure(err)
			return failure.Status, nil, failure
		}
		return status, result, nil
	})
}
