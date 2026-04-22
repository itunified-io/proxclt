// Package state provides the SQLite-backed persistence layer for proxctl.
//
// Phase 1 (scaffold): types + stubs. Real implementation lands in Phase 2
// with modernc.org/sqlite (pure Go, no CGO) per design doc §4.5.
package state

import (
	"errors"
	"time"
)

// ErrNotImplemented is returned by stubbed methods during scaffold phase.
var ErrNotImplemented = errors.New("state: not implemented yet (scaffold)")

// DB is the handle for the proxctl state database (~/.proxctl/state.db).
type DB struct {
	path string
}

// VM is a row in the `vms` table.
type VM struct {
	ID        int
	Context   string
	Node      string
	Name      string
	EnvPath   string
	EnvSHA256 string
	Distro    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	SpecJSON  string
}

// Open opens (and initialises) the state DB at path.
// Phase 1: returns a handle but does not create any tables.
func Open(path string) (*DB, error) {
	return &DB{path: path}, nil
}

// Close releases the underlying connection.
func (d *DB) Close() error { return nil }

// Path returns the configured DB file path.
func (d *DB) Path() string { return d.path }

// InitSchema will create the `vms`, `kickstarts`, `snapshots`, `apply_history` tables.
// Phase 1 stub.
func (d *DB) InitSchema() error { return ErrNotImplemented }

// RecordVM persists a VM row. Phase 1 stub.
func (d *DB) RecordVM(_ VM) error { return ErrNotImplemented }
