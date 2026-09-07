package cli

import (
	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/output"
)

type meInfo struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Email  string `json:"email"`
	ImgURL string `json:"imgUrl,omitempty"`
}

func (m meInfo) Columns() []string { return []string{"field", "value"} }

func (m meInfo) Rows() [][]string {
	kv := output.KeyValues{{"id", m.ID}, {"handle", m.Handle}, {"email", m.Email}}
	if m.ImgURL != "" {
		kv = append(kv, [2]string{"imgUrl", m.ImgURL})
	}
	return kv.Rows()
}

var meCmd = &cobra.Command{
	Use:     "me",
	Short:   "Show the user the active token belongs to",
	Example: "  figctl me\n  figctl me --profile acme --json",
	Args:    cobra.NoArgs,
}

func init() {
	meCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		rctx, cancel := session.Context()
		defer cancel()
		me, err := session.Client.GetMe(rctx)
		if err != nil {
			return err
		}
		return ctx.Print(meInfo{ID: me.ID, Handle: me.Handle, Email: me.Email, ImgURL: me.ImgURL})
	})
	rootCmd.AddCommand(meCmd)
}
