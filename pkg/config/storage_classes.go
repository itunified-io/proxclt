package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// StorageClasses is the shared storage-class catalogue (kind: StorageClasses).
type StorageClasses struct {
	Kind    string                  `yaml:"kind"    json:"kind"    validate:"required,eq=StorageClasses"`
	Classes map[string]StorageClass `yaml:"-"       json:"classes" validate:"required,min=1,dive"`
}

// StorageClass is a named Proxmox storage backend.
type StorageClass struct {
	Backend string `yaml:"backend"          json:"backend"          validate:"required"`
	Shared  bool   `yaml:"shared,omitempty" json:"shared,omitempty"`
}

// UnmarshalYAML splits "kind" from inline class map entries.
func (s *StorageClasses) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("storage_classes: expected mapping, got %v", node.Kind)
	}
	s.Classes = make(map[string]StorageClass)
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if k.Value == "kind" {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("storage_classes.kind: expected scalar")
			}
			s.Kind = v.Value
			continue
		}
		var sc StorageClass
		if err := v.Decode(&sc); err != nil {
			return fmt.Errorf("storage_classes.%s: %w", k.Value, err)
		}
		s.Classes[k.Value] = sc
	}
	return nil
}

// MarshalYAML flattens StorageClasses back to an inline-map layout.
func (s StorageClasses) MarshalYAML() (any, error) {
	m := map[string]any{"kind": s.Kind}
	for k, v := range s.Classes {
		m[k] = v
	}
	return m, nil
}
