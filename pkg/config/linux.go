package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Linux is the linux.yaml passthrough manifest. proxclt reads this for
// cross-file validation (disk tag refs) but otherwise treats it as opaque.
type Linux struct {
	Kind string         `yaml:"kind" json:"kind" validate:"required,eq=Linux"`
	Raw  map[string]any `yaml:"-"    json:"raw,omitempty"`
}

// UnmarshalYAML separates "kind" from everything else (stored in Raw).
func (l *Linux) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("linux: expected mapping, got %v", node.Kind)
	}
	l.Raw = make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if k.Value == "kind" {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("linux.kind: expected scalar")
			}
			l.Kind = v.Value
			continue
		}
		var any any
		if err := v.Decode(&any); err != nil {
			return fmt.Errorf("linux.%s: %w", k.Value, err)
		}
		l.Raw[k.Value] = any
	}
	return nil
}

// MarshalYAML flattens Linux back to the inline-map layout.
func (l Linux) MarshalYAML() (any, error) {
	m := map[string]any{"kind": l.Kind}
	for k, v := range l.Raw {
		m[k] = v
	}
	return m, nil
}
