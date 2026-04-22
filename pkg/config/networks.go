package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Networks is the shared network-zone catalogue (kind: Networks).
// All keys other than "kind" are zone names.
type Networks struct {
	Kind  string                 `yaml:"kind"  json:"kind"  validate:"required,eq=Networks"`
	Zones map[string]NetworkZone `yaml:"-"     json:"zones" validate:"required,min=1,dive"`
}

// NetworkZone describes one named L3 zone (public, vip, private, …).
type NetworkZone struct {
	CIDR    string   `yaml:"cidr"              json:"cidr"              validate:"required,cidr"`
	Gateway string   `yaml:"gateway,omitempty" json:"gateway,omitempty" validate:"omitempty,ip"`
	DNS     []string `yaml:"dns,omitempty"     json:"dns,omitempty"     validate:"omitempty,dive,ip"`
}

// UnmarshalYAML implements custom unmarshalling to split the "kind" scalar
// from the remaining map entries which become Zones.
func (n *Networks) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("networks: expected mapping, got %v", node.Kind)
	}
	n.Zones = make(map[string]NetworkZone)
	for i := 0; i < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if k.Value == "kind" {
			if v.Kind != yaml.ScalarNode {
				return fmt.Errorf("networks.kind: expected scalar")
			}
			n.Kind = v.Value
			continue
		}
		var zone NetworkZone
		if err := v.Decode(&zone); err != nil {
			return fmt.Errorf("networks.%s: %w", k.Value, err)
		}
		n.Zones[k.Value] = zone
	}
	return nil
}

// MarshalYAML flattens Networks back to the inline-map layout.
func (n Networks) MarshalYAML() (any, error) {
	m := map[string]any{"kind": n.Kind}
	for k, v := range n.Zones {
		m[k] = v
	}
	return m, nil
}
