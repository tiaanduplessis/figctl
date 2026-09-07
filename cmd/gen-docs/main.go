// Command gen-docs writes the figctl command reference to docs/commands.md.
// Run it with "make docs" after changing any command, flag, or help text; a
// test in internal/docs fails when the committed file has drifted.
package main

import (
	"fmt"
	"os"

	"github.com/tiaanduplessis/figctl/internal/cli"
	"github.com/tiaanduplessis/figctl/internal/docs"
)

// outPath is where the reference is written, relative to the module root.
const outPath = "docs/commands.md"

func main() {
	root := cli.Root()
	root.InitDefaultCompletionCmd()
	if err := os.WriteFile(outPath, []byte(docs.Markdown(root)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "gen-docs:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "wrote "+outPath)
}
