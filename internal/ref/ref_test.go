package ref

import (
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

func TestParse(t *testing.T) {
	const key = "AbC123def456GHI789jkl0"
	cases := []struct {
		name    string
		in      string
		want    Ref
		wantErr bool
	}{
		{"bare key", key, Ref{FileKey: key, Kind: KindKey}, false},
		{"bare key with spaces", "  " + key + " ", Ref{FileKey: key, Kind: KindKey}, false},
		{"file url", "https://www.figma.com/file/" + key + "/Web-App", Ref{FileKey: key, Kind: KindFile}, false},
		{"design url", "https://www.figma.com/design/" + key + "/Web-App", Ref{FileKey: key, Kind: KindDesign}, false},
		{"design url with node", "https://www.figma.com/design/" + key + "/Web-App?node-id=12-34&t=abc", Ref{FileKey: key, NodeID: "12:34", Kind: KindDesign}, false},
		{"design url with colon node", "https://www.figma.com/design/" + key + "/Web-App?node-id=12%3A34", Ref{FileKey: key, NodeID: "12:34", Kind: KindDesign}, false},
		{"design url with version", "https://www.figma.com/design/" + key + "/Web-App?node-id=1-2&version-id=987654", Ref{FileKey: key, NodeID: "1:2", VersionID: "987654", Kind: KindDesign}, false},
		{"design url without name", "https://www.figma.com/design/" + key, Ref{FileKey: key, Kind: KindDesign}, false},
		{"board url", "https://www.figma.com/board/" + key + "/Retro", Ref{FileKey: key, Kind: KindBoard}, false},
		{"proto url", "https://www.figma.com/proto/" + key + "/Flow?node-id=3-4", Ref{FileKey: key, NodeID: "3:4", Kind: KindProto}, false},
		{"slides url", "https://www.figma.com/slides/" + key, Ref{FileKey: key, Kind: KindSlides}, false},
		{"branch url", "https://www.figma.com/design/" + key + "/branch/BranchKey0123456789/Web-App", Ref{FileKey: "BranchKey0123456789", Kind: KindDesign}, false},
		{"no scheme", "www.figma.com/design/" + key + "/Web-App?node-id=1-2", Ref{FileKey: key, NodeID: "1:2", Kind: KindDesign}, false},
		{"bare host", "figma.com/file/" + key, Ref{FileKey: key, Kind: KindFile}, false},
		{"instance node id", "https://www.figma.com/design/" + key + "?node-id=I12-34%3B56-78", Ref{FileKey: key, NodeID: "I12:34;56:78", Kind: KindDesign}, false},
		{"empty", "", Ref{}, true},
		{"not figma", "https://example.com/design/" + key, Ref{}, true},
		{"unsupported form", "https://www.figma.com/community/file/" + key, Ref{}, true},
		{"missing key", "https://www.figma.com/design/", Ref{}, true},
		{"bad key", "https://www.figma.com/design/bad_key!/Name", Ref{}, true},
		{"bad node", "https://www.figma.com/design/" + key + "?node-id=abc", Ref{}, true},
		{"short garbage", "abc", Ref{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				if figctl.From(err).Code != figctl.CodeUsage {
					t.Fatalf("code = %s, want USAGE", figctl.From(err).Code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseNodeID(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"1:2", "1:2", false},
		{"1-2", "1:2", false},
		{"0:1", "0:1", false},
		{" 12-345 ", "12:345", false},
		{"I1:2;3:4", "I1:2;3:4", false},
		{"I1-2;3-4", "I1:2;3:4", false},
		{"", "", true},
		{"12", "", true},
		{"a:b", "", true},
		{"1:2:3", "", true},
		{"1-2-3", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseNodeID(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
