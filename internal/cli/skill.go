package cli

import (
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/skill"
)

// skillResult is the payload of skill install and skill uninstall.
type skillResult struct {
	Agent   string         `json:"agent"`
	Project bool           `json:"project"`
	DryRun  bool           `json:"dryRun"`
	Actions []skill.Action `json:"actions"`
	// Paths lists the files that changed, in the order they were written.
	Paths []string `json:"paths"`
}

// Columns implements output.Tabular.
func (r skillResult) Columns() []string { return []string{"agent", "op", "bytes", "path"} }

// Rows implements output.Tabular.
func (r skillResult) Rows() [][]string {
	rows := make([][]string, 0, len(r.Actions))
	for _, a := range r.Actions {
		rows = append(rows, []string{string(a.Agent), a.Op, strconv.Itoa(a.Bytes), a.Path})
	}
	return rows
}

// skillPrintResult is the payload of skill print. In every mode but JSON
// the document itself is written to stdout instead of an envelope, so it
// can be redirected into a file.
type skillPrintResult struct {
	Agent   string `json:"agent"`
	File    string `json:"file"`
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Content string `json:"content"`
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Install the figctl agent skill into a coding agent",
	Long: `Write the figctl skill (an onboarding document plus a command, output
schema, and workflow reference) into the place a coding agent reads it
from. The reference files are generated from the same command tree and
JSON Schemas the CLI uses, so they cannot drift from the binary.`,
	Example: `  figctl skill install
  figctl skill install --agent all --project
  figctl skill print --file workflows -o md`,
}

var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Write the skill files for one or more agents",
	Long: `Write the skill for the selected agent.

  claude   ~/.claude/skills/figctl/ (SKILL.md and reference/), or
           .claude/skills/figctl/ with --project
  cursor   .cursor/rules/figctl.mdc
  copilot  a marked block in .github/copilot-instructions.md
  codex    a marked block in AGENTS.md
  generic  the same AGENTS.md block as codex
  all      claude, cursor, copilot, and codex

Every target but claude is project scoped and is written under the
working directory whether or not --project is given.

Installs are idempotent. Files figctl owns carry a marker comment and are
replaced in place; a file of the same name that figctl did not write is
left alone unless --force. Shared instruction files keep everything
outside the <!-- BEGIN figctl --> and <!-- END figctl --> markers. Run it
again after upgrading figctl to refresh the generated reference.`,
	Example: `  figctl skill install
  figctl skill install --agent claude --project
  figctl skill install --agent all --dry-run`,
	Args: cobra.NoArgs,
}

var skillPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Print a skill document to stdout",
	Long: `Print one skill document instead of installing it, for manual
placement or for reading the reference without a file on disk.

--file selects the document: SKILL, commands, schemas, or workflows.
--agent selects the shape of SKILL: the skill file for claude, the .mdc
rule for cursor, and the marked block for copilot, codex, and generic.
The reference documents are the same for every agent.

In JSON mode the document is a string field of the envelope; in every
other mode the document itself is written to stdout, so "-o md" can be
redirected into a file.`,
	Example: `  figctl skill print --file workflows -o md
  figctl skill print --agent codex -o md >> AGENTS.md
  figctl skill print --file commands --json`,
	Args: cobra.NoArgs,
}

var skillUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the skill files or marked blocks",
	Long: `Remove what skill install wrote: the files figctl owns, and the
<!-- BEGIN figctl --> block from shared instruction files, leaving the
rest of those files untouched. A file figctl did not write is left alone
unless --force.

Removing files needs a confirmation, so pass --yes when stdin is not a
terminal.`,
	Example: `  figctl skill uninstall --yes
  figctl skill uninstall --agent all --project --yes`,
	Args: cobra.NoArgs,
}

// skillOptions builds the install options from the flags and the
// environment.
func skillOptions(ctx *Context, agent string, project, force, dryRun bool) (skill.Options, error) {
	parsed, err := skill.ParseAgent(agent)
	if err != nil {
		return skill.Options{}, figctl.Newf(figctl.CodeUsage, "%s", err.Error()).
			WithHint("Use --agent %s.", strings.Join(skill.AgentNames(), "|"))
	}
	cwd, err := ctx.Cwd()
	if err != nil {
		return skill.Options{}, err
	}
	home, err := ctx.Home()
	if err != nil {
		return skill.Options{}, err
	}
	return skill.Options{Agent: parsed, Home: home, Dir: cwd, Project: project, Force: force, DryRun: dryRun}, nil
}

// skillEnvelope wraps the actions of an install or uninstall.
func skillEnvelope(ctx *Context, opts skill.Options, actions []skill.Action) *output.Envelope {
	result := skillResult{
		Agent:   string(opts.Agent),
		Project: opts.Project,
		DryRun:  opts.DryRun,
		Actions: actions,
		Paths:   []string{},
	}
	skipped := 0
	for _, a := range actions {
		if a.Written() {
			result.Paths = append(result.Paths, a.Path)
		}
		if a.Op == skill.OpSkip {
			skipped++
		}
	}
	env := ctx.Envelope(result)
	if opts.DryRun {
		env.AddHint("Dry run: nothing was written. Re-run without --dry-run to apply " + strconv.Itoa(len(result.Paths)) + " change(s).")
	}
	if skipped > 0 {
		env.AddHint(strconv.Itoa(skipped) + " file(s) were left alone because figctl did not write them; pass --force to replace them.")
	}
	return env
}

func init() {
	installAgent := skillInstallCmd.Flags().String("agent", string(skill.AgentClaude), "target agent: "+strings.Join(skill.AgentNames(), ", "))
	installProject := skillInstallCmd.Flags().Bool("project", false, "install the claude skill into this repository instead of the home directory")
	installForce := skillInstallCmd.Flags().Bool("force", false, "replace a file of the same name that figctl did not write")
	installDryRun := skillInstallCmd.Flags().Bool("dry-run", false, "list what would be written, with byte counts, without writing")
	skillInstallCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		opts, err := skillOptions(ctx, *installAgent, *installProject, *installForce, *installDryRun)
		if err != nil {
			return err
		}
		actions, err := skill.Install(opts)
		if err != nil {
			return figctl.Wrap(figctl.CodeInternal, err, err.Error()).
				WithHint("Check the target directory is writable.")
		}
		env := skillEnvelope(ctx, opts, actions)
		if !opts.DryRun {
			env.AddHint("Start a new agent session so the skill is picked up.")
		}
		return ctx.Printer.Print(env)
	})

	printAgent := skillPrintCmd.Flags().String("agent", string(skill.AgentClaude), "shape of the SKILL document: "+strings.Join(skill.AgentNames(), ", "))
	printFile := skillPrintCmd.Flags().String("file", "SKILL", "document to print: "+strings.Join(skill.FileNames(), ", "))
	skillPrintCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		agent, err := skill.ParseAgent(*printAgent)
		if err != nil {
			return figctl.Newf(figctl.CodeUsage, "%s", err.Error()).
				WithHint("Use --agent %s.", strings.Join(skill.AgentNames(), "|"))
		}
		content, err := skill.Render(agent, *printFile)
		if err != nil {
			return figctl.Newf(figctl.CodeUsage, "%s", err.Error()).
				WithHint("Use --file %s.", strings.Join(skill.FileNames(), "|"))
		}
		if ctx.Printer.Mode != output.ModeJSON {
			_, err := io.WriteString(ctx.Stdout, content)
			return err
		}
		file, _ := skill.Lookup(*printFile)
		env := ctx.Envelope(skillPrintResult{
			Agent:   string(agent),
			File:    file.Name,
			Path:    skill.Location(agent, file.Name),
			Bytes:   len(content),
			Content: content,
		})
		env.AddHint("Add -o md to write the document itself to stdout instead of this envelope.")
		return ctx.Printer.Print(env)
	})

	uninstallAgent := skillUninstallCmd.Flags().String("agent", string(skill.AgentClaude), "target agent: "+strings.Join(skill.AgentNames(), ", "))
	uninstallProject := skillUninstallCmd.Flags().Bool("project", false, "remove the claude skill from this repository instead of the home directory")
	uninstallForce := skillUninstallCmd.Flags().Bool("force", false, "remove a file of the same name that figctl did not write")
	uninstallDryRun := skillUninstallCmd.Flags().Bool("dry-run", false, "list what would be removed without removing it")
	skillUninstallCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		opts, err := skillOptions(ctx, *uninstallAgent, *uninstallProject, *uninstallForce, *uninstallDryRun)
		if err != nil {
			return err
		}
		if !opts.DryRun {
			if err := confirm(ctx, "Remove the figctl skill for "+string(opts.Agent)+"?"); err != nil {
				return err
			}
		}
		actions, err := skill.Uninstall(opts)
		if err != nil {
			return figctl.Wrap(figctl.CodeInternal, err, err.Error()).
				WithHint("Check the target directory is writable.")
		}
		return ctx.Printer.Print(skillEnvelope(ctx, opts, actions))
	})

	skillCmd.AddCommand(skillInstallCmd, skillPrintCmd, skillUninstallCmd)
	rootCmd.AddCommand(skillCmd)
}
