package cli

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// Help pages and the version screen. The command (cmd/barge) decides what
// they say; this file decides how they look. With colour off (a pipe, a
// file, NO_COLOR) the logo is left out and the rest comes out as plain text.

// Style says what the terminal can show.
type Style struct {
	Color     bool // ANSI colours at all
	TrueColor bool // 24-bit colours; otherwise the nearest of 256
}

// logo.png is the icon redrawn on a 32×32 pixel grid. Each terminal cell
// shows two pixels, one above the other, with "▀": foreground on top,
// background below. So the logo is 32 columns by 16 lines.
//
//go:embed logo.png
var logoPNG []byte

var (
	logoOnce sync.Once
	logoImg  image.Image
)

// Brand colours from the icon.
var (
	colMint = rgb{0x5F, 0xD7, 0xAF} // section titles, flags, the version
	colBlue = rgb{0x5F, 0xAF, 0xD7} // placeholders and links
)

// Tagline is the one-line description shown next to the logo.
const Tagline = "Find the code only one person knows — before they leave."

var taglineLines = []string{"Find the code only one person knows —", "before they leave."}

// HelpRow is one command or flag: Name in the left column, Text on the
// right, Default (if any) dimmed after it.
type HelpRow struct {
	Name    string
	Text    string
	Default string
}

// Example is a command line with a short note.
type Example struct {
	Cmd  string
	Note string
}

// HelpSection is a titled block of rows, examples or plain lines.
type HelpSection struct {
	Title    string
	Rows     []HelpRow
	Examples []Example
	Lines    []string
}

// HelpPage is one help screen.
type HelpPage struct {
	// Logo shows the logo with Version and the tagline (the main page).
	Logo     bool
	Version  string
	Title    string // "barge scan"
	Summary  string
	Usage    []string
	Sections []HelpSection
	Footer   []string
}

// BuildInfo describes the running binary.
type BuildInfo struct {
	Version  string
	Commit   string // short hash, "" if unknown
	Date     string // YYYY-MM-DD, "" if unknown
	Modified bool   // built from a tree with uncommitted changes
	Go       string
	Platform string // os/arch
	License  string
	Home     string
}

const (
	pageWidth = 80
	indent    = "  "
)

// RenderHelp prints a help page.
func RenderHelp(w io.Writer, p HelpPage, st Style) {
	s := styler{st}
	b := &strings.Builder{}

	if p.Logo {
		s.beside(b, []string{
			s.bold("Barge") + " " + s.paint(colMint, false, p.Version),
			s.dim(taglineLines[0]),
			s.dim(taglineLines[1]),
		}, 6)
	} else {
		fmt.Fprintf(b, "\n%s%s", indent, s.paint(colMint, true, p.Title))
		if p.Summary != "" {
			fmt.Fprintf(b, " %s %s", s.dim("—"), p.Summary)
		}
		b.WriteString("\n")
	}

	if len(p.Usage) > 0 {
		s.section(b, "Usage")
		for _, u := range p.Usage {
			fmt.Fprintf(b, "%s%s\n", indent+indent, s.usage(u))
		}
	}

	for _, sec := range p.Sections {
		s.section(b, sec.Title)
		for _, l := range s.rows(sec.Rows) {
			b.WriteString(indent + indent + l + "\n")
		}
		for _, l := range s.examples(sec.Examples) {
			b.WriteString(indent + indent + l + "\n")
		}
		for _, l := range sec.Lines {
			for _, wl := range wrap(l, pageWidth-4) {
				fmt.Fprintf(b, "%s%s\n", indent+indent, wl)
			}
		}
	}

	if len(p.Footer) > 0 {
		b.WriteString("\n")
		for _, f := range p.Footer {
			for _, l := range wrap(f, pageWidth-len(indent)) {
				fmt.Fprintf(b, "%s%s\n", indent, s.dim(l))
			}
		}
	}
	b.WriteString("\n")
	io.WriteString(w, b.String())
}

// RenderVersion prints the version screen: the logo with build details.
func RenderVersion(w io.Writer, v BuildInfo, st Style) {
	s := styler{st}
	lines := []string{s.bold("Barge") + " " + s.paint(colMint, false, v.Version)}
	// The commit and build date are known only for builds from a git
	// checkout; a line saying "unknown" would tell nothing.
	var build []string
	if v.Commit != "" {
		c := "commit " + v.Commit
		if v.Modified {
			c += " (modified)"
		}
		build = append(build, c)
	}
	if v.Date != "" {
		build = append(build, "built "+v.Date)
	}
	if len(build) > 0 {
		lines = append(lines, s.dim(strings.Join(build, " · ")))
	}
	lines = append(lines,
		s.dim(v.Go+" · "+v.Platform),
		s.dim(v.License),
		s.paint(colBlue, false, v.Home),
	)
	b := &strings.Builder{}
	s.beside(b, lines, 5)
	b.WriteString("\n")
	io.WriteString(w, b.String())
}

// --- drawing ---

type styler struct{ Style }

type rgb struct{ r, g, b uint8 }

func (s styler) code(c rgb, background bool) string {
	layer := "38"
	if background {
		layer = "48"
	}
	if s.TrueColor {
		return fmt.Sprintf("%s;2;%d;%d;%d", layer, c.r, c.g, c.b)
	}
	return fmt.Sprintf("%s;5;%d", layer, xterm256(c))
}

func (s styler) paint(c rgb, bold bool, text string) string {
	if !s.Color || text == "" {
		return text
	}
	code := s.code(c, false)
	if bold {
		code = "1;" + code
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s styler) sgr(code, text string) string {
	if !s.Color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s styler) dim(t string) string  { return s.sgr("2", t) }
func (s styler) bold(t string) string { return s.sgr("1", t) }

// beside prints the logo with text lines to its right, starting at line
// top of the logo. Without colour there is no logo, only the text.
func (s styler) beside(b *strings.Builder, text []string, top int) {
	b.WriteString("\n")
	if !s.Color {
		for _, t := range text {
			b.WriteString(strings.TrimRight(" "+t, " ") + "\n")
		}
		return
	}
	logo := s.logo()
	for i, line := range logo {
		b.WriteString(" " + line)
		if j := i - top; j >= 0 && j < len(text) && text[j] != "" {
			b.WriteString("   " + text[j])
		}
		b.WriteString("\n")
	}
}

// logo renders logo.png as half-block cells.
func (s styler) logo() []string {
	logoOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(logoPNG))
		if err == nil {
			logoImg = img
		}
	})
	if logoImg == nil {
		return nil
	}
	bounds := logoImg.Bounds()
	px := func(x, y int) (rgb, bool) {
		if y >= bounds.Max.Y {
			return rgb{}, false
		}
		r, g, b, a := logoImg.At(x, y).RGBA()
		return rgb{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}, a >= 0x8000
	}
	var lines []string
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		var l strings.Builder
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			top, topOK := px(x, y)
			bot, botOK := px(x, y+1)
			switch {
			case topOK && botOK:
				fmt.Fprintf(&l, "\x1b[%s;%sm▀", s.code(top, false), s.code(bot, true))
			case topOK:
				fmt.Fprintf(&l, "\x1b[0;%sm▀", s.code(top, false))
			case botOK:
				fmt.Fprintf(&l, "\x1b[0;%sm▄", s.code(bot, false))
			default:
				l.WriteString("\x1b[0m ")
			}
		}
		l.WriteString("\x1b[0m")
		lines = append(lines, l.String())
	}
	return lines
}

func (s styler) section(b *strings.Builder, title string) {
	fmt.Fprintf(b, "\n%s%s\n", indent, s.paint(colMint, true, strings.ToUpper(title)))
}

// usage highlights the command words and colours placeholders.
func (s styler) usage(u string) string {
	fields := strings.Fields(u)
	for i, f := range fields {
		if strings.HasPrefix(f, "[") || strings.HasPrefix(f, "<") {
			fields[i] = s.paint(colBlue, false, f)
		} else {
			fields[i] = s.bold(f)
		}
	}
	return strings.Join(fields, " ")
}

// rows lays out commands or flags in two columns, wrapping the text.
func (s styler) rows(rows []HelpRow) []string {
	nameW := 0
	for _, r := range rows {
		nameW = max(nameW, utf8.RuneCountInString(r.Name))
	}
	nameW = min(nameW, 26)
	textW := pageWidth - 2*len(indent) - nameW - 3
	var out []string
	for _, r := range rows {
		name := s.bold(r.Name)
		if strings.HasPrefix(r.Name, "-") {
			name = s.paint(colMint, false, r.Name)
		}
		text := r.Text
		if r.Default != "" {
			// \x00 marks where the dimmed part starts; \x01 keeps it on one line.
			text += " \x00" + strings.ReplaceAll("(default "+r.Default+")", " ", "\x01")
		}
		nameLen := utf8.RuneCountInString(r.Name)
		for i, l := range wrap(text, textW) {
			l = strings.ReplaceAll(l, "\x01", " ")
			if k := strings.IndexByte(l, 0); k >= 0 {
				l = l[:k] + s.dim(l[k+1:])
			}
			switch {
			case i == 0 && nameLen <= nameW:
				out = append(out, name+strings.Repeat(" ", nameW-nameLen+3)+l)
			case i == 0:
				out = append(out, name, strings.Repeat(" ", nameW+3)+l)
			default:
				out = append(out, strings.Repeat(" ", nameW+3)+l)
			}
		}
	}
	return out
}

// examples prints "$ command   note", or the note above when it does not fit.
func (s styler) examples(ex []Example) []string {
	cmdW, noteW := 0, 0
	for _, e := range ex {
		cmdW = max(cmdW, utf8.RuneCountInString(e.Cmd))
		noteW = max(noteW, utf8.RuneCountInString(e.Note))
	}
	beside := 2*len(indent)+2+cmdW+3+noteW <= pageWidth
	var out []string
	for _, e := range ex {
		line := s.dim("$ ") + s.bold(e.Cmd)
		switch {
		case e.Note == "":
		case beside:
			line += strings.Repeat(" ", cmdW-utf8.RuneCountInString(e.Cmd)+3) + s.dim(e.Note)
		default:
			out = append(out, s.dim("# "+e.Note))
		}
		out = append(out, line)
	}
	return out
}

// xterm256 maps a colour to the nearest of the 256-colour palette (the
// 6×6×6 cube or the grey ramp), for terminals without 24-bit colour.
func xterm256(c rgb) int {
	levels := [6]int{0, 95, 135, 175, 215, 255}
	nearest := func(v uint8) int {
		best, bestD := 0, 1<<30
		for i, l := range levels {
			if d := (int(v) - l) * (int(v) - l); d < bestD {
				best, bestD = i, d
			}
		}
		return best
	}
	ri, gi, bi := nearest(c.r), nearest(c.g), nearest(c.b)
	cube := 16 + 36*ri + 6*gi + bi
	dist := func(r, g, b int) int {
		dr, dg, db := int(c.r)-r, int(c.g)-g, int(c.b)-b
		return dr*dr + dg*dg + db*db
	}
	best, bestD := cube, dist(levels[ri], levels[gi], levels[bi])
	avg := (int(c.r) + int(c.g) + int(c.b)) / 3
	if gi := min(max((avg-8)/10, 0), 23); dist(8+10*gi, 8+10*gi, 8+10*gi) < bestD {
		best = 232 + gi
	}
	return best
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m|\x1b\\]8;;[^\x1b]*\x1b\\\\")

// visibleWidth is the number of columns s takes, ignoring escape codes.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansi.ReplaceAllString(s, ""))
}

// wrap breaks s into lines of at most width columns, at spaces.
func wrap(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	return append(lines, cur)
}
