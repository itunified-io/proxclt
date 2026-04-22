// Package proxmox provides a typed REST client for the Proxmox VE API.
package proxmox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// APIError is returned when the Proxmox API responds with a non-2xx status
// or an error envelope (`{"errors": {...}}`).
type APIError struct {
	StatusCode int
	Message    string
	Errors     map[string]string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "proxmox api error: status=%d", e.StatusCode)
	if e.Message != "" {
		fmt.Fprintf(&b, " message=%q", e.Message)
	}
	if len(e.Errors) > 0 {
		keys := make([]string, 0, len(e.Errors))
		for k := range e.Errors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString(" errors={")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s=%q", k, e.Errors[k])
		}
		b.WriteString("}")
	}
	return b.String()
}

// Sentinel errors for well-known not-found conditions.
var (
	ErrVMNotFound       = errors.New("proxmox: vm not found")
	ErrSnapshotNotFound = errors.New("proxmox: snapshot not found")
	ErrStorageNotFound  = errors.New("proxmox: storage not found")
)
