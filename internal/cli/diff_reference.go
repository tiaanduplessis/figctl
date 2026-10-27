package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/figma"
	"github.com/tiaanduplessis/figctl/internal/output"
)

// diffReference identifies the exact design capture used by an offline comparison.
type diffReference struct {
	Manifest    string           `json:"manifest"`
	NodeID      string           `json:"nodeId"`
	File        *output.FileInfo `json:"file,omitempty"`
	Scale       float64          `json:"scale"`
	PixelWidth  int              `json:"pixelWidth,omitempty"`
	PixelHeight int              `json:"pixelHeight,omitempty"`
	Bounds      *figma.Rectangle `json:"bounds,omitempty"`
	Background  *figma.Color     `json:"background,omitempty"`
}

func readDiffReference(path, nodeID string) (string, *diffReference, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // the manifest is explicitly selected by the caller
	if err != nil {
		code := figctl.CodeImageIO
		if errors.Is(err, os.ErrNotExist) {
			code = figctl.CodeNotFound
		}
		return "", nil, figctl.Wrap(code, err, "reading render manifest: "+err.Error())
	}
	var env struct {
		Command string           `json:"command"`
		File    *output.FileInfo `json:"file"`
		Data    json.RawMessage  `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", nil, figctl.Wrap(figctl.CodeUsage, err, "invalid manifest JSON: "+err.Error()).WithHint("Save stdout from figctl render --json or node context --json.")
	}
	type capture struct {
		path string
		ref  diffReference
	}
	var captures []capture
	manifest, err := filepath.Abs(path)
	if err != nil {
		return "", nil, figctl.Wrap(figctl.CodeImageIO, err, "resolving manifest path: "+err.Error())
	}
	switch env.Command {
	case "render":
		var d renderData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return "", nil, figctl.Wrap(figctl.CodeUsage, err, "invalid render manifest: "+err.Error())
		}
		for _, r := range d.Items {
			if r.Format == "png" && r.Error == "" && r.Path != "" {
				captures = append(captures, capture{r.Path, diffReference{Manifest: manifest, NodeID: r.NodeID, File: env.File, Scale: r.Scale, PixelWidth: r.PixelWidth, PixelHeight: r.PixelHeight, Bounds: r.Bounds, Background: r.Background}})
			}
		}
	case "node.context":
		var d contextData
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return "", nil, figctl.Wrap(figctl.CodeUsage, err, "invalid context manifest: "+err.Error())
		}
		for _, n := range d.Nodes {
			if r := n.Screenshot; r != nil && r.Format == "png" && r.Path != "" {
				captures = append(captures, capture{r.Path, diffReference{Manifest: manifest, NodeID: n.ID, File: env.File, Scale: r.Scale, PixelWidth: r.PixelWidth, PixelHeight: r.PixelHeight, Bounds: r.Bounds, Background: r.Background}})
			}
		}
	default:
		return "", nil, usageErr("unsupported manifest command", "Use the JSON envelope from figctl render or figctl node context.")
	}
	var matched []capture
	for _, c := range captures {
		if nodeID == "" || c.ref.NodeID == nodeID {
			matched = append(matched, c)
		}
	}
	if len(matched) != 1 {
		return "", nil, usageErr("manifest must select exactly one successful PNG", "Pass --node <id> for a multi-node manifest. Check that the selected node has a successful PNG capture.")
	}
	return matched[0].path, &matched[0].ref, nil
}
