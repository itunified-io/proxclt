package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Mode controls replay behavior.
type Mode int

const (
	// ModeStrict re-executes each tool and asserts the actual output
	// matches the recorded output exactly (after normalization).
	// Returns non-nil error on any divergence.
	ModeStrict Mode = iota

	// ModeObserve re-executes each tool but only logs divergence;
	// returns nil unless an actual execution error occurs.
	ModeObserve

	// ModeDryRun does NOT re-execute; just walks the trace, validates
	// the hash chain, and prints what would be replayed.
	ModeDryRun
)

// Executor is the interface a replayer needs from its environment to
// re-execute a Record's tool call. Real implementations dispatch to
// Bash, MCP servers, or the host process. Tests can supply mocks.
type Executor interface {
	// Execute runs the tool described by rec and returns its raw
	// output as JSON bytes. The implementation MUST NOT mutate rec.
	Execute(ctx context.Context, rec Record) (json.RawMessage, error)
}

// Result reports the outcome of replaying a single Record.
type Result struct {
	StepID     string
	Tool       string
	Match      bool   // output matches recorded
	Diff       string // human-readable diff if !Match
	ExecErr    error  // error from Executor.Execute, nil if it ran
	Diverged   bool   // true if either Match==false or ExecErr!=nil
}

// Replay walks records in order and re-executes via exec, comparing
// outputs to the recorded ones. Returns one Result per record.
//
// In ModeStrict, an error is returned at the first divergence (caller
// can still inspect the partial Results slice). In ModeObserve, all
// records are replayed and divergences are logged via results.
func Replay(ctx context.Context, records []Record, exec Executor, mode Mode, log io.Writer) ([]Result, error) {
	if log == nil {
		log = io.Discard
	}
	results := make([]Result, 0, len(records))

	if idx, err := VerifyChain(records); err != nil {
		return nil, fmt.Errorf("replay: chain verification failed at record %d: %w", idx, err)
	}

	for i, rec := range records {
		res := Result{StepID: rec.StepID, Tool: rec.Tool}

		if mode == ModeDryRun {
			fmt.Fprintf(log, "[%d] DRY-RUN %s tool=%s decision=%s\n", i, rec.StepID, rec.Tool, rec.Decision)
			res.Match = true
			results = append(results, res)
			continue
		}

		// Skipped/Denied/Failed records are NOT re-executed in any mode —
		// they were not "real" actions originally.
		if rec.Decision != DecisionExecuted {
			fmt.Fprintf(log, "[%d] SKIP %s tool=%s decision=%s (no re-execution)\n", i, rec.StepID, rec.Tool, rec.Decision)
			res.Match = true
			results = append(results, res)
			continue
		}

		got, err := exec.Execute(ctx, rec)
		if err != nil {
			res.ExecErr = err
			res.Diverged = true
			fmt.Fprintf(log, "[%d] FAIL %s tool=%s exec error: %v\n", i, rec.StepID, rec.Tool, err)
			results = append(results, res)
			if mode == ModeStrict {
				return results, fmt.Errorf("replay: exec failed at record %d (%s): %w", i, rec.StepID, err)
			}
			continue
		}

		if equalNormalized(got, rec.Output) {
			res.Match = true
			fmt.Fprintf(log, "[%d] OK   %s tool=%s\n", i, rec.StepID, rec.Tool)
			results = append(results, res)
			continue
		}

		res.Diff = fmt.Sprintf("recorded: %s\nactual:   %s", string(rec.Output), string(got))
		res.Diverged = true
		fmt.Fprintf(log, "[%d] DIFF %s tool=%s\n%s\n", i, rec.StepID, rec.Tool, res.Diff)
		results = append(results, res)
		if mode == ModeStrict {
			return results, fmt.Errorf("replay: output divergence at record %d (%s)", i, rec.StepID)
		}
	}
	return results, nil
}

// equalNormalized compares two JSON byte slices for semantic equality.
// Re-marshaling normalizes key order + whitespace.
func equalNormalized(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	var av, bv interface{}
	if err := json.Unmarshal(a, &av); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return bytes.Equal(a, b)
	}
	an, _ := json.Marshal(av)
	bn, _ := json.Marshal(bv)
	return bytes.Equal(an, bn)
}
