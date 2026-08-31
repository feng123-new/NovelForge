package engineadapter

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/voocel/ainovel-cli/assets"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/host"
)

// Mode identifies a deterministic entry into the existing ainovel engine.
type Mode string

const (
	ModeStart  Mode = "start"
	ModeResume Mode = "resume"
)

// OpenRequest contains already-validated engine startup data. Web/API code must
// not duplicate host, route, model, retry or checkpoint behavior.
type OpenRequest struct {
	Mode   Mode
	Config bootstrap.Config
	Bundle assets.Bundle
	Prompt string
}

// Session exposes the existing engine's event and stream contracts.
type Session interface {
	Events() <-chan host.Event
	Stream() <-chan string
	Done() <-chan struct{}
	Close()
}

// EngineService is the Web/job boundary for the current engine.
type EngineService interface {
	Open(ctx context.Context, request OpenRequest) (Session, error)
}

// LegacyAdapter delegates to the mature host engine retained from ainovel-cli.
type LegacyAdapter struct{}

func (LegacyAdapter) Open(ctx context.Context, request OpenRequest) (Session, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	engine, err := host.New(request.Config, request.Bundle)
	if err != nil {
		return nil, err
	}
	session := &legacySession{engine: engine}

	switch request.Mode {
	case ModeStart:
		prompt := strings.TrimSpace(request.Prompt)
		if prompt == "" {
			session.Close()
			return nil, errors.New("prepared prompt is required")
		}
		if err := engine.PrepareUserRules(prompt); err != nil {
			session.Close()
			return nil, err
		}
		if err := engine.StartPrepared(prompt); err != nil {
			session.Close()
			return nil, err
		}
	case ModeResume:
		label, err := engine.Resume()
		if err != nil {
			session.Close()
			return nil, err
		}
		if strings.TrimSpace(label) == "" {
			session.Close()
			return nil, errors.New("no resumable engine session")
		}
	default:
		session.Close()
		return nil, errors.New("unsupported engine mode")
	}

	go func() {
		select {
		case <-ctx.Done():
			session.Close()
		case <-engine.Done():
		}
	}()
	return session, nil
}

type legacySession struct {
	engine *host.Host
	once   sync.Once
}

func (s *legacySession) Events() <-chan host.Event { return s.engine.Events() }
func (s *legacySession) Stream() <-chan string     { return s.engine.Stream() }
func (s *legacySession) Done() <-chan struct{}     { return s.engine.Done() }

func (s *legacySession) Close() {
	s.once.Do(func() {
		s.engine.Close()
	})
}
