package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/itunified-io/proxctl/pkg/config"
	"github.com/itunified-io/proxctl/pkg/kickstart"
	"github.com/itunified-io/proxctl/pkg/proxmox"
)

// defaultMaxConcurrency is the default cap on concurrent per-node workflows.
const defaultMaxConcurrency = 4

// MultiNodeWorkflow fans one env manifest out to many nodes, each driven by an
// internal SingleVMWorkflow. VM creation and boot run in parallel; the shared
// install-ISO / kickstart-ISO upload path is serialized by ISOUploadMu to
// avoid racing the Proxmox storage upload endpoint.
type MultiNodeWorkflow struct {
	Config   *config.Env
	Client   *proxmox.Client
	Renderer *kickstart.Renderer
	Builder  *kickstart.ISOBuilder
	DryRun   bool

	// MaxConcurrency caps concurrent per-node Apply goroutines. <=0 → 4.
	MaxConcurrency int
	// ContinueOnError keeps running other nodes when one fails.
	ContinueOnError bool
	// ISOUploadMu serializes ISO uploads across nodes. If nil, one is allocated
	// internally.
	ISOUploadMu *sync.Mutex
	// SkipKickstartBuild propagates to every per-node SingleVMWorkflow. See
	// SingleVMWorkflow.SkipKickstartBuild.
	SkipKickstartBuild bool
}

// NewMultiNodeWorkflow is a small factory that fills sensible defaults.
func NewMultiNodeWorkflow(cfg *config.Env, client *proxmox.Client, rnd *kickstart.Renderer, builder *kickstart.ISOBuilder) *MultiNodeWorkflow {
	return &MultiNodeWorkflow{
		Config:         cfg,
		Client:         client,
		Renderer:       rnd,
		Builder:        builder,
		MaxConcurrency: defaultMaxConcurrency,
		ISOUploadMu:    &sync.Mutex{},
	}
}

// NodeResult captures per-node execution outcome.
type NodeResult struct {
	Node    string
	Success bool
	Err     error
	Changes []Change
}

// nodeNames returns the deterministic sorted list of node names from the
// manifest.
func (m *MultiNodeWorkflow) nodeNames() ([]string, error) {
	if m.Config == nil {
		return nil, errors.New("multi-node: Config is nil")
	}
	hyp := m.Config.Spec.Hypervisor.Resolved()
	if hyp == nil {
		return nil, errors.New("multi-node: hypervisor not resolved")
	}
	names := make([]string, 0, len(hyp.Nodes))
	for n := range hyp.Nodes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// perNode builds a SingleVMWorkflow for a given node, inheriting shared fields
// including the cross-node ISO-upload mutex.
func (m *MultiNodeWorkflow) perNode(name string) *SingleVMWorkflow {
	return &SingleVMWorkflow{
		Config:             m.Config,
		NodeName:           name,
		Client:             m.Client,
		Renderer:           m.Renderer,
		Builder:            m.Builder,
		DryRun:             m.DryRun,
		UploadMu:           m.isoMu(),
		SkipKickstartBuild: m.SkipKickstartBuild,
	}
}

// maxConcurrency returns the effective limit.
func (m *MultiNodeWorkflow) maxConcurrency() int {
	if m.MaxConcurrency > 0 {
		return m.MaxConcurrency
	}
	return defaultMaxConcurrency
}

// isoMu returns the effective ISO upload mutex.
func (m *MultiNodeWorkflow) isoMu() *sync.Mutex {
	if m.ISOUploadMu != nil {
		return m.ISOUploadMu
	}
	m.ISOUploadMu = &sync.Mutex{}
	return m.ISOUploadMu
}

// Plan computes per-node change sets sequentially (plan is cheap, read-only).
// The result is keyed by node name.
func (m *MultiNodeWorkflow) Plan(ctx context.Context) (map[string][]Change, error) {
	names, err := m.nodeNames()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]Change, len(names))
	for _, name := range names {
		w := m.perNode(name)
		changes, err := w.Plan(ctx)
		if err != nil {
			return nil, fmt.Errorf("plan %s: %w", name, err)
		}
		out[name] = changes
	}
	return out, nil
}

// applyNode runs the full change list for one node. SingleVMWorkflow.Apply
// honours the shared UploadMu so cross-node upload serialisation comes for
// free.
func (m *MultiNodeWorkflow) applyNode(ctx context.Context, name string, changes []Change) error {
	return m.perNode(name).Apply(ctx, changes)
}

// Apply runs per-node Apply concurrently using errgroup + a semaphore. In
// fail-fast mode, the first error cancels the context. ContinueOnError keeps
// remaining nodes running and aggregates errors.
func (m *MultiNodeWorkflow) Apply(ctx context.Context) error {
	plan, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	return m.applyPlan(ctx, plan)
}

func (m *MultiNodeWorkflow) applyPlan(ctx context.Context, plan map[string][]Change) error {
	names, err := m.nodeNames()
	if err != nil {
		return err
	}
	sem := make(chan struct{}, m.maxConcurrency())

	if m.ContinueOnError {
		var (
			wg   sync.WaitGroup
			emu  sync.Mutex
			errs []error
		)
		for _, name := range names {
			name := name
			changes := plan[name]
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				if err := m.applyNode(ctx, name, changes); err != nil {
					emu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", name, err))
					emu.Unlock()
				}
			}()
		}
		wg.Wait()
		return errors.Join(errs...)
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, name := range names {
		name := name
		changes := plan[name]
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()
			if err := m.applyNode(gctx, name, changes); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			return nil
		})
	}
	return g.Wait()
}

// Verify runs per-node Verify concurrently, aggregating errors.
func (m *MultiNodeWorkflow) Verify(ctx context.Context) error {
	names, err := m.nodeNames()
	if err != nil {
		return err
	}
	sem := make(chan struct{}, m.maxConcurrency())
	var (
		wg   sync.WaitGroup
		emu  sync.Mutex
		errs []error
	)
	for _, name := range names {
		name := name
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := m.perNode(name).Verify(ctx); err != nil {
				emu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				emu.Unlock()
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Rollback reverses in-flight Apply best-effort across all nodes. Errors are
// aggregated so the caller sees everything that failed.
func (m *MultiNodeWorkflow) Rollback(ctx context.Context) error {
	names, err := m.nodeNames()
	if err != nil {
		return err
	}
	// Reverse order: if there was any dependency ordering in Apply (alphabetical),
	// undo in reverse to tear down dependents before their dependencies.
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	var errs []error
	for _, name := range names {
		if err := m.perNode(name).Rollback(ctx, nil); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Up runs Plan → Apply → Verify honouring DryRun. Verify failures are reported
// as warnings (matching SingleVMWorkflow.Up).
func (m *MultiNodeWorkflow) Up(ctx context.Context) error {
	plan, err := m.Plan(ctx)
	if err != nil {
		return err
	}
	if m.DryRun {
		// Dry-run: exercise Apply on each node sequentially so the dry-run log
		// is deterministic per node.
		for name, changes := range plan {
			if err := m.perNode(name).Apply(ctx, changes); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	}
	if err := m.applyPlan(ctx, plan); err != nil {
		return err
	}
	if err := m.Verify(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warn: verify: %v\n", err)
	}
	return nil
}

// Down tears down every node. Reverse-alphabetical order so network-ish names
// (e.g. "net-*") come down after VM nodes in typical conventions.
func (m *MultiNodeWorkflow) Down(ctx context.Context, force bool) error {
	names, err := m.nodeNames()
	if err != nil {
		return err
	}
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	var errs []error
	for _, name := range names {
		if err := m.perNode(name).Down(ctx, force); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
