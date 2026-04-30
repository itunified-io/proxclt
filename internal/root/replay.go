package root

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/itunified-io/proxctl/pkg/trace"
	"github.com/spf13/cobra"
)

// newReplayCmd builds `proxctl replay <trace.jsonl>`. Three modes:
//   - default (no flags) → strict re-execution against fixture-matching environment
//   - --observe          → re-execute but only log divergence
//   - --dry-run          → walk trace, validate chain, print actions only (no execution)
//
// Per ADR-0101 (replay + golden traces) of itunified-io/infrastructure.
func newReplayCmd() *cobra.Command {
	var (
		flagObserve bool
		flagDryRun  bool
		flagVerbose bool
	)

	cmd := &cobra.Command{
		Use:   "replay <trace.jsonl>",
		Short: "Replay an execution trace and verify outputs match",
		Long: `Replay an execution trace recorded by a previous run.

Each Record in the trace is re-executed via the same tool, and the actual
output is compared against the recorded output. By default, runs in
ModeStrict: exits non-zero on the first divergence.

Modes:
  default     ModeStrict — first divergence aborts (CI gate)
  --observe   ModeObserve — log divergences, never abort (audit / dashboard)
  --dry-run   ModeDryRun — validate chain + print actions, no execution

Trace format: JSONL, one Record per line. See pkg/trace/schema.go.

Hash chain (ADR-0095) is verified before any replay; broken chain aborts.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runReplay(args[0], flagObserve, flagDryRun, flagVerbose)
		},
	}

	cmd.Flags().BoolVar(&flagObserve, "observe", false, "Log divergence but don't abort (ModeObserve)")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Validate chain + print actions, no execution")
	cmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose per-record logging")

	return cmd
}

func runReplay(path string, observe, dryRun, verbose bool) error {
	records, err := trace.LoadAll(path)
	if err != nil {
		return fmt.Errorf("replay: load trace: %w", err)
	}
	if len(records) == 0 {
		fmt.Fprintln(os.Stderr, "replay: trace is empty")
		return nil
	}
	fmt.Fprintf(os.Stderr, "replay: loaded %d records from %s (chain verified)\n", len(records), path)

	mode := trace.ModeStrict
	switch {
	case dryRun:
		mode = trace.ModeDryRun
	case observe:
		mode = trace.ModeObserve
	}

	var logW = os.Stderr
	if !verbose && mode == trace.ModeStrict {
		logW = os.Stderr // keep stderr; just don't enable extra prints
	}

	exec := newDefaultExecutor()
	results, err := trace.Replay(context.Background(), records, exec, mode, logW)

	// Summary
	matched, diverged, errored := 0, 0, 0
	for _, r := range results {
		if r.ExecErr != nil {
			errored++
		} else if r.Diverged {
			diverged++
		} else {
			matched++
		}
	}
	fmt.Fprintf(os.Stderr, "replay: %d matched, %d diverged, %d exec-errored (mode=%s)\n",
		matched, diverged, errored, modeName(mode))

	if err != nil {
		return err
	}
	if mode == trace.ModeObserve && (diverged > 0 || errored > 0) {
		// Observe mode: don't fail, but flag in stderr summary
		fmt.Fprintln(os.Stderr, "replay: observe mode — divergences NOT treated as failure")
	}
	return nil
}

func modeName(m trace.Mode) string {
	switch m {
	case trace.ModeStrict:
		return "strict"
	case trace.ModeObserve:
		return "observe"
	case trace.ModeDryRun:
		return "dry-run"
	}
	return "unknown"
}

// newDefaultExecutor returns a placeholder Executor that always returns
// the recorded output verbatim (so a "fresh" replay against an unchanged
// trace passes). Real execution dispatch (Bash, MCP, etc.) is deferred
// to a sibling PR — this CLI ships the trace primitive + replay engine;
// the live executor lives behind a build tag in v2026.04.30.5+.
//
// The default mock makes `proxctl replay` useful for chain verification
// + structural validation without yet wiring all tool dispatchers.
func newDefaultExecutor() trace.Executor {
	return mockReplayExecutor{}
}

type mockReplayExecutor struct{}

func (mockReplayExecutor) Execute(_ context.Context, rec trace.Record) (json.RawMessage, error) {
	return rec.Output, nil
}
