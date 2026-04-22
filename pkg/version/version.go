// Package version exposes the baked-in build metadata for proxctl.
// Values are injected via -ldflags at build time (see .goreleaser.yaml).
package version

// Version is the semantic / CalVer version of this binary (e.g. v2026.04.11.1).
var Version = "dev"

// Commit is the short git commit the binary was built from.
var Commit = "none"

// Date is the ISO-8601 build date.
var Date = "unknown"
