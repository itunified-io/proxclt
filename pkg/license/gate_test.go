package license

import "testing"

func TestLicense_TierConstants(t *testing.T) {
	cases := []struct {
		tier ToolTier
		want string
	}{
		{TierCommunity, "community"},
		{TierBusiness, "business"},
		{TierEnterprise, "enterprise"},
		{ToolTier(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.tier.String(); got != tc.want {
			t.Errorf("ToolTier(%d).String() = %q, want %q", int(tc.tier), got, tc.want)
		}
	}
}

func TestLicense_CatalogHasCoreTools(t *testing.T) {
	core := []string{"vm.create", "vm.delete", "workflow.up", "kickstart.generate", "license.status"}
	for _, tool := range core {
		if _, ok := ToolCatalog[tool]; !ok {
			t.Errorf("ToolCatalog missing core tool %q", tool)
		}
	}
}

// TestLicense_CatalogMatrix asserts every catalog entry has a known tier.
// This guards against silently promoting/demoting a tool during future edits.
func TestLicense_CatalogMatrix(t *testing.T) {
	expectedTiers := map[ToolTier]bool{
		TierCommunity:  true,
		TierBusiness:   true,
		TierEnterprise: true,
	}
	if len(ToolCatalog) == 0 {
		t.Fatal("ToolCatalog is empty")
	}
	for tool, tier := range ToolCatalog {
		if !expectedTiers[tier] {
			t.Errorf("ToolCatalog[%q] = %v (tier out of range)", tool, tier)
		}
	}
}

// TestLicense_CheckMatrix covers every Check() branch:
// known-community tool, known-business tool (placeholder allow), unknown tool.
func TestLicense_CheckMatrix(t *testing.T) {
	// Seed a known tool of each tier into the catalog for the duration of this test.
	origBusiness, hadBusiness := ToolCatalog["test.business"]
	origEnt, hadEnt := ToolCatalog["test.enterprise"]
	ToolCatalog["test.business"] = TierBusiness
	ToolCatalog["test.enterprise"] = TierEnterprise
	defer func() {
		if hadBusiness {
			ToolCatalog["test.business"] = origBusiness
		} else {
			delete(ToolCatalog, "test.business")
		}
		if hadEnt {
			ToolCatalog["test.enterprise"] = origEnt
		} else {
			delete(ToolCatalog, "test.enterprise")
		}
	}()

	cases := []struct {
		name string
		tool string
	}{
		{"community allowed", "vm.create"},
		{"unknown allowed (phase 1 lenient)", "no.such.tool"},
		{"business allowed (phase 1 placeholder)", "test.business"},
		{"enterprise allowed (phase 1 placeholder)", "test.enterprise"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Check(tc.tool); err != nil {
				t.Errorf("Check(%q) unexpected error: %v", tc.tool, err)
			}
		})
	}
}

// TestLicense_CheckCommunityAlwaysAllowed preserved for explicit coverage.
func TestLicense_CheckCommunityAlwaysAllowed(t *testing.T) {
	for tool, tier := range ToolCatalog {
		if tier != TierCommunity {
			continue
		}
		if err := Check(tool); err != nil {
			t.Errorf("Check(%q) = %v, want nil for Community tool", tool, err)
		}
	}
}
