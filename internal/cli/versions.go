package cli

import (
	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figma"
)

const defaultVersionsLimit = 30

type versionRow struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"createdAt"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	User        string `json:"user"`
}

type versionList []versionRow

func (l versionList) Columns() []string {
	return []string{"id", "createdAt", "label", "user", "description"}
}

func (l versionList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, v := range l {
		rows = append(rows, []string{v.ID, v.CreatedAt, v.Label, v.User, v.Description})
	}
	return rows
}

var versionsCmd = &cobra.Command{
	Use:   "versions",
	Short: "List saved versions of a file",
}

var versionsListCmd = &cobra.Command{
	Use:   "list <ref>",
	Short: "List versions, newest first",
	Long: `List the saved versions of a file, newest first. Use a version id with
--file-version on any other command to read the file as it was at that
version. Pass --cursor from nextCursor to page further back.`,
	Example: `  figctl versions list KEY
  figctl versions list KEY --limit 5 --cursor 2100120000`,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	versionsListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		r, err := session.Ref(refArg(args))
		if err != nil {
			return err
		}
		limit := ctx.Flags.Limit
		if limit <= 0 {
			limit = defaultVersionsLimit
		}
		rctx, cancel := session.Context()
		defer cancel()
		resp, err := session.Client.GetVersions(rctx, r.FileKey, figma.PageOptions{PageSize: limit, Before: ctx.Flags.Cursor})
		if err != nil {
			return err
		}
		session.Touch(r.FileKey)
		list := versionList{}
		for _, v := range resp.Versions {
			row := versionRow{ID: v.ID, CreatedAt: v.CreatedAt, User: v.User.Handle}
			if v.Label != nil {
				row.Label = *v.Label
			}
			if v.Description != nil {
				row.Description = *v.Description
			}
			list = append(list, row)
		}
		env := ctx.Envelope(list)
		if next := cursorParam(resp.Pagination.NextPage, "before"); next != "" {
			env.NextCursor = &next
		}
		return ctx.Printer.Print(env)
	})
	versionsCmd.AddCommand(versionsListCmd)
	rootCmd.AddCommand(versionsCmd)
}
