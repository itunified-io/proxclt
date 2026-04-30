// Package trace implements append-only execution traces for skills + Go CLIs.
//
// Per ADR-0101 (replay + golden traces) of the itunified-io/infrastructure
// agentic-AI hardening roadmap. Each Record captures one tool invocation
// (Bash, MCP, internal Go function call) with its input, output, decision,
// and a hash-chain link to the previous record (per ADR-0095).
//
// Records form an append-only JSONL file (one Record per line) at a path
// like ~/.lab/<dbsys>/trace-<ts>.jsonl. The same file is the input to the
// replay command (`proxctl replay --strict <path>`).
package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Decision describes what happened to the tool call.
type Decision string

const (
	DecisionExecuted Decision = "executed"
	DecisionSkipped  Decision = "skipped"
	DecisionDenied   Decision = "denied"
	DecisionFailed   Decision = "failed"
)

// Record is one entry in an execution trace. Append-only. Fields are
// deliberately stable — adding fields requires a SchemaVersion bump.
type Record struct {
	// SchemaVersion is the trace record schema version. Bumped when
	// fields are added/renamed/removed. Replayers MUST refuse traces
	// with unknown SchemaVersion.
	SchemaVersion int `json:"schema_version"`

	// TS is unix epoch nanoseconds at record creation time.
	TS int64 `json:"ts"`

	// StepID is the Plan-RAG step identifier (ADR-0096), e.g.
	// "/lab-up step 5" or "proxctl vm create 2701". Used to correlate
	// across plan file / TodoWrite / Bash description / OTEL spans.
	StepID string `json:"step_id"`

	// Tool is the invoked tool name. Forms:
	//   - "Bash", "Edit", "Write", "Read"  — Claude Code built-ins
	//   - "mcp__<server>__<tool>"          — MCP tool
	//   - "<binary>[:<subcommand>]"        — CLI binary
	//   - "<go-package>.<func>"            — internal Go function
	Tool string `json:"tool"`

	// Input is the raw tool input as JSON. Replayers feed this back to
	// the same tool to re-execute. Contents are tool-specific.
	Input json.RawMessage `json:"input"`

	// Output is the tool's response. May be nil for Decision=Skipped/Denied.
	Output json.RawMessage `json:"output,omitempty"`

	// Decision describes what happened.
	Decision Decision `json:"decision"`

	// DenyRule is the policy rule ID (e.g. "LAB-007") if Decision==Denied.
	DenyRule string `json:"deny_rule,omitempty"`

	// Error is a free-text error message if Decision==Failed. Stable
	// canonical errors (e.g. "vm not found") should match across runs;
	// transient errors (e.g. timeouts with timestamps) MUST be normalized
	// before comparison in --strict replay mode.
	Error string `json:"error,omitempty"`

	// DurationMs is wall-clock duration of the tool call in milliseconds.
	// Replay does NOT enforce duration matching — only Decision + Output.
	DurationMs int64 `json:"duration_ms"`

	// PrevHash is the hex-encoded SHA-256 of the previous Record in the
	// trace, or empty for the first record (genesis).
	PrevHash string `json:"prev_hash,omitempty"`

	// Hash is the hex-encoded SHA-256 of this Record's canonical
	// representation (computed by ComputeHash). Verified by readers
	// to detect tampering.
	Hash string `json:"hash"`
}

// CurrentSchemaVersion is the version stamped on new Records.
const CurrentSchemaVersion = 1

// ErrUnknownSchema is returned when a trace contains a Record with a
// SchemaVersion the replayer doesn't understand.
var ErrUnknownSchema = errors.New("trace: unknown schema_version (refusing to replay)")

// ErrChainBroken is returned when a Record's PrevHash doesn't match the
// hash of the previous Record in the trace.
var ErrChainBroken = errors.New("trace: hash chain broken")

// ErrHashMismatch is returned when a Record's Hash doesn't match its
// re-computed canonical hash.
var ErrHashMismatch = errors.New("trace: record hash mismatch")

// ComputeHash returns the canonical SHA-256 over a Record. The Hash
// field of the input is zeroed during the computation; the returned
// hex string is what should be assigned to r.Hash before serialization.
//
// The canonical form is the JSON-marshaled Record with Hash="" and
// fields in their declared struct order (Go json.Marshal preserves this).
// Adding a field to Record makes records hash-incompatible — bump
// CurrentSchemaVersion in that change.
func (r Record) ComputeHash() (string, error) {
	r.Hash = ""
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Validate checks intrinsic consistency: schema version is recognized,
// hash matches the canonical re-compute. Does NOT verify the chain
// (use VerifyChain on a slice of Records for that).
func (r Record) Validate() error {
	if r.SchemaVersion != CurrentSchemaVersion {
		return ErrUnknownSchema
	}
	want, err := r.ComputeHash()
	if err != nil {
		return err
	}
	if want != r.Hash {
		return ErrHashMismatch
	}
	return nil
}

// VerifyChain validates that each record's PrevHash equals the previous
// record's Hash, and each record's Hash matches its canonical re-compute.
// Returns the index of the first broken record, or -1 if the entire
// chain is valid.
func VerifyChain(records []Record) (int, error) {
	for i, r := range records {
		if err := r.Validate(); err != nil {
			return i, err
		}
		if i == 0 {
			if r.PrevHash != "" {
				return 0, ErrChainBroken
			}
			continue
		}
		if r.PrevHash != records[i-1].Hash {
			return i, ErrChainBroken
		}
	}
	return -1, nil
}
