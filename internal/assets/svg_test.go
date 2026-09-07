package assets

import (
	"reflect"
	"strings"
	"testing"
)

const sampleSVG = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
<g id="Icon" clip-path="url(#clip0_1_2)">
<path id="Vector" d="M4 11h12" fill="#111827" stroke="#111827" stroke-width="2"/>
<rect id="Frame" width="24" height="24" fill="none"/>
</g>
<defs>
<clipPath id="clip0_1_2"><rect width="24" height="24" fill="white"/></clipPath>
</defs>
</svg>`

func TestCleanSVGStripDimensions(t *testing.T) {
	out := string(CleanSVG([]byte(sampleSVG), SVGCleanOptions{StripDimensions: true}))
	if !strings.HasPrefix(out, `<svg viewBox="0 0 24 24" fill="none"`) {
		t.Fatalf("root dimensions kept:\n%s", out)
	}
	if !strings.Contains(out, `<rect id="Frame" width="24" height="24"`) {
		t.Fatal("child dimensions must be untouched")
	}
	noViewBox := `<svg width="24" height="24" xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`
	if got := string(CleanSVG([]byte(noViewBox), SVGCleanOptions{StripDimensions: true})); got != noViewBox {
		t.Fatalf("dimensions removed without a viewBox:\n%s", got)
	}
}

func TestCleanSVGStripIDs(t *testing.T) {
	out := string(CleanSVG([]byte(sampleSVG), SVGCleanOptions{StripIDs: true}))
	for _, gone := range []string{`id="Icon"`, `id="Vector"`, `id="Frame"`} {
		if strings.Contains(out, gone) {
			t.Fatalf("%s kept:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, `<clipPath id="clip0_1_2">`) || !strings.Contains(out, `clip-path="url(#clip0_1_2)"`) {
		t.Fatalf("referenced id must survive:\n%s", out)
	}
	if !strings.Contains(out, `<g clip-path=`) {
		t.Fatalf("attribute spacing broken:\n%s", out)
	}
}

func TestCleanSVGCurrentColor(t *testing.T) {
	if got := SVGColors([]byte(sampleSVG)); !reflect.DeepEqual(got, []string{"#111827", "white"}) {
		t.Fatalf("colors = %v", got)
	}
	// Two colors (the clipPath rect is white): nothing changes.
	if got := string(CleanSVG([]byte(sampleSVG), SVGCleanOptions{CurrentColor: true})); got != sampleSVG {
		t.Fatalf("multi color icon must be untouched:\n%s", got)
	}
	single := `<svg viewBox="0 0 24 24" fill="none"><path fill="#111827" fill-rule="evenodd" d="M0 0"/><path stroke="#111827" fill="none" d="M1 1"/><path fill="url(#g)" d="M2 2"/></svg>`
	out := string(CleanSVG([]byte(single), SVGCleanOptions{CurrentColor: true}))
	want := `<svg viewBox="0 0 24 24" fill="none"><path fill="currentColor" fill-rule="evenodd" d="M0 0"/><path stroke="currentColor" fill="none" d="M1 1"/><path fill="url(#g)" d="M2 2"/></svg>`
	if out != want {
		t.Fatalf("current color:\n%s\nwant\n%s", out, want)
	}
	all := string(CleanSVG([]byte(sampleSVG), SVGCleanOptions{StripIDs: true, StripDimensions: true, CurrentColor: true}))
	if strings.Contains(all, `id="Vector"`) || strings.Contains(all, `<svg width=`) {
		t.Fatalf("combined:\n%s", all)
	}
	if !(SVGCleanOptions{StripIDs: true}).Enabled() || (SVGCleanOptions{}).Enabled() {
		t.Fatal("Enabled")
	}
	if got := string(CleanSVG([]byte(sampleSVG), SVGCleanOptions{})); got != sampleSVG {
		t.Fatal("no options must be a no-op")
	}
}
