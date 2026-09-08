package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/imagediff"
)

type diffData struct{ imagediff.Result }

func (d diffData) Columns() []string {
	return []string{"passed", "dimensions", "mismatchedPixels", "totalPixels", "mismatchRatio", "diffPath"}
}

func (d diffData) Rows() [][]string {
	return [][]string{{strconv.FormatBool(d.Passed), fmt.Sprintf("%dx%d", d.Width, d.Height),
		strconv.Itoa(d.MismatchedPixels), strconv.Itoa(d.TotalPixels), strconv.FormatFloat(d.MismatchRatio, 'g', -1, 64), d.DiffPath}}
}

var diffCmd = &cobra.Command{
	Use:   "diff <expected.png> <actual.png>",
	Short: "Compare local PNG screenshots and optionally write a visual diff",
	Long: `Compare two local PNGs without credentials or network requests.
Images must have equal pixel dimensions; no resizing or cropping is applied.
Each input is limited to 16 million pixels. Match viewport, crop, fonts, content,
and capture scale. Render defaults to 2x; use --scale 1 for a 1x screenshot.

--threshold controls per-pixel color tolerance (0 is most sensitive).
--max-diff-ratio is the accepted fraction of mismatched pixels (0.01 = 1%).
Detected anti-aliasing differences are ignored unless --include-aa is set.
Transparency is compared over a checkerboard. The ratio measures pixel
mismatch, not design quality.

Exit 0 when mismatchRatio <= maxDiffRatio; exit 7 otherwise. Both return the
same data envelope, including passed. Invalid inputs exit 2, missing paths
exit 4, and other file I/O failures exit 1. --out writes a PNG on pass or fail,
replacing an existing output but never an input. Its parent directory must exist.
Red marks mismatches, yellow marks ignored anti-aliasing, and unchanged areas
are faded. Without --out, only the comparison result is produced.`,
	Example: `  figctl diff design.png implementation.png --out diff.png --max-diff-ratio 0.01 --json
  figctl diff before.png after.png --threshold 0 --include-aa`,
	Args: cobra.ExactArgs(2),
}

func init() {
	var opts imagediff.Options
	f := diffCmd.Flags()
	f.StringVar(&opts.Out, "out", "", "destination PNG path (optional)")
	f.Float64Var(&opts.Threshold, "threshold", 0.1, "per-pixel color tolerance between 0 and 1")
	f.Float64Var(&opts.MaxDiffRatio, "max-diff-ratio", 0, "allowed mismatch fraction between 0 and 1 (0.01 = 1%)")
	f.BoolVar(&opts.IncludeAA, "include-aa", false, "count detected anti-aliasing differences")
	diffCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if f.Changed("out") && opts.Out == "" {
			return figctl.New(figctl.CodeUsage, "--out must name a PNG file")
		}
		result, err := imagediff.Compare(args[0], args[1], opts)
		if err != nil {
			return err
		}
		env := ctx.Envelope(diffData{Result: *result})
		if result.DiffPath != "" {
			env.AddHint("Read the visual diff with your image/vision tool: " + result.DiffPath)
		}
		if !result.Passed {
			return ctx.PrintPartial(env, figctl.Newf(figctl.CodeImageMismatch, "image mismatch ratio %g exceeds the allowed %g", result.MismatchRatio, result.MaxDiffRatio))
		}
		return ctx.Printer.Print(env)
	})
	rootCmd.AddCommand(diffCmd)
}
