package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
)

//go:embed openapi.json
var openAPISpec []byte

func init() {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, openAPISpec, "", "  "); err == nil {
		openAPISpec = append(formatted.Bytes(), '\n')
	}
}
