// proxctl — Proxmox VM provisioning CLI.
// See docs/ and design doc 024-proxctl-design.md for the full spec.
package main

import "github.com/itunified-io/proxctl/internal/root"

func main() {
	root.Execute()
}
