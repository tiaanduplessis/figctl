package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/config"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// profileView is the redacted, output-ready view of a profile.
type profileView struct {
	Name          string        `json:"name"`
	Default       bool          `json:"default"`
	Source        config.Source `json:"source,omitempty"`
	Handle        string        `json:"handle,omitempty"`
	Email         string        `json:"email,omitempty"`
	TeamID        string        `json:"teamId,omitempty"`
	BaseURL       string        `json:"baseUrl,omitempty"`
	Expires       string        `json:"expires,omitempty"`
	ExpiresInDays *int          `json:"expiresInDays,omitempty"`
	TokenStore    string        `json:"tokenStore,omitempty"`
}

func (v profileView) Columns() []string { return []string{"field", "value"} }

func (v profileView) Rows() [][]string {
	kv := output.KeyValues{
		{"name", v.Name},
		{"default", strconv.FormatBool(v.Default)},
	}
	optional := [][2]string{
		{"source", string(v.Source)},
		{"handle", v.Handle},
		{"email", v.Email},
		{"teamId", v.TeamID},
		{"baseUrl", v.BaseURL},
		{"expires", v.Expires},
		{"tokenStore", v.TokenStore},
	}
	for _, pair := range optional {
		if pair[1] != "" {
			kv = append(kv, pair)
		}
	}
	if v.ExpiresInDays != nil {
		kv = append(kv, [2]string{"expiresInDays", strconv.Itoa(*v.ExpiresInDays)})
	}
	return kv.Rows()
}

// profileList renders profiles as one row each.
type profileList []profileView

func (l profileList) Columns() []string {
	return []string{"name", "default", "handle", "email", "teamId", "baseUrl", "expires"}
}

func (l profileList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, v := range l {
		def := ""
		if v.Default {
			def = "*"
		}
		rows = append(rows, []string{v.Name, def, v.Handle, v.Email, v.TeamID, v.BaseURL, v.Expires})
	}
	return rows
}

type removeResult struct {
	Name    string `json:"name"`
	Removed bool   `json:"removed"`
}

func (r removeResult) Columns() []string { return []string{"field", "value"} }

func (r removeResult) Rows() [][]string {
	return output.KeyValues{{"name", r.Name}, {"removed", strconv.FormatBool(r.Removed)}}.Rows()
}

func newProfileView(cfg *config.Config, name string, now time.Time) profileView {
	p := cfg.Profiles[name]
	v := profileView{
		Name:    name,
		Default: cfg.DefaultProfile == name,
		Handle:  p.Handle,
		Email:   p.Email,
		TeamID:  p.TeamID,
		BaseURL: p.BaseURL,
		Expires: p.Expires,
	}
	if days, ok := p.DaysUntilExpiry(now); ok {
		v.ExpiresInDays = &days
	}
	return v
}

// expiryHint returns a warning when the recorded token expiry is within
// config.ExpiryWarning, or has passed.
func expiryHint(v profileView) string {
	if v.ExpiresInDays == nil {
		return ""
	}
	days := *v.ExpiresInDays
	switch {
	case days < 0:
		return fmt.Sprintf("Token for profile %q expired on %s; create a new token and run figctl auth login --profile %s.", v.Name, v.Expires, v.Name)
	case time.Duration(days)*24*time.Hour <= config.ExpiryWarning:
		return fmt.Sprintf("Token for profile %q expires in %d day(s) (%s); rotate it soon.", v.Name, days, v.Expires)
	}
	return ""
}

func requireProfile(cfg *config.Config, name string) error {
	if _, ok := cfg.Profiles[name]; ok {
		return nil
	}
	return figctl.Newf(figctl.CodeNotFound, "profile %q does not exist", name).
		WithHint("Run figctl profile list to see configured profiles, or figctl auth login --profile %s to add it.", name)
}

// addOptions are the flags shared by profile add and auth login.
type addOptions struct {
	expires     string
	team        string
	baseURL     string
	makeDefault bool
}

func addProfileFlags(cmd *cobra.Command, o *addOptions) {
	cmd.Flags().StringVar(&o.expires, "expires", "", "token expiry date as YYYY-MM-DD (Figma tokens expire after at most 90 days)")
	cmd.Flags().StringVar(&o.team, "team", "", "default team id for this profile")
	cmd.Flags().StringVar(&o.baseURL, "base-url", "", "API base URL, for example https://api-figma-gov.com")
	cmd.Flags().BoolVar(&o.makeDefault, "default", false, "make this the default profile")
}

// addProfile stores a token and the profile settings. It is shared by
// profile add and auth login.
func addProfile(ctx *Context, name string, o addOptions) error {
	if err := config.ValidateName(name); err != nil {
		return err
	}
	if err := config.ValidateExpires(o.expires); err != nil {
		return err
	}
	if o.baseURL != "" && !strings.HasPrefix(o.baseURL, "https://") {
		return figctl.Newf(figctl.CodeUsage, "invalid --base-url %q", o.baseURL).
			WithHint("The base URL must start with https://, for example https://api-figma-gov.com.")
	}
	token, err := readToken(ctx)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	p := cfg.Profiles[name]
	if o.team != "" {
		p.TeamID = o.team
	}
	if o.baseURL != "" {
		p.BaseURL = o.baseURL
	}
	p.Expires = o.expires
	me, err := validateToken(ctx, token, p.BaseURL)
	if err != nil {
		return err
	}
	p.Handle = me.Handle
	p.Email = me.Email
	store, err := ctx.Store()
	if err != nil {
		return err
	}
	if err := store.Set(name, token); err != nil {
		return figctl.Wrap(figctl.CodeInternal, err, "storing token: "+err.Error())
	}
	cfg.Profiles[name] = p
	if o.makeDefault || cfg.DefaultProfile == "" {
		cfg.DefaultProfile = name
	}
	if err := cfg.Save(); err != nil {
		return err
	}

	view := newProfileView(cfg, name, time.Now())
	view.TokenStore = store.Kind()
	ctx.SetProfile(&config.Active{Name: name, Profile: p})
	env := ctx.Envelope(view)
	env.AddHint(fmt.Sprintf("Token validated with the Figma API as %s (%s).", me.Handle, me.Email))
	if o.expires == "" {
		env.AddHint("Record the token expiry with --expires YYYY-MM-DD to get a warning before it expires.")
	}
	env.AddHint("Run figctl init to pin this profile in the current repository.")
	return ctx.Printer.Print(env)
}

// validateToken checks a token with GET me before it is stored. Figma
// rejections become AUTH_INVALID so nothing is written for a bad token.
func validateToken(ctx *Context, token, baseURL string) (*figma.Me, error) {
	client := ctx.newClient(token, baseURL, nil)
	rctx, cancel := context.WithTimeout(context.Background(), ctx.Flags.Timeout+10*time.Second)
	defer cancel()
	me, err := client.GetMe(rctx)
	if err != nil {
		e := figctl.From(err)
		switch e.Code {
		case figctl.CodeAuthInvalid, figctl.CodeAuthScope, figctl.CodeForbidden, figctl.CodeNotFound, figctl.CodeUsage:
			return nil, figctl.Wrap(figctl.CodeAuthInvalid, err, "Figma rejected the token: "+e.Message).
				WithHint("Nothing was stored. Create a personal access token in Figma settings with the scopes from figctl auth scopes (at least current_user:read) and try again.")
		}
		return nil, err
	}
	return me, nil
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named accounts (one per client or organization)",
}

var profileAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add or update a profile and store its token",
	Long: `Add or update a profile. The token is read from --token-file, from stdin
when stdin is not a terminal, or from a hidden prompt on a terminal. It is
never accepted as a flag value.`,
	Example: `  figctl profile add acme --expires 2026-12-01
  printf '%s' "$FIGMA_TOKEN" | figctl profile add acme --team 123456 --default
  figctl profile add gov --base-url https://api-figma-gov.com --token-file ./token.txt`,
	Args: cobra.ExactArgs(1),
}

var profileListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List profiles",
	Example: "  figctl profile list\n  figctl profile list --json --fields name,expires",
	Args:    cobra.NoArgs,
}

var profileUseCmd = &cobra.Command{
	Use:     "use <name>",
	Short:   "Set the default profile",
	Example: "  figctl profile use acme",
	Args:    cobra.ExactArgs(1),
}

var profileShowCmd = &cobra.Command{
	Use:     "show [name]",
	Short:   "Show a profile (the active one when no name is given), token redacted",
	Example: "  figctl profile show\n  figctl profile show acme",
	Args:    cobra.MaximumNArgs(1),
}

var profileRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove a profile and its stored token",
	Example: "  figctl profile remove acme --yes",
	Args:    cobra.ExactArgs(1),
}

func init() {
	var add addOptions
	addProfileFlags(profileAddCmd, &add)
	profileAddCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		return addProfile(ctx, args[0], add)
	})

	profileListCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		now := time.Now()
		list := profileList{}
		var hints []string
		for _, name := range cfg.Names() {
			v := newProfileView(cfg, name, now)
			list = append(list, v)
			if hint := expiryHint(v); hint != "" {
				hints = append(hints, hint)
			}
		}
		env := ctx.Envelope(list)
		env.Hints = append(env.Hints, hints...)
		if len(list) == 0 {
			env.AddHint("No profiles yet. Run figctl auth login --profile <name>.")
		}
		return ctx.Printer.Print(env)
	})

	profileUseCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := requireProfile(cfg, args[0]); err != nil {
			return err
		}
		cfg.DefaultProfile = args[0]
		if err := cfg.Save(); err != nil {
			return err
		}
		return ctx.Print(newProfileView(cfg, args[0], time.Now()))
	})

	profileShowCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		var view profileView
		if len(args) == 1 {
			if err := requireProfile(cfg, args[0]); err != nil {
				return err
			}
			view = newProfileView(cfg, args[0], time.Now())
		} else {
			opts, err := ctx.ResolveOptions(false)
			if err != nil {
				return err
			}
			active, err := config.Select(cfg, opts)
			if err != nil {
				return err
			}
			view = newProfileView(cfg, active.Name, time.Now())
			view.Source = active.Source
		}
		env := ctx.Envelope(view)
		if hint := expiryHint(view); hint != "" {
			env.AddHint(hint)
		}
		return ctx.Printer.Print(env)
	})

	profileRemoveCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		name := args[0]
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := requireProfile(cfg, name); err != nil {
			return err
		}
		if err := confirm(ctx, fmt.Sprintf("Remove profile %q and its stored token?", name)); err != nil {
			return err
		}
		store, err := ctx.Store()
		if err != nil {
			return err
		}
		if err := store.Delete(name); err != nil && !errors.Is(err, config.ErrNotFound) {
			return figctl.Wrap(figctl.CodeInternal, err, "removing token: "+err.Error())
		}
		delete(cfg.Profiles, name)
		if cfg.DefaultProfile == name {
			cfg.DefaultProfile = ""
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		env := ctx.Envelope(removeResult{Name: name, Removed: true})
		if cfg.DefaultProfile == "" && len(cfg.Profiles) > 0 {
			env.AddHint("No default profile is set; run figctl profile use <name>.")
		}
		return ctx.Printer.Print(env)
	})

	profileCmd.AddCommand(profileAddCmd, profileListCmd, profileUseCmd, profileShowCmd, profileRemoveCmd)
	rootCmd.AddCommand(profileCmd)
}
