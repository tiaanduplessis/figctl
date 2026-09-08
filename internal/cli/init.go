package cli

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/ref"
)

type initResult struct {
	Path    string            `json:"path"`
	Profile string            `json:"profile"`
	Files   map[string]string `json:"files,omitempty"`
	Default string            `json:"default,omitempty"`
}

func (r initResult) Columns() []string { return []string{"field", "value"} }

func (r initResult) Rows() [][]string {
	kv := output.KeyValues{{"path", r.Path}, {"profile", r.Profile}}
	for _, name := range sortedKeys(r.Files) {
		kv = append(kv, [2]string{"file " + name, r.Files[name]})
	}
	if r.Default != "" {
		kv = append(kv, [2]string{"default", r.Default})
	}
	return kv.Rows()
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Write .figctl.yaml in the current directory with the profile to use",
	Long: `Write .figctl.yaml in the current directory. The file names the profile
and the Figma files this repository works with, and contains no secrets, so it
can be committed. Commands walk up from the working directory to find it.

Name each file with --file <name>=<key or URL>. A repository usually refers to
more than one, because a design system lives in its own file, and a name can
then be used wherever a command takes a ref:

  figctl file tree design-system
  figctl tokens export design-system --format css

--default picks the file used when a command is given no ref at all. With a
single configured file that is implied.`,
	Example: `  figctl init --profile acme --file web=https://www.figma.com/design/KEY/Web-App
  figctl init --profile acme --file app=KEY1 --file design-system=KEY2 --default app`,
	Args: cobra.NoArgs,
}

func init() {
	files := initCmd.Flags().StringArray("file", nil, "a named Figma file as <name>=<key or URL>; repeatable")
	defaultFile := initCmd.Flags().String("default", "", "name of the file used when a command is given no ref")
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
		project := &config.Project{Profile: name, Default: *defaultFile}
		if len(*files) > 0 {
			project.Files = map[string]string{}
		}
		for _, entry := range *files {
			fileName, value, found := strings.Cut(entry, "=")
			fileName = strings.TrimSpace(fileName)
			if !found || fileName == "" || strings.TrimSpace(value) == "" {
				return figctl.Newf(figctl.CodeUsage, "--file %q is not in the form <name>=<key or URL>", entry).
					WithHint("For example --file design-system=https://www.figma.com/design/KEY/Design-System.")
			}
			if err := config.ValidateName(fileName); err != nil {
				return err
			}
			parsed, err := ref.Parse(strings.TrimSpace(value))
			if err != nil {
				return err
			}
			project.Files[fileName] = parsed.FileKey
		}
		if err := project.Validate(); err != nil {
			return err
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
		env := ctx.Envelope(initResult{Path: path, Profile: name, Files: project.Files, Default: project.Default})
		if _, ok := cfg.Profiles[name]; !ok {
			env.AddHint("Profile " + name + " is not configured on this machine yet; run figctl auth login --profile " + name + ".")
		}
		env.AddHint("Commit " + config.ProjectFileName + " so teammates and agents use the same profile.")
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(initCmd)
}

// sortedKeys returns map keys in a stable order, so repeated runs print the
// same rows.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
