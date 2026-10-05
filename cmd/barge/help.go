package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"unicode"
	"unicode/utf8"

	"gitlab.com/codebarge/barge/features/ownership/transport/cli"
)

const homeURL = "https://gitlab.com/codebarge/barge"

// command describes one subcommand for the help pages.
type command struct {
	name     string
	summary  string
	usage    string
	about    []string
	flags    func() *flag.FlagSet // nil: no flags
	own      string               // title of the command's own flags section
	examples []cli.Example
	notes    []string
}

var commands = []command{
	{
		name:    "scan",
		summary: "who knows each module, the risk, and what to do next",
		usage:   "barge scan [flags] [path]",
		about: []string{
			"Reads the git history of the repository at path (default: the current folder) " +
				"and lists modules from the riskiest: who knows each one, its bus factor, and how " +
				"much of it AI wrote. Ends with concrete next steps.",
		},
		flags: func() *flag.FlagSet { fs, _ := newScanFlags(); return fs },
		own:   "Output",
		examples: []cli.Example{
			{Cmd: "barge scan", Note: "the repository you are in"},
			{Cmd: "barge scan ~/code/shop --all --top 0", Note: "every module"},
			{Cmd: "barge scan --leaving oleg@acme.io", Note: "what happens if Oleg leaves"},
			{Cmd: "barge scan --only 'pkg/*'", Note: "only modules under pkg/"},
			{Cmd: "barge scan --module 'internal/features/*'", Note: "each feature is one module"},
			{Cmd: "barge scan --fail-on high", Note: "in CI: fail on high risk"},
		},
		notes: []string{"Exit codes: 0 fine, 1 --fail-on threshold reached, 2 error."},
	},
	{
		name:    "handover",
		summary: "a handover checklist for one person, in Markdown",
		usage:   "barge handover [flags] <email> [path]",
		about: []string{
			"Builds a checklist for the person who committed as <email>: the modules only they " +
				"know, what the bus factor becomes without them, a successor for each, the files " +
				"they wrote and questions about their largest commits and reverts.",
		},
		flags: func() *flag.FlagSet { fs, _ := newHandoverFlags(); return fs },
		own:   "Output",
		examples: []cli.Example{
			{Cmd: "barge handover oleg@acme.io -o handover.md", Note: "write the checklist to a file"},
			{Cmd: "barge handover oleg@acme.io ~/code/shop --max-modules 0", Note: "every module"},
		},
		notes: []string{`Not sure which e-mail someone used? Run "barge scan --json" and look under "people".`},
	},
	{
		name:    "version",
		summary: "version and build details",
		usage:   "barge version [--short | --json]",
		flags:   func() *flag.FlagSet { fs, _ := newVersionFlags(); return fs },
		own:     "Flags",
		examples: []cli.Example{
			{Cmd: "barge version"},
			{Cmd: "barge --version", Note: "one line, for scripts"},
		},
	},
	{
		name:    "help",
		summary: "this help, or the help for one command",
		usage:   "barge help [command]",
		examples: []cli.Example{
			{Cmd: "barge help scan"},
			{Cmd: "barge scan -h", Note: "the same"},
		},
	},
}

func findCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// mainHelp is the page for "barge", "barge help" and "barge -h".
func mainHelp() cli.HelpPage {
	rows := make([]cli.HelpRow, len(commands))
	for i, c := range commands {
		rows[i] = cli.HelpRow{Name: c.name, Text: capitalise(c.summary)}
	}
	return cli.HelpPage{
		Logo:    true,
		Version: currentVersion(),
		Usage:   []string{"barge <command> [flags] [path]"},
		Sections: []cli.HelpSection{
			{Title: "Commands", Rows: rows},
			{Title: "Quick start", Examples: []cli.Example{
				{Cmd: "barge scan", Note: "who knows what here"},
				{Cmd: "barge scan --leaving oleg@acme.io", Note: "what if Oleg leaves"},
				{Cmd: "barge handover oleg@acme.io -o handover.md", Note: "his handover checklist"},
			}},
			{Title: "Configuration", Lines: []string{
				"Commit a .barge.json at the repository root to set modules, exclusions, " +
					"people who are leaving and owner corrections once for everyone.",
			}},
		},
		Footer: []string{
			`Run "barge help <command>" for its flags. Path defaults to the current folder.`,
			"Barge reads your local git history only: nothing leaves your machine.",
			"Docs and issues: " + homeURL,
		},
	}
}

// commandHelp is the page for "barge help <command>" and "barge <command> -h".
func commandHelp(c command) cli.HelpPage {
	p := cli.HelpPage{Title: "barge " + c.name, Summary: c.summary, Usage: []string{c.usage}}
	if len(c.about) > 0 {
		p.Sections = append(p.Sections, cli.HelpSection{Title: "About", Lines: c.about})
	}
	if c.flags != nil {
		own, shared := flagRows(c.flags())
		if len(own) > 0 {
			p.Sections = append(p.Sections, cli.HelpSection{Title: c.own, Rows: own})
		}
		if len(shared) > 0 {
			p.Sections = append(p.Sections, cli.HelpSection{Title: "Analysis", Rows: shared})
		}
	}
	if len(c.examples) > 0 {
		p.Sections = append(p.Sections, cli.HelpSection{Title: "Examples", Examples: c.examples})
	}
	p.Footer = append(p.Footer, c.notes...)
	return p
}

// argNames are the placeholders shown after flags that take a value.
var argNames = map[string]string{
	"depth": "N", "top": "N", "max-modules": "N",
	"rev": "REV", "as-of": "DATE",
	"half-life": "DAYS", "active-days": "DAYS",
	"module": "GLOB", "exclude": "GLOB",
	"leaving": "EMAIL", "left": "EMAIL",
	"fail-on": "LEVEL", "o": "FILE", "only": "GLOB",
}

// flagRows turns a flag set into help rows: the command's own flags first,
// then the analysis flags every analysing command shares.
func flagRows(fs *flag.FlagSet) (own, shared []cli.HelpRow) {
	sharedNames := map[string]bool{}
	var o options
	probe := flag.NewFlagSet("", flag.ContinueOnError)
	o.register(probe)
	probe.VisitAll(func(f *flag.Flag) { sharedNames[f.Name] = true })

	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		if len(f.Name) == 1 {
			name = "-" + f.Name
		}
		if a := argNames[f.Name]; a != "" {
			name += " " + a
		}
		def := f.DefValue
		if def == "false" || def == "" || def == "0" || def == "[]" {
			def = ""
		}
		row := cli.HelpRow{Name: name, Text: capitalise(f.Usage), Default: def}
		if sharedNames[f.Name] {
			shared = append(shared, row)
		} else {
			own = append(own, row)
		}
	})
	return own, shared
}

// help handles "barge help [command]".
func help(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		cli.RenderHelp(stdout, mainHelp(), termStyle(stdout))
		return exitOK
	}
	c, ok := findCommand(args[0])
	if !ok {
		unknownCommand(args[0], stderr)
		return exitUsageErr
	}
	cli.RenderHelp(stdout, commandHelp(c), termStyle(stdout))
	return exitOK
}

func unknownCommand(name string, stderr io.Writer) {
	fmt.Fprintf(stderr, "barge: unknown command %q\n", name)
	if s := suggest(name); s != "" {
		fmt.Fprintf(stderr, "Did you mean \"barge %s\"?\n", s)
	}
	fmt.Fprintln(stderr, `Run "barge help" to see all commands.`)
}

// suggest returns the command closest to a mistyped one, if any is close.
func suggest(name string) string {
	best, bestD := "", 3
	for _, c := range commands {
		if d := distance(strings.ToLower(name), c.name); d < bestD {
			best, bestD = c.name, d
		}
	}
	return best
}

// distance is the Levenshtein edit distance.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// --- version ---

type versionFlags struct{ short, json bool }

func newVersionFlags() (*flag.FlagSet, *versionFlags) {
	v := &versionFlags{}
	fs := flag.NewFlagSet("barge version", flag.ContinueOnError)
	fs.BoolVar(&v.short, "short", false, "print only the version number")
	fs.BoolVar(&v.json, "json", false, "print the build details as JSON")
	return fs, v
}

func versionCmd(args []string, stdout, stderr io.Writer) int {
	fs, v := newVersionFlags()
	if code, done := parseOrHelp(fs, "version", args, stdout, stderr); done {
		return code
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, "barge: version takes no arguments")
		return exitUsageErr
	}
	bi := buildInfo()
	switch {
	case v.json:
		if err := writeJSON(stdout, map[string]any{
			"version": bi.Version, "commit": bi.Commit, "date": bi.Date, "modified": bi.Modified,
			"go": bi.Go, "platform": bi.Platform,
		}); err != nil {
			fmt.Fprintln(stderr, "barge:", err)
			return exitError
		}
	case v.short:
		fmt.Fprintln(stdout, bi.Version)
	default:
		cli.RenderVersion(stdout, bi, termStyle(stdout))
	}
	return exitOK
}

// currentVersion is, in order: the version set at build time, the module
// version when installed with "go install ...@vX.Y.Z", or the version
// written in the source (releaseVersion).
func currentVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		v := strings.TrimSuffix(bi.Main.Version, "+dirty") // "modified" is shown separately
		if v != "" && v != "(devel)" && !pseudoVersion.MatchString(v) {
			return v
		}
	}
	return releaseVersion
}

// pseudoVersion matches the versions Go makes up for untagged commits, such
// as v0.0.0-20261006200248-c9bbbe3cb1f7: they say less than releaseVersion.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)

func buildInfo() cli.BuildInfo {
	info := cli.BuildInfo{
		Version:  currentVersion(),
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
		License:  "Apache 2.0",
		Home:     homeURL,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Commit = s.Value
				if len(info.Commit) > 7 {
					info.Commit = info.Commit[:7]
				}
			case "vcs.time":
				if len(s.Value) >= 10 {
					info.Date = s.Value[:10]
				}
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
	}
	return info
}

// parseOrHelp parses flags; on -h it prints the command's help page, on an
// error a hint. done is true when the command should return code now.
func parseOrHelp(fs *flag.FlagSet, name string, args []string, stdout, stderr io.Writer) (code int, done bool) {
	fs.SetOutput(stderr)
	fs.Usage = func() {} // our own help below, not the flag package's
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			c, _ := findCommand(name)
			cli.RenderHelp(stdout, commandHelp(c), termStyle(stdout))
			return exitOK, true
		}
		fmt.Fprintf(stderr, "Run \"barge help %s\" to see all flags.\n", name)
		return exitUsageErr, true
	}
	return 0, false
}

func capitalise(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// termStyle says what the terminal behind w can show.
func termStyle(w io.Writer) cli.Style {
	if !useColor(w) {
		return cli.Style{}
	}
	return cli.Style{Color: true, TrueColor: trueColor()}
}

// trueColor guesses whether the terminal shows 24-bit colour. Most modern
// ones do; the rest get the nearest of 256 colours.
func trueColor() bool {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	if os.Getenv("WT_SESSION") != "" { // Windows Terminal
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "vscode", "ghostty", "Hyper", "Tabby", "rio":
		return true
	}
	term := os.Getenv("TERM")
	for _, t := range []string{"direct", "truecolor", "kitty", "ghostty", "alacritty", "foot", "wezterm"} {
		if strings.Contains(term, t) {
			return true
		}
	}
	return false
}
