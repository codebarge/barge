package cli

import (
	"strings"
	"testing"
)

func TestLogoIs32By16(t *testing.T) {
	for _, st := range []Style{{Color: true, TrueColor: true}, {Color: true}} {
		lines := styler{st}.logo()
		if len(lines) != 16 {
			t.Fatalf("%+v: %d lines, want 16", st, len(lines))
		}
		for i, l := range lines {
			if w := visibleWidth(l); w != 32 {
				t.Errorf("%+v: line %d is %d columns wide", st, i, w)
			}
		}
	}
}

func TestColouredHelpFits80Columns(t *testing.T) {
	var b strings.Builder
	RenderHelp(&b, HelpPage{
		Logo:    true,
		Version: "v1.2.3",
		Usage:   []string{"barge test [flags]"},
		Sections: []HelpSection{{
			Title: "Flags",
			Rows: []HelpRow{
				{Name: "--long-flag-name VALUE", Text: strings.Repeat("words that wrap ", 12), Default: "42"},
				{Name: "--x", Text: "short"},
			},
			Examples: []Example{{Cmd: "barge test", Note: "a note"}},
		}},
	}, Style{Color: true, TrueColor: true})
	for _, l := range strings.Split(b.String(), "\n") {
		if w := visibleWidth(l); w > 80 {
			t.Errorf("line of %d columns: %q", w, l)
		}
	}
	if !strings.Contains(b.String(), "▀") || !strings.Contains(b.String(), "FLAGS") {
		t.Errorf("no logo or section title:\n%s", b.String())
	}
}

func TestXterm256(t *testing.T) {
	cases := []struct {
		c    rgb
		want int
	}{
		{rgb{0, 0, 0}, 16},
		{rgb{255, 255, 255}, 231},
		{rgb{0x5F, 0xD7, 0xAF}, 79}, // exactly a cube colour
		{rgb{128, 128, 128}, 244},   // a grey: the ramp is closer than the cube
	}
	for _, c := range cases {
		if got := xterm256(c.c); got != c.want {
			t.Errorf("xterm256(%v) = %d, want %d", c.c, got, c.want)
		}
	}
}

func TestColouredVersionFits80Columns(t *testing.T) {
	var b strings.Builder
	RenderVersion(&b, BuildInfo{Version: "v10.20.30", Commit: "1a2b3c4", Modified: true, Date: "2026-10-06",
		Go: "go1.26.1", Platform: "windows/amd64", License: "Apache 2.0", Home: "https://gitlab.com/codebarge/barge"},
		Style{Color: true})
	for _, l := range strings.Split(b.String(), "\n") {
		if w := visibleWidth(l); w > 80 {
			t.Errorf("line of %d columns: %q", w, l)
		}
	}
}

func TestPlainVersionHasNoEscapes(t *testing.T) {
	var b strings.Builder
	RenderVersion(&b, BuildInfo{Version: "v1.2.3", Go: "go1.26.1", Platform: "linux/amd64", License: "Apache 2.0", Home: "https://x"}, Style{})
	if strings.Contains(b.String(), "\x1b") || !strings.Contains(b.String(), "v1.2.3") || !strings.Contains(b.String(), "go1.26.1") {
		t.Errorf("plain version:\n%s", b.String())
	}
}
