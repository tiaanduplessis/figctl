// Command figctl is an agent-first Figma CLI.
package main

import (
	"os"

	"github.com/tiaanduplessis/figctl/internal/cli"
)

func main() {
	os.Exit(cli.Run(cli.Options{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
