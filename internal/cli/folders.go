package cli

import (
	"github.com/spf13/cobra"
)

type folderRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
}

type folderList []folderRow

func (l folderList) Columns() []string { return []string{"id", "name", "parentFolderId"} }

func (l folderList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, f := range l {
		rows = append(rows, []string{f.ID, f.Name, f.ParentFolderID})
	}
	return rows
}

var foldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "Discover folders and files (v2 API)",
}

var foldersListCmd = &cobra.Command{
	Use:   "list [team-id|folder-id]",
	Short: "List the top level folders of a team, or the subfolders of a folder",
	Long: `List folders. By default the id is a team id and the team's top level
folders are listed; with --folder the id is a folder id and its subfolders
are listed. Without an id the profile's default team is used.`,
	Example: `  figctl folders list 123456789
  figctl folders list 2001 --folder`,
	Args: cobra.MaximumNArgs(1),
}

var foldersFilesCmd = &cobra.Command{
	Use:     "files <folder-id>",
	Short:   "List the files in a folder",
	Example: "  figctl folders files 2001\n  figctl folders files 2001 --branches",
	Args:    cobra.ExactArgs(1),
}

func init() {
	var isFolder bool
	foldersListCmd.Flags().BoolVar(&isFolder, "folder", false, "treat the id as a folder id and list its subfolders")
	foldersListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		var name string
		list := folderList{}
		if isFolder {
			if len(args) != 1 {
				return usageErr("folder id required with --folder", "Run figctl folders list <folder-id> --folder.")
			}
			resp, err := session.Client.GetFolderFolders(rctx, args[0])
			if err != nil {
				return err
			}
			name = resp.Name
			for _, f := range resp.Folders {
				list = append(list, folderRowFrom(f.ID, f.Name, f.ParentFolderID))
			}
		} else {
			teamID, err := teamIDArg(session, args)
			if err != nil {
				return err
			}
			resp, err := session.Client.GetTeamFolders(rctx, teamID)
			if err != nil {
				return err
			}
			name = resp.Name
			for _, f := range resp.Folders {
				list = append(list, folderRowFrom(f.ID, f.Name, f.ParentFolderID))
			}
		}
		env := ctx.Envelope(list)
		env.AddHint("Contents of " + name + ". Use figctl folders files <folder-id> to list files, or figctl folders list <folder-id> --folder for subfolders.")
		return ctx.Printer.Print(env)
	})

	var branches bool
	foldersFilesCmd.Flags().BoolVar(&branches, "branches", false, "include branch data for each file")
	foldersFilesCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetFolderFiles(rctx, args[0], branches)
		if err != nil {
			return err
		}
		env := ctx.Envelope(fileListFrom(resp.Files))
		env.AddHint("Folder " + resp.Name + ".")
		return ctx.Printer.Print(env)
	})

	foldersCmd.AddCommand(foldersListCmd, foldersFilesCmd)
	rootCmd.AddCommand(foldersCmd)
}

func folderRowFrom(id, name string, parent *string) folderRow {
	row := folderRow{ID: id, Name: name}
	if parent != nil {
		row.ParentFolderID = *parent
	}
	return row
}
