package qualitygate

import (
	"context"
	"encoding/json"
)

// WriterContextCompiler is a read-only project context boundary. Its output is
// included before computing the idempotent model request hash.
type WriterContextCompiler interface {
	CompileWriterContext(context.Context, WriterRequest, int) (json.RawMessage, error)
}
