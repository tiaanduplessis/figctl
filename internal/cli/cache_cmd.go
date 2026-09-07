package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/cache"
	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/output"
	"github.com/tiaanduplessis/figctl/internal/ref"
)

type cacheEntryView struct {
	Key     string `json:"key"`
	Bytes   int64  `json:"bytes"`
	Age     string `json:"age"`
	Version string `json:"version,omitempty"`
}

type cacheFileView struct {
	Key           string           `json:"key"`
	Bytes         int64            `json:"bytes"`
	Entries       []cacheEntryView `json:"entries"`
	Blobs         int              `json:"blobs"`
	Version       string           `json:"version,omitempty"`
	LastTouchedAt string           `json:"lastTouchedAt,omitempty"`
	CheckedAge    string           `json:"checkedAge,omitempty"`
	TooLarge      bool             `json:"tooLarge,omitempty"`
}

type cacheProfileView struct {
	Name  string          `json:"name"`
	Bytes int64           `json:"bytes"`
	Files []cacheFileView `json:"files"`
}

type cacheStatusView struct {
	Root     string             `json:"root"`
	Bytes    int64              `json:"bytes"`
	Size     string             `json:"size"`
	Profiles []cacheProfileView `json:"profiles"`
}

func (v cacheStatusView) Columns() []string {
	return []string{"profile", "file", "entries", "blobs", "size", "version", "checked"}
}

func (v cacheStatusView) Rows() [][]string {
	var rows [][]string
	for _, p := range v.Profiles {
		if len(p.Files) == 0 {
			rows = append(rows, []string{p.Name, "", "0", "0", cache.FormatBytes(p.Bytes), "", ""})
		}
		for _, f := range p.Files {
			rows = append(rows, []string{p.Name, f.Key, strconv.Itoa(len(f.Entries)), strconv.Itoa(f.Blobs), cache.FormatBytes(f.Bytes), f.Version, f.CheckedAge})
		}
	}
	return rows
}

type cacheClearResult struct {
	Root         string `json:"root"`
	Profile      string `json:"profile,omitempty"`
	File         string `json:"file,omitempty"`
	RemovedBytes int64  `json:"removedBytes"`
	Removed      string `json:"removed"`
}

func (r cacheClearResult) Columns() []string { return []string{"field", "value"} }

func (r cacheClearResult) Rows() [][]string {
	kv := output.KeyValues{{"root", r.Root}}
	if r.Profile != "" {
		kv = append(kv, [2]string{"profile", r.Profile})
	}
	if r.File != "" {
		kv = append(kv, [2]string{"file", r.File})
	}
	kv = append(kv, [2]string{"removed", r.Removed})
	return kv.Rows()
}

func age(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect and clear the on-disk response cache",
}

var cacheStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show cache size and entries per profile and file",
	Example: "  figctl cache status\n  figctl cache status --json",
	Args:    cobra.NoArgs,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete cached data for every profile, one profile, or one file",
	Long: `Delete cached data. Without flags every profile's cache is removed. Use
--profile to clear one account and --file to clear one file. Requires
confirmation on a terminal and --yes otherwise.`,
	Example: `  figctl cache clear --yes
  figctl cache clear --profile acme --file KEY --yes`,
	Args: cobra.NoArgs,
}

func init() {
	cacheStatusCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		root, err := cache.Dir()
		if err != nil {
			return err
		}
		status, err := cache.Scan(root)
		if err != nil {
			return err
		}
		now := time.Now()
		view := cacheStatusView{Root: root, Bytes: status.Bytes, Size: cache.FormatBytes(status.Bytes), Profiles: []cacheProfileView{}}
		for _, p := range status.Profiles {
			pv := cacheProfileView{Name: p.Name, Bytes: p.Bytes, Files: []cacheFileView{}}
			for _, f := range p.Files {
				fv := cacheFileView{Key: f.Key, Bytes: f.Bytes, Entries: []cacheEntryView{}, Blobs: f.Blobs, Version: f.Version, LastTouchedAt: f.LastTouchedAt, TooLarge: f.TooLarge}
				if f.CheckedAt != nil {
					fv.CheckedAge = age(*f.CheckedAt, now)
				}
				for _, e := range f.Entries {
					fv.Entries = append(fv.Entries, cacheEntryView{Key: e.Key, Bytes: e.Bytes, Age: age(e.FetchedAt, now), Version: e.Version})
				}
				pv.Files = append(pv.Files, fv)
			}
			view.Profiles = append(view.Profiles, pv)
		}
		env := ctx.Envelope(view)
		if len(view.Profiles) == 0 {
			env.AddHint("The cache is empty; it fills as you run file commands.")
		}
		return ctx.Printer.Print(env)
	})

	var file string
	cacheClearCmd.Flags().StringVar(&file, "file", "", "only clear this file key or Figma URL")
	cacheClearCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		root, err := cache.Dir()
		if err != nil {
			return err
		}
		fileKey := ""
		if file != "" {
			r, err := ref.Parse(file)
			if err != nil {
				return err
			}
			fileKey = r.FileKey
		}
		what := "the whole cache at " + root
		switch {
		case ctx.Flags.Profile != "" && fileKey != "":
			what = "cached data of file " + fileKey + " for profile " + ctx.Flags.Profile
		case ctx.Flags.Profile != "":
			what = "cached data for profile " + ctx.Flags.Profile
		case fileKey != "":
			what = "cached data of file " + fileKey + " for every profile"
		}
		if err := confirm(ctx, "Delete "+what+"?"); err != nil {
			return err
		}
		removed, err := cache.Clear(root, ctx.Flags.Profile, fileKey)
		if err != nil {
			return err
		}
		return ctx.Print(cacheClearResult{Root: root, Profile: ctx.Flags.Profile, File: fileKey, RemovedBytes: removed, Removed: cache.FormatBytes(removed)})
	})

	cacheCmd.AddCommand(cacheStatusCmd, cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}

func usageErr(message, hint string) error {
	return figctl.New(figctl.CodeUsage, message).WithHint("%s", hint)
}
