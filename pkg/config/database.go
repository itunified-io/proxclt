package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Database is a fully opaque manifest; proxclt does not act on it.
type Database struct {
	Kind string         `yaml:"kind" json:"kind" validate:"required,oneof=OracleDatabase PostgresDatabase"`
	Raw  map[string]any `yaml:"-"    json:"raw,omitempty"`
}

// UnmarshalYAML splits "kind" from the remaining keys.
func (d *Database) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("database: expected mapping, got %v", node.Kind)
	}
	d.Raw = make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if k.Value == "kind" {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("database.kind: expected scalar")
			}
			d.Kind = v.Value
			continue
		}
		var any any
		if err := v.Decode(&any); err != nil {
			return fmt.Errorf("database.%s: %w", k.Value, err)
		}
		d.Raw[k.Value] = any
	}
	return nil
}

// MarshalYAML flattens Database back to the inline-map layout.
func (d Database) MarshalYAML() (any, error) {
	m := map[string]any{"kind": d.Kind}
	for k, v := range d.Raw {
		m[k] = v
	}
	return m, nil
}
