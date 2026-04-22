package config

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

// GenerateSchema produces a JSON Schema document for the Env model.
func GenerateSchema() (string, error) {
	r := &jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: false,
		DoNotReference:             false,
	}
	schema := r.Reflect(&Env{})
	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal schema: %w", err)
	}
	return string(b), nil
}
