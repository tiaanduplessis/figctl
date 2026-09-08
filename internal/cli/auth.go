package cli

import (
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

type authStatus struct {
	Profile         string           `json:"profile"`
	Source          config.Source    `json:"source"`
	TokenSource     string           `json:"tokenSource"`
	HasToken        bool             `json:"hasToken"`
	Validated       bool             `json:"validated"`
	ValidationError *validationError `json:"validationError,omitempty"`
	Handle          string           `json:"handle,omitempty"`
	Email           string           `json:"email,omitempty"`
	UserID          string           `json:"userId,omitempty"`
	TeamID          string           `json:"teamId,omitempty"`
	BaseURL         string           `json:"baseUrl,omitempty"`
	Expires         string           `json:"expires,omitempty"`
	ExpiresInDays   *int             `json:"expiresInDays,omitempty"`
	ProjectConfig   string           `json:"projectConfig,omitempty"`
	RateLimit       *figma.RateLimit `json:"rateLimit,omitempty"`
}

type validationError struct {
	Code    figctl.Code `json:"code"`
	Message string      `json:"message"`
}

func (s authStatus) Columns() []string { return []string{"field", "value"} }

func (s authStatus) Rows() [][]string {
	kv := output.KeyValues{
		{"profile", s.Profile},
		{"source", string(s.Source)},
		{"tokenSource", s.TokenSource},
		{"hasToken", strconv.FormatBool(s.HasToken)},
		{"validated", strconv.FormatBool(s.Validated)},
	}
	if s.ValidationError != nil {
		kv = append(kv, [2]string{"validationError", string(s.ValidationError.Code) + ": " + s.ValidationError.Message})
	}
	for _, pair := range [][2]string{
		{"handle", s.Handle},
		{"email", s.Email},
		{"userId", s.UserID},
		{"teamId", s.TeamID},
		{"baseUrl", s.BaseURL},
		{"expires", s.Expires},
		{"projectConfig", s.ProjectConfig},
	} {
		if pair[1] != "" {
			kv = append(kv, pair)
		}
	}
	if s.ExpiresInDays != nil {
		kv = append(kv, [2]string{"expiresInDays", strconv.Itoa(*s.ExpiresInDays)})
	}
	if s.RateLimit != nil {
		kv = append(kv, [2]string{"rateLimit", describeRateLimit(*s.RateLimit)})
	}
	return kv.Rows()
}

func describeRateLimit(rl figma.RateLimit) string {
	var parts []string
	if rl.PlanTier != "" {
		parts = append(parts, "plan "+rl.PlanTier)
	}
	if rl.Type != "" {
		parts = append(parts, "limit "+rl.Type)
	}
	if rl.RetryAfterSeconds > 0 {
		parts = append(parts, fmt.Sprintf("retry after %ds", rl.RetryAfterSeconds))
	}
	if rl.UpgradeLink != "" {
		parts = append(parts, rl.UpgradeLink)
	}
	return strings.Join(parts, ", ")
}

type logoutResult struct {
	Profile   string `json:"profile"`
	LoggedOut bool   `json:"loggedOut"`
}

func (r logoutResult) Columns() []string { return []string{"field", "value"} }

func (r logoutResult) Rows() [][]string {
	return output.KeyValues{{"profile", r.Profile}, {"loggedOut", strconv.FormatBool(r.LoggedOut)}}.Rows()
}

type scopeInfo struct {
	Scope    string `json:"scope"`
	Required bool   `json:"required"`
	UsedBy   string `json:"usedBy"`
}

type scopeList []scopeInfo

func (l scopeList) Columns() []string { return []string{"scope", "required", "usedBy"} }

func (l scopeList) Rows() [][]string {
	rows := make([][]string, 0, len(l))
	for _, s := range l {
		rows = append(rows, []string{s.Scope, strconv.FormatBool(s.Required), s.UsedBy})
	}
	return rows
}

// scopes is the list of token scopes the CLI uses (plan section 2.4).
var scopes = scopeList{
	{"file_content:read", true, "file info/tree/find/get, node inspect/context, render, assets export"},
	{"file_metadata:read", true, "file info, cache validation"},
	{"library_content:read", true, "components list/search, styles list (file)"},
	{"library_assets:read", false, "components get --key, styles get --key"},
	{"team_library_content:read", false, "components list/search, styles list (team)"},
	{"file_variables:read", false, "variables list/get, tokens export/resolve. Enterprise plan only: the scope is not offered on the token screen on other plans"},
	{"file_dev_resources:read", false, "devresources list, node context"},
	{"file_dev_resources:write", false, "devresources add/update/remove"},
	{"file_comments:read", false, "comments list, node context"},
	{"file_comments:write", false, "comments add"},
	{"file_versions:read", false, "versions list, --file-version"},
	{"projects:read", false, "projects list (v1 fallback)"},
	{"folders:read", false, "folders list"},
	{"current_user:read", true, "me, auth status"},
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Log in, log out, check status, and list required scopes",
}

var authLoginCmd = &cobra.Command{
	Use:   "login [name]",
	Short: "Store a token for a profile (same as profile add)",
	Long: `Store a token for a profile. The profile name comes from the argument or
--profile. The token is read from --token-file, from stdin when stdin is not
a terminal, or from a hidden prompt on a terminal.`,
	Example: `  figctl auth login --profile acme --expires 2026-12-01
  printf '%s' "$TOKEN" | figctl auth login acme --default`,
	Args: cobra.MaximumNArgs(1),
}

var authLogoutCmd = &cobra.Command{
	Use:     "logout [name]",
	Short:   "Remove the stored token for a profile, keeping its settings",
	Example: "  figctl auth logout\n  figctl auth logout acme",
	Args:    cobra.MaximumNArgs(1),
}

var authStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show the active profile and how it was selected",
	Example: "  figctl auth status\n  figctl auth status --profile acme --json",
	Args:    cobra.NoArgs,
}

var authScopesCmd = &cobra.Command{
	Use:     "scopes",
	Short:   "List the token scopes figctl uses and which commands need them",
	Example: "  figctl auth scopes",
	Args:    cobra.NoArgs,
}

func profileNameArg(ctx *Context, args []string) (string, bool) {
	if len(args) == 1 {
		return args[0], true
	}
	if ctx.Flags.Profile != "" {
		return ctx.Flags.Profile, true
	}
	return "", false
}

func init() {
	var add addOptions
	addProfileFlags(authLoginCmd, &add)
	authLoginCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		name, ok := profileNameArg(ctx, args)
		if !ok {
			return figctl.New(figctl.CodeUsage, "profile name required").
				WithHint("Run figctl auth login --profile <name> (or figctl auth login <name>).")
		}
		return addProfile(ctx, name, add)
	})

	authLogoutCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		name, ok := profileNameArg(ctx, args)
		if !ok {
			opts, err := ctx.ResolveOptions(false)
			if err != nil {
				return err
			}
			active, err := config.Select(cfg, opts)
			if err != nil {
				return err
			}
			if active.Source == config.SourceTokenEnv {
				return figctl.New(figctl.CodeUsage, "the active token comes from FIGMA_TOKEN").
					WithHint("Unset FIGMA_TOKEN, or name the profile: figctl auth logout <name>.")
			}
			name = active.Name
		}
		if err := requireProfile(cfg, name); err != nil {
			return err
		}
		store, err := ctx.Store()
		if err != nil {
			return err
		}
		if err := store.Delete(name); err != nil && !errors.Is(err, config.ErrNotFound) {
			return figctl.Wrap(figctl.CodeInternal, err, "removing token: "+err.Error())
		}
		env := ctx.Envelope(logoutResult{Profile: name, LoggedOut: true})
		env.AddHint("Profile settings were kept; run figctl profile remove " + name + " to delete them too.")
		return ctx.Printer.Print(env)
	})

	authStatusCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		session, err := ctx.Session()
		if err != nil {
			return err
		}
		active, cfg := session.Active, session.Config
		status := authStatus{
			Profile:     active.Name,
			Source:      active.Source,
			TokenSource: active.TokenSource,
			HasToken:    active.Token != "",
			Handle:      active.Profile.Handle,
			Email:       active.Profile.Email,
			TeamID:      active.Profile.TeamID,
			BaseURL:     active.Profile.BaseURL,
			Expires:     active.Profile.Expires,
		}
		if active.Project != nil {
			status.ProjectConfig = active.Project.Path
		}
		view := newProfileView(cfg, active.Name, time.Now())
		status.ExpiresInDays = view.ExpiresInDays
		env := ctx.Envelope(status)
		rctx, cancel := session.Context()
		defer cancel()
		me, err := session.Client.GetMe(rctx)
		if err != nil {
			e := figctl.From(err)
			status.ValidationError = &validationError{Code: e.Code, Message: e.Message}
			if e.Hint != "" {
				env.AddHint(e.Hint)
			}
		} else {
			status.Validated = true
			status.Handle, status.Email, status.UserID = me.Handle, me.Email, me.ID
			env.Profile = &output.ProfileInfo{Name: active.Name, Handle: me.Handle}
		}
		if rl := session.Client.LastRateLimit(); rl.Seen {
			status.RateLimit = &rl
		}
		env.Data = status
		if hint := expiryHint(view); hint != "" {
			env.AddHint(hint)
		}
		return ctx.Printer.Print(env)
	})

	authScopesCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		env := ctx.Envelope(scopes)
		env.AddHint("Select these scopes when creating a personal access token in Figma settings. The legacy files:read scope is deprecated.")
		return ctx.Printer.Print(env)
	})

	authCmd.AddCommand(authLoginCmd, authLogoutCmd, authStatusCmd, authScopesCmd)
	rootCmd.AddCommand(authCmd)
}
