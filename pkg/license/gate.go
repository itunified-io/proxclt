// Package license wraps dbx/pkg/core/license for proxctl's per-tool gate.
//
// Phase 1 (scaffold): thin stub with ToolTier constants + ToolCatalog map.
// Phase 2+ will wire Check() through to github.com/itunified-io/dbx/pkg/core/license.
package license

// ToolTier classifies each proxctl leaf command against a commercial tier.
type ToolTier int

const (
	// TierCommunity is always allowed — free / AGPL tools.
	TierCommunity ToolTier = iota
	// TierBusiness requires a Business license (e.g. workflow --parallel, profile CRUD).
	TierBusiness
	// TierEnterprise requires an Enterprise license (e.g. audit export, airgap, rbac).
	TierEnterprise
)

// String returns the tier name.
func (t ToolTier) String() string {
	switch t {
	case TierCommunity:
		return "community"
	case TierBusiness:
		return "business"
	case TierEnterprise:
		return "enterprise"
	default:
		return "unknown"
	}
}

// ToolCatalog maps each proxctl leaf command to its required tier.
// Authoritative source for the license gate — see docs/licensing.md.
var ToolCatalog = map[string]ToolTier{
	// config group
	"config.validate":        TierCommunity,
	"config.render":          TierCommunity,
	"config.use-context":     TierCommunity,
	"config.current-context": TierCommunity,
	"config.get-contexts":    TierCommunity,

	// env group
	"env.new":     TierCommunity,
	"env.list":    TierCommunity,
	"env.use":     TierCommunity,
	"env.current": TierCommunity,
	"env.add":     TierCommunity,
	"env.remove":  TierCommunity,
	"env.show":    TierCommunity,

	// vm group
	"vm.create": TierCommunity,
	"vm.start":  TierCommunity,
	"vm.stop":   TierCommunity,
	"vm.reboot": TierCommunity,
	"vm.delete": TierCommunity,
	"vm.list":   TierCommunity,
	"vm.status": TierCommunity,

	// snapshot group
	"snapshot.create":  TierCommunity,
	"snapshot.restore": TierCommunity,
	"snapshot.list":    TierCommunity,
	"snapshot.delete":  TierCommunity,

	// kickstart group
	"kickstart.generate":  TierCommunity,
	"kickstart.build-iso": TierCommunity,
	"kickstart.upload":    TierCommunity,
	"kickstart.distros":   TierCommunity,

	// boot group
	"boot.configure-first-boot": TierCommunity,
	"boot.eject-iso":            TierCommunity,

	// workflow group (base commands are Community; --parallel>1 gates to Business at runtime)
	"workflow.plan":   TierCommunity,
	"workflow.up":     TierCommunity,
	"workflow.down":   TierCommunity,
	"workflow.status": TierCommunity,
	"workflow.verify": TierCommunity,

	// license group
	"license.status":     TierCommunity,
	"license.activate":   TierCommunity,
	"license.show":       TierCommunity,
	"license.seats-used": TierCommunity,
}

// Check is the entry point for every subcommand handler.
// Phase 1: stub that always allows Community and returns an "unlicensed" error
// for Business/Enterprise. Phase 2 will call dbx/pkg/core/license.
func Check(tool string) error {
	tier, ok := ToolCatalog[tool]
	if !ok {
		// Unknown tools are treated as Community to avoid false-negatives
		// during early development. Phase 2 will make this stricter.
		return nil
	}
	if tier == TierCommunity {
		return nil
	}
	// Placeholder: real license loading lands in Phase 2.
	return nil
}
