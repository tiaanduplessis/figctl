package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/ref"
)

type initResult struct {
	Path    string `json:"path"`
	Profile string `json:"profile"`
	File    string `json:"file,omitempty"`
}

func (r initResult) Columns() []string { return []string{"field", "value"} }

func (r initResult) Rows() [][]string {
	kv := output.KeyValues{{"path", r.Path}, {"profile", r.Profile}}
	if r.File != "" {
		kv = append(kv, [2]string{"file", r.File})
	}
	return kv.Rows()
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Write .figctl.yaml in the current directory with the profile to use",
	Long: `Write .figctl.yaml in the current directory. The file names the profile
(and optionally the default Figma file) for this repository and contains no
secrets, so it can be committed. Commands walk up from the working directory
to find it.`,
	Example: `  figctl init --profile acme
  figctl init --profile acme --file https://www.figma.com/design/KEY/Web-App`,
	Args: cobra.NoArgs,
}

func init() {
	file := initCmd.Flags().String("file", "", "default file key or Figma URL for this repository")
	initCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		name := ctx.Flags.Profile
		if name == "" {
			name = cfg.DefaultProfile
		}
		if name == "" {
			return figctl.New(figctl.CodeUsage, "no profile to write").
				WithHint("Run figctl init --profile <name>, or set a default with figctl profile use <name>.")
		}
		if err := config.ValidateName(name); err != nil {
			return err
		}
		project := &config.Project{Profile: name}
		if *file != "" {
			parsed, err := ref.Parse(*file)
			if err != nil {
				return err
			}
			project.File = parsed.FileKey
		}
		cwd, err := ctx.Cwd()
		if err != nil {
			return err
		}
		path := filepath.Join(cwd, config.ProjectFileName)
		if _, err := os.Stat(path); err == nil {
			if err := confirm(ctx, "Overwrite "+path+"?"); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return figctl.Wrap(figctl.CodeInternal, err, "checking "+path+": "+err.Error())
		}
		if _, err := project.Save(cwd); err != nil {
			return err
		}
		env := ctx.Envelope(initResult{Path: path, Profile: name, File: project.File})
		if _, ok := cfg.Profiles[name]; !ok {
			env.AddHint("Profile " + name + " is not configured on this machine yet; run figctl auth login --profile " + name + ".")
		}
		env.AddHint("Commit " + config.ProjectFileName + " so teammates and agents use the same profile.")
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(initCmd)
}
