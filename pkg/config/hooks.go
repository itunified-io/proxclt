package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Hooks is the set of lifecycle hooks that can fire during proxclt operations.
type Hooks struct {
	OnValidateSuccess []Hook `yaml:"on_validate_success,omitempty" json:"on_validate_success,omitempty" validate:"dive"`
	OnApplyStart      []Hook `yaml:"on_apply_start,omitempty"      json:"on_apply_start,omitempty"      validate:"dive"`
	OnApplySuccess    []Hook `yaml:"on_apply_success,omitempty"    json:"on_apply_success,omitempty"    validate:"dive"`
	OnApplyFailure    []Hook `yaml:"on_apply_failure,omitempty"    json:"on_apply_failure,omitempty"    validate:"dive"`
	OnVMReady         []Hook `yaml:"on_vm_ready,omitempty"         json:"on_vm_ready,omitempty"         validate:"dive"`
	OnTeardownSuccess []Hook `yaml:"on_teardown_success,omitempty" json:"on_teardown_success,omitempty" validate:"dive"`
}

// Hook is a single hook invocation.
type Hook struct {
	Type   string         `yaml:"type"  json:"type"  validate:"required,oneof=slack exec webhook audit log"`
	Params map[string]any `yaml:"-"     json:"params,omitempty"`
}

// UnmarshalYAML splits "type" from the remaining params.
func (h *Hook) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("hook: expected mapping, got %v", node.Kind)
	}
	h.Params = make(map[string]any)
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if k.Value == "type" {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("hook.type: expected scalar")
			}
			h.Type = v.Value
			continue
		}
		var any any
		if err := v.Decode(&any); err != nil {
			return fmt.Errorf("hook.%s: %w", k.Value, err)
		}
		h.Params[k.Value] = any
	}
	return nil
}

// MarshalYAML flattens Hook back to the inline-map layout.
func (h Hook) MarshalYAML() (any, error) {
	m := map[string]any{"type": h.Type}
	for k, v := range h.Params {
		m[k] = v
	}
	return m, nil
}
