package qualitygate

import (
 "context"
 "encoding/json"
)

// EditorialContextProvider supplies selected skills and advisory rule findings
// before the model-call hash is computed. It has no chapter commit authority.
type EditorialContextProvider interface {
 CompileEditorialContext(context.Context,EditorRequest)(json.RawMessage,[]string,error)
}
