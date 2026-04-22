// Package config defines the proxclt env manifest model, YAML loader,
// $ref resolution, profile inheritance, secret placeholder resolution,
// cross-field validation, and JSON Schema export.
package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Env is the top-level master manifest (kind: Env).
type Env struct {
	Version  string      `yaml:"version"  json:"version"  validate:"required,eq=1"`
	Kind     string      `yaml:"kind"     json:"kind"     validate:"required,eq=Env"`
	Metadata EnvMetadata `yaml:"metadata" json:"metadata" validate:"required"`
	Extends  string      `yaml:"extends,omitempty" json:"extends,omitempty"`
	Spec     EnvSpec     `yaml:"spec"     json:"spec"     validate:"required"`
	Hooks    *Hooks      `yaml:"hooks,omitempty" json:"hooks,omitempty"`
}

// EnvMetadata holds descriptive metadata for an env.
type EnvMetadata struct {
	Name           string            `yaml:"name"                      json:"name"                      validate:"required,hostname_rfc1123"`
	Domain         string            `yaml:"domain,omitempty"          json:"domain,omitempty"`
	ProxmoxContext string            `yaml:"proxmox_context,omitempty" json:"proxmox_context,omitempty"`
	DbxContext     string            `yaml:"dbx_context,omitempty"     json:"dbx_context,omitempty"`
	Tags           map[string]string `yaml:"tags,omitempty"            json:"tags,omitempty"`
	Description    string            `yaml:"description,omitempty"     json:"description,omitempty"`
}

// EnvSpec is the spec block of the master manifest.
type EnvSpec struct {
	Hypervisor     Ref[Hypervisor]     `yaml:"hypervisor"        json:"hypervisor"        validate:"required"`
	Linux          *Ref[Linux]         `yaml:"linux,omitempty"   json:"linux,omitempty"`
	Networks       Ref[Networks]       `yaml:"networks"          json:"networks"          validate:"required"`
	StorageClasses Ref[StorageClasses] `yaml:"storage_classes"   json:"storage_classes"   validate:"required"`
	Cluster        *Ref[Cluster]       `yaml:"cluster,omitempty" json:"cluster,omitempty"`
	Databases      []Ref[Database]     `yaml:"databases,omitempty" json:"databases,omitempty"`
}

// Ref[T] represents a value that is either inlined or referenced via $ref
// to another YAML file.
type Ref[T any] struct {
	// Ref is the relative path to the referenced file (if any).
	Ref string `yaml:"-" json:"$ref,omitempty"`
	// Value is the resolved value. Populated after loading.
	Value *T `yaml:"-" json:"-"`
	// Inline holds the value if it was embedded directly in YAML.
	Inline *T `yaml:"-" json:"-"`
}

// UnmarshalYAML implements custom unmarshalling for Ref[T]:
// if the node contains only a "$ref" key, it is stored as a file reference;
// otherwise the node is unmarshalled as an inline T.
func (r *Ref[T]) UnmarshalYAML(node *yaml.Node) error {
	// Look for a $ref key when it's a mapping.
	if node.Kind == yaml.MappingNode {
		// Detect a single-key $ref map.
		if len(node.Content) == 2 {
			key := node.Content[0]
			val := node.Content[1]
			if key.Kind == yaml.ScalarNode && key.Value == "$ref" && val.Kind == yaml.ScalarNode {
				r.Ref = val.Value
				return nil
			}
		}
		// Otherwise, also check if $ref is one of multiple keys — treat as reference.
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == "$ref" {
				val := node.Content[i+1]
				if val.Kind == yaml.ScalarNode {
					r.Ref = val.Value
					return nil
				}
			}
		}
	}
	// Fall back to inline decoding.
	var t T
	if err := node.Decode(&t); err != nil {
		return fmt.Errorf("ref inline decode: %w", err)
	}
	r.Inline = &t
	return nil
}

// MarshalYAML implements custom marshalling for Ref[T] so that the rendered
// output always shows the resolved Value (or inline) — $refs are flattened.
func (r Ref[T]) MarshalYAML() (any, error) {
	if r.Value != nil {
		return r.Value, nil
	}
	if r.Inline != nil {
		return r.Inline, nil
	}
	if r.Ref != "" {
		return map[string]string{"$ref": r.Ref}, nil
	}
	return nil, nil
}

// Resolved returns the effective T, preferring the loaded Value, falling back to Inline.
func (r *Ref[T]) Resolved() *T {
	if r == nil {
		return nil
	}
	if r.Value != nil {
		return r.Value
	}
	return r.Inline
}
