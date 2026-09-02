package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed openapi.json
var baseOpenAPISpec []byte

//go:embed openapi_phase5.json
var phase5OpenAPI []byte

var openAPISpec []byte

func init() {
	openAPISpec = mergeOpenAPIDocuments(baseOpenAPISpec, phase5OpenAPI)
}

func mergeOpenAPIDocuments(baseBytes, extensionBytes []byte) []byte {
	var base map[string]any
	var extension map[string]any
	if err := json.Unmarshal(baseBytes, &base); err != nil {
		panic(fmt.Sprintf("invalid embedded base OpenAPI: %v", err))
	}
	if err := json.Unmarshal(extensionBytes, &extension); err != nil {
		panic(fmt.Sprintf("invalid embedded OpenAPI extension: %v", err))
	}
	mergeObjectSection(base, extension, "paths")
	baseComponents, _ := base["components"].(map[string]any)
	if baseComponents == nil {
		baseComponents = map[string]any{}
		base["components"] = baseComponents
	}
	extComponents, _ := extension["components"].(map[string]any)
	if extComponents != nil {
		for _, section := range []string{"schemas", "parameters", "responses"} {
			mergeObjectSection(baseComponents, extComponents, section)
		}
	}
	formatted, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("format merged OpenAPI: %v", err))
	}
	return append(formatted, '\n')
}

func mergeObjectSection(target, extension map[string]any, key string) {
	extensionSection, _ := extension[key].(map[string]any)
	if len(extensionSection) == 0 {
		return
	}
	targetSection, _ := target[key].(map[string]any)
	if targetSection == nil {
		targetSection = map[string]any{}
		target[key] = targetSection
	}
	for name, value := range extensionSection {
		if _, exists := targetSection[name]; exists {
			panic(fmt.Sprintf("duplicate OpenAPI %s entry %q", key, name))
		}
		targetSection[name] = value
	}
}
