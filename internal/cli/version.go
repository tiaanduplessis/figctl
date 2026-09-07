package cli

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/output"
)

// Set at build time through -ldflags (see the Makefile).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type versionInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Date        string `json:"date"`
	GoVersion   string `json:"goVersion"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	UpdateCheck string `json:"updateCheck,omitempty"`
}

func (v versionInfo) Columns() []string { return []string{"field", "value"} }

func (v versionInfo) Rows() [][]string {
	kv := output.KeyValues{
		{"version", v.Version},
		{"commit", v.Commit},
		{"date", v.Date},
		{"goVersion", v.GoVersion},
		{"os", v.OS},
		{"arch", v.Arch},
	}
	if v.UpdateCheck != "" {
		kv = append(kv, [2]string{"updateCheck", v.UpdateCheck})
	}
	return kv.Rows()
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the figctl version",
	Example: "  figctl version\n  figctl version --json",
	Args:    cobra.NoArgs,
}

func init() {
	check := versionCmd.Flags().Bool("check", false, "check for a newer release")
	versionCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		info := versionInfo{
			Version:   version,
			Commit:    commit,
			Date:      date,
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
		}
		if *check {
			info.UpdateCheck = "not implemented"
		}
		env := ctx.Envelope(info)
		if *check {
			env.AddHint("Release checking (--check) is not implemented yet. See https://github.com/tiaanduplessis/figctl/releases for new versions.")
		}
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(versionCmd)
}
