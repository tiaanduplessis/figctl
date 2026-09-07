package cli

import (
	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
)

type projectRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type projectList []projectRow

func (l projectList) Columns() []string { return []string{"id", "name"} }

func (l projectList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, p := range l {
		rows = append(rows, []string{p.ID, p.Name})
	}
	return rows
}

type fileRow struct {
	Key          string       `json:"key"`
	Name         string       `json:"name"`
	LastModified string       `json:"lastModified"`
	Branches     []branchInfo `json:"branches,omitempty"`
}

type fileList []fileRow

func (l fileList) Columns() []string { return []string{"key", "name", "lastModified"} }

func (l fileList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, f := range l {
		rows = append(rows, []string{f.Key, f.Name, f.LastModified})
	}
	return rows
}

func fileListFrom(files []figma.FileSummary) fileList {
	list := fileList{}
	for _, f := range files {
		row := fileRow{Key: f.Key, Name: f.Name, LastModified: f.LastModified}
		for _, b := range f.Branches {
			row.Branches = append(row.Branches, branchInfo{Key: b.Key, Name: b.Name, LastModified: b.LastModified})
		}
		list = append(list, row)
	}
	return list
}

// teamIDArg returns the team id from the argument or the profile default.
func teamIDArg(session *Session, args []string) (string, error) {
	if len(args) == 1 && args[0] != "" {
		return args[0], nil
	}
	if session.Active.Profile.TeamID != "" {
		return session.Active.Profile.TeamID, nil
	}
	return "", figctl.New(figctl.CodeUsage, "team id required").
		WithHint("Pass the team id (from the team URL https://www.figma.com/files/team/<id>/...) or set one with figctl profile add <name> --team <id>.")
}

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Discover projects and files (v1 API, deprecated in favor of folders)",
}

var projectsListCmd = &cobra.Command{
	Use:     "list [team-id]",
	Short:   "List the projects of a team",
	Example: "  figctl projects list 123456789\n  figctl projects list",
	Args:    cobra.MaximumNArgs(1),
}

var projectsFilesCmd = &cobra.Command{
	Use:     "files <project-id>",
	Short:   "List the files in a project",
	Example: "  figctl projects files 1001",
	Args:    cobra.ExactArgs(1),
}

func init() {
	projectsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		teamID, err := teamIDArg(session, args)
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetTeamProjects(rctx, teamID)
		if err != nil {
			return err
		}
		list := projectList{}
		for _, p := range resp.Projects {
			list = append(list, projectRow{ID: p.ID, Name: p.Name})
		}
		env := ctx.Envelope(list)
		env.AddHint("Team " + resp.Name + ". The projects API is deprecated; prefer figctl folders list.")
		return ctx.Printer.Print(env)
	})

	var branches bool
	projectsFilesCmd.Flags().BoolVar(&branches, "branches", false, "include branch data for each file")
	projectsFilesCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetProjectFiles(rctx, args[0], branches)
		if err != nil {
			return err
		}
		env := ctx.Envelope(fileListFrom(resp.Files))
		env.AddHint("Project " + resp.Name + ".")
		return ctx.Printer.Print(env)
	})

	projectsCmd.AddCommand(projectsListCmd, projectsFilesCmd)
	rootCmd.AddCommand(projectsCmd)
}
