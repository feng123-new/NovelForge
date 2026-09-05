package qualityruntime

import (
	"context"
	"errors"

	"github.com/voocel/ainovel-cli/internal/project"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
)

// QualityRuntime resolves project paths only inside Repository. Secrets never
// travel through Foundation requests, browser settings or model payloads.
type QualityRuntime struct {
	repository *project.Repository
	explicit   string
	fallback   *QualityModel
}

func New(workspace, explicit string) (*QualityRuntime, error) {
	repository, err := project.NewRepository(workspace)
	if err != nil {
		return nil, err
	}
	fallback, err := LoadQualityModel("", repository.Workspace(), explicit)
	if err != nil {
		return nil, err
	}
	return &QualityRuntime{repository: repository, explicit: explicit, fallback: fallback}, nil
}

// Configured reports workspace-default availability, not provider health.
func (r *QualityRuntime) Configured() bool { return r != nil && r.fallback != nil }

func (r *QualityRuntime) ForProject(ctx context.Context, id string) (qualitygate.ModelInvoker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := r.repository.ConfigurationRoot(ctx, id)
	if err != nil {
		return nil, err
	}
	model, err := LoadQualityModel("", root, r.explicit)
	if err != nil {
		return nil, err
	}
	if model != nil {
		return model, nil
	}
	if r.fallback != nil {
		return r.fallback, nil
	}
	return nil, errors.New("quality model is not configured for this project")
}

func (r *QualityRuntime) Invoke(ctx context.Context, operation string, payload []byte) ([]byte, qualitygate.ModelUsage, error) {
	return r.fallback.Invoke(ctx, operation, payload)
}
