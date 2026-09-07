package assets

import (
	"reflect"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Button/Primary":      "button-primary",
		"icon/arrow-right":    "icon-arrow-right",
		"  Hero  Image (2x) ": "hero-image-2x",
		"Ünicode":             "nicode",
		"///":                 "node",
		"":                    "node",
		"2:6":                 "2-6",
		"I2:5;3:21":           "i2-5-3-21",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if IDSlug("2:6") != "2-6" {
		t.Fatal("IDSlug")
	}
}

func TestScaleSuffixAndExtension(t *testing.T) {
	if ScaleSuffix(1) != "" || ScaleSuffix(0) != "" || ScaleSuffix(2) != "@2x" || ScaleSuffix(1.5) != "@1.5x" || ScaleSuffix(0.5) != "@0.5x" {
		t.Fatal("scale suffix")
	}
	if Extension("PNG") != "png" || Extension("jpeg") != "jpg" || Extension("JPG") != "jpg" || Extension("svg") != "svg" {
		t.Fatal("extension")
	}
	if DefaultScale("png") != 2 || DefaultScale("jpg") != 2 || DefaultScale("svg") != 1 || DefaultScale("pdf") != 1 {
		t.Fatal("default scale")
	}
}

func TestAssignNames(t *testing.T) {
	items := []Item{
		{NodeID: "2:6", Name: "Button/Primary", Format: "png", Scale: 1},
		{NodeID: "3:11", Name: "Button/Primary", Format: "png", Scale: 1},
		{NodeID: "3:11", Name: "Button/Primary", Format: "png", Scale: 1},
		{NodeID: "2:6", Name: "Button/Primary", Format: "png", Scale: 2},
		{NodeID: "2:2", Name: "Login", Format: "PNG", Scale: 2, Suffix: "@retina"},
		{NodeID: "2:9", Name: "icon/arrow-right", Format: "svg", Scale: 1},
		{NodeID: "9:9", Name: "", Format: "jpg", Scale: 1},
	}
	got := assignNames(items, NameByName)
	want := []string{
		"button-primary.png",
		"button-primary-3-11.png",
		"button-primary-3-11-2.png",
		"button-primary@2x.png",
		"login@retina.png",
		"icon-arrow-right.svg",
		"9-9.jpg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names by name = %v, want %v", got, want)
	}
	got = assignNames(items[:4], NameByID)
	want = []string{"2-6.png", "3-11.png", "3-11-2.png", "2-6@2x.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names by id = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(assignNames(items[:2], NameByName), assignNames(items[:2], NameByName)) {
		t.Fatal("names must be deterministic")
	}
}

func TestValidate(t *testing.T) {
	for _, f := range []string{"png", "jpg", "svg", "pdf", "PNG"} {
		if err := ValidateFormat(f); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	if ValidateFormat("gif") == nil || ValidateFormat("") == nil {
		t.Fatal("bad formats accepted")
	}
	if ValidateScale(0.01) != nil || ValidateScale(4) != nil || ValidateScale(2.5) != nil {
		t.Fatal("valid scales rejected")
	}
	if ValidateScale(0) == nil || ValidateScale(4.01) == nil || ValidateScale(-1) == nil {
		t.Fatal("invalid scales accepted")
	}
	if ValidateIconPatterns([]string{"icon/*", "Icon*"}) != nil {
		t.Fatal("valid patterns rejected")
	}
	if ValidateIconPatterns([]string{"[icon"}) == nil {
		t.Fatal("bad pattern accepted")
	}
}
