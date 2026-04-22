// Command docgen renders the proxctl Cobra command tree into Markdown
// under docs/cli-reference/.
//
// Usage:
//
//	go run ./cmd/docgen [out-dir]
//
// The default out-dir is docs/cli-reference. One Markdown file per command
// is produced (proxctl.md, proxctl_config.md, proxctl_config_validate.md, …)
// plus a cross-linked index page. Commit the generated files so GitHub UI
// renders them directly.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"

	"github.com/itunified-io/proxctl/internal/root"
)

func main() {
	out := "docs/cli-reference"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "docgen: mkdir:", err)
		os.Exit(1)
	}
	r := root.New()
	r.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(r, out); err != nil {
		fmt.Fprintln(os.Stderr, "docgen: gen:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "docgen: wrote Markdown tree to %s\n", filepath.Clean(out))
}
