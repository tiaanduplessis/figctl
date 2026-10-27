package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/imagediff"
)

type diffData struct {
	imagediff.Result
	Reference *diffReference `json:"reference,omitempty"`
}

func (d diffData) Columns() []string {
	return []string{"passed", "dimensions", "mismatchedPixels", "totalPixels", "mismatchRatio", "diffPath"}
}

func (d diffData) Rows() [][]string {
	return [][]string{{strconv.FormatBool(d.Passed), fmt.Sprintf("%dx%d", d.Width, d.Height),
		strconv.Itoa(d.MismatchedPixels), strconv.Itoa(d.TotalPixels), strconv.FormatFloat(d.MismatchRatio, 'g', -1, 64), d.DiffPath}}
}

var diffCmd = &cobra.Command{
	Use:   "diff [expected.png] <actual.png>",
	Short: "Compare local PNG screenshots and optionally write a visual diff",
	Long: `Compare two local PNGs without credentials or network requests.
Use --manifest render.json (or a node context JSON envelope) to select the
expected PNG without guessing its filename. Pass --node for a multi-node
manifest; only the actual PNG argument is needed in this mode. Comparison
results retain the reference file version, node, and capture geometry.

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
  figctl diff before.png after.png --threshold 0 --include-aa
  figctl diff --manifest render.json --node 2:2 implementation.png --out diff.png`,
}

func init() {
	var opts imagediff.Options
	var manifest, nodeID string
	f := diffCmd.Flags()
	f.StringVar(&manifest, "manifest", "", "JSON envelope from render or node context supplying the expected PNG")
	f.StringVar(&nodeID, "node", "", "node id to select from --manifest")
	diffCmd.Args = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("manifest") {
			return cobra.ExactArgs(1)(cmd, args)
		}
		return cobra.ExactArgs(2)(cmd, args)
	}
	f.StringVar(&opts.Out, "out", "", "destination PNG path (optional)")
	f.Float64Var(&opts.Threshold, "threshold", 0.1, "per-pixel color tolerance between 0 and 1")
	f.Float64Var(&opts.MaxDiffRatio, "max-diff-ratio", 0, "allowed mismatch fraction between 0 and 1 (0.01 = 1%)")
	f.BoolVar(&opts.IncludeAA, "include-aa", false, "count detected anti-aliasing differences")
	diffCmd.RunE = run(func(ctx *Context, _ *cobra.Command, args []string) error {
		if f.Changed("out") && opts.Out == "" {
			return figctl.New(figctl.CodeUsage, "--out must name a PNG file")
		}
		expected, actual := args[0], args[len(args)-1]
		var reference *diffReference
		if f.Changed("manifest") {
			if manifest == "" {
				return usageErr("--manifest must name a JSON file", "Save stdout from render --json or node context --json.")
			}
			if nodeID != "" {
				var err error
				nodeID, err = parseNodeArg(nodeID)
				if err != nil {
					return err
				}
			}
			var err error
			expected, reference, err = readDiffReference(manifest, nodeID)
			if err != nil {
				return err
			}
		} else if f.Changed("node") {
			return usageErr("--node needs --manifest", "Use two PNG paths, or --manifest render.json --node <id> actual.png.")
		}
		if reference != nil && opts.Out != "" {
			sourceInfo, err := os.Stat(reference.Manifest)
			if err != nil {
				return figctl.Wrap(figctl.CodeImageIO, err, "checking manifest input: "+err.Error())
			}
			if destinationInfo, err := os.Stat(opts.Out); err == nil && os.SameFile(sourceInfo, destinationInfo) {
				return usageErr("diff output must not overwrite the manifest input", "Choose a separate --out PNG path.")
			}
		}
		result, err := imagediff.Compare(expected, actual, opts)
		if err != nil {
			return err
		}
		env := ctx.Envelope(diffData{Result: *result, Reference: reference})
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
