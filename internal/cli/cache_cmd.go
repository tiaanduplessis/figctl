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
	EntryCount    int              `json:"entryCount"`
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
	Details  bool               `json:"details"`
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
			rows = append(rows, []string{p.Name, f.Key, strconv.Itoa(f.EntryCount), strconv.Itoa(f.Blobs), cache.FormatBytes(f.Bytes), f.Version, f.CheckedAge})
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
	Use:   "status",
	Short: "Show a compact cache summary per profile and file",
	Long: `Show file summaries without dumping individual cache entries. Use --details
to page through entries, and --file and --profile to narrow the inventory.
--limit and --cursor page files by default, or entries with --details.
Bytes and entryCount describe the full matching inventory, not just the page.`,
	Example: "  figctl cache status\n  figctl cache status --details --file KEY --limit 20",
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
	var details bool
	var statusFile string
	cacheStatusCmd.Flags().BoolVar(&details, "details", false, "include individual entries (paged with --limit and --cursor)")
	cacheStatusCmd.Flags().StringVar(&statusFile, "file", "", "only this file key or Figma URL")
	cacheStatusCmd.RunE = run(func(ctx *Context, _ *cobra.Command, _ []string) error {
		root, err := cache.Dir()
		if err != nil {
			return err
		}
		status, err := cache.Scan(root)
		if err != nil {
			return err
		}
		fileKey := ""
		if statusFile != "" {
			r, err := ref.Parse(statusFile)
			if err != nil {
				return err
			}
			fileKey = r.FileKey
		}
		type record struct {
			profile string
			file    cache.FileStatus
			entry   *cache.EntryInfo
		}
		var records []record
		var total int64
		profileBytes := map[string]int64{}
		for _, p := range status.Profiles {
			if ctx.Flags.Profile != "" && p.Name != ctx.Flags.Profile {
				continue
			}
			for _, f := range p.Files {
				if fileKey != "" && f.Key != fileKey {
					continue
				}
				total += f.Bytes
				profileBytes[p.Name] += f.Bytes
				if details && len(f.Entries) > 0 {
					for _, e := range f.Entries {
						records = append(records, record{p.Name, f, &e})
					}
				} else {
					records = append(records, record{p.Name, f, nil})
				}
			}
		}
		page, next, err := paginate(records, ctx.Flags.Limit, ctx.Flags.Cursor)
		if err != nil {
			return err
		}
		now := time.Now()
		view := cacheStatusView{Root: root, Bytes: total, Size: cache.FormatBytes(total), Details: details, Profiles: []cacheProfileView{}}
		for _, row := range page {
			if len(view.Profiles) == 0 || view.Profiles[len(view.Profiles)-1].Name != row.profile {
				view.Profiles = append(view.Profiles, cacheProfileView{Name: row.profile, Bytes: profileBytes[row.profile], Files: []cacheFileView{}})
			}
			pv := &view.Profiles[len(view.Profiles)-1]
			f := row.file
			if len(pv.Files) == 0 || pv.Files[len(pv.Files)-1].Key != f.Key {
				fv := cacheFileView{Key: f.Key, Bytes: f.Bytes, Entries: []cacheEntryView{}, EntryCount: len(f.Entries), Blobs: f.Blobs, Version: f.Version, LastTouchedAt: f.LastTouchedAt, TooLarge: f.TooLarge}
				if f.CheckedAt != nil {
					fv.CheckedAge = age(*f.CheckedAt, now)
				}
				pv.Files = append(pv.Files, fv)
			}
			if e := row.entry; e != nil {
				fv := &pv.Files[len(pv.Files)-1]
				fv.Entries = append(fv.Entries, cacheEntryView{Key: e.Key, Bytes: e.Bytes, Age: age(e.FetchedAt, now), Version: e.Version})
			}
		}
		env := ctx.Envelope(view)
		env.NextCursor, env.Truncated = next, next != nil
		if total == 0 {
			env.AddHint("No cached files match these filters.")
		} else if !details {
			env.AddHint("Summary only: entryCount includes all entries; use --details --file KEY to read a bounded page of entries.")
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
