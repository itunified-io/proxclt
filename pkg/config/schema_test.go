package config

import (
	"strings"
	"testing"
)

func TestGenerateSchema(t *testing.T) {
	s, err := GenerateSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) < 100 {
		t.Errorf("schema too short: %d bytes", len(s))
	}
	if !strings.Contains(s, "Env") {
		t.Errorf("schema missing Env reference")
	}
}
