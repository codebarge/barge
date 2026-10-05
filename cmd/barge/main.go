// Command barge finds the parts of a codebase that only one person knows.
//
//	barge scan [flags] [path]
//	barge handover [flags] <email> [path]
//	barge version
//	barge help [command]
//
// It reads the local git history only: no code or data leaves the machine.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/codebarge/barge/core/git"
	"gitlab.com/codebarge/barge/features/ownership/dto"
	"gitlab.com/codebarge/barge/features/ownership/repository/gitsource"
	"gitlab.com/codebarge/barge/features/ownership/service"
	"gitlab.com/codebarge/barge/features/ownership/transport/cli"
)

// Exit codes.
const (
	exitOK       = 0
	exitRisk     = 1 // --fail-on threshold reached
	exitError    = 2 // the scan could not run
	exitUsageErr = 2 // bad flags or configuration
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return help(nil, stdout, stderr)
	}
	switch args[0] {
	case "scan":
		return scan(ctx, args[1:], stdout, stderr)
	case "handover":
		return handover(ctx, args[1:], stdout, stderr)
	case "version":
		return versionCmd(args[1:], stdout, stderr)
	case "--version", "-v":
		fmt.Fprintln(stdout, "barge", currentVersion())
		return exitOK
	case "help":
		return help(args[1:], stdout, stderr)
	case "--help", "-h":
		return help(nil, stdout, stderr)
	default:
		unknownCommand(args[0], stderr)
		return exitUsageErr
	}
}

// options shared by every command that analyses a repository.
type options struct {
	depth      int
	rev        string
	asOf       string
	halfLife   int
	activeDays int
	noConfig   bool
	modules    listFlag
	exclude    listFlag
	leaving    listFlag
	left       listFlag
}

func (o *options) register(fs *flag.FlagSet) {
	fs.IntVar(&o.depth, "depth", 0, "group files by their first N directories (0 = by containing directory)")
	fs.StringVar(&o.rev, "rev", "HEAD", "revision to analyse, e.g. main or origin/develop")
	fs.StringVar(&o.asOf, "as-of", "", "reference date YYYY-MM-DD (default: date of the newest commit)")
	fs.IntVar(&o.halfLife, "half-life", 180, "days for a change to lose half its weight")
	fs.IntVar(&o.activeDays, "active-days", 180, "people without commits for this many days count as inactive")
	fs.BoolVar(&o.noConfig, "no-config", false, "ignore "+service.ConfigFileName)
	fs.Var(&o.modules, "module", "count each directory matching this glob as one module, e.g. 'internal/features/*' (repeatable); this groups, it does not filter: see --only")
	fs.Var(&o.exclude, "exclude", "glob of files to ignore, e.g. 'docs/**' (repeatable)")
	fs.Var(&o.leaving, "leaving", "email of someone who is leaving: see what happens (repeatable)")
	fs.Var(&o.left, "left", "email of someone who already left (repeatable)")
}

// target is a repository ready to analyse.
type target struct {
	repo *git.Repo
	head string
	rev  string
	cfg  service.Config
}

func (t target) header(started time.Time) cli.Header {
	return cli.Header{Repo: filepath.Base(t.repo.Dir), Rev: t.label(), Elapsed: time.Since(started)}
}

// prepare opens the repository, applies .barge.json and resolves the
// revision. It prints its own errors and returns an exit code on failure.
func prepare(ctx context.Context, dir string, o options, stderr io.Writer) (target, int) {
	cfg, err := buildConfig(o)
	if err != nil {
		fmt.Fprintln(stderr, "barge:", err)
		return target{}, exitUsageErr
	}
	if _, err := os.Stat(dir); err != nil {
		if strings.Contains(dir, "://") {
			fmt.Fprintf(stderr, "barge: %s is a URL; clone it first (git clone %s) and pass the folder\n", dir, dir)
		} else {
			fmt.Fprintf(stderr, "barge: %s does not exist\n", dir)
		}
		return target{}, exitError
	}
	repo, err := git.Open(ctx, dir)
	if err != nil {
		fmt.Fprintf(stderr, "barge: %s is not inside a git repository\n", dir)
		return target{}, exitError
	}
	if !o.noConfig {
		data, err := os.ReadFile(filepath.Join(repo.Dir, service.ConfigFileName))
		switch {
		case err == nil:
			fc, perr := service.ParseFileConfig(data)
			if perr != nil {
				fmt.Fprintln(stderr, "barge:", perr)
				return target{}, exitUsageErr
			}
			flagStatuses := cfg.Statuses
			cfg.Statuses = nil
			fc.Apply(&cfg)
			cfg.Statuses = append(cfg.Statuses, flagStatuses...) // flags win
		case !errors.Is(err, os.ErrNotExist):
			fmt.Fprintln(stderr, "barge:", err)
			return target{}, exitError
		}
	}
	head, err := repo.ResolveRev(ctx, o.rev)
	if err != nil {
		fmt.Fprintf(stderr, "barge: cannot resolve %q: is the repository empty, or is it a remote branch (try origin/%s)?\n", o.rev, o.rev)
		return target{}, exitError
	}
	return target{repo: repo, head: head, rev: o.rev, cfg: cfg}, exitOK
}

// scanFlags are the flags of "barge scan".
type scanFlags struct {
	options
	jsonOut, all, noColor bool
	top                   int
	failOn                string
	only                  listFlag
}

func newScanFlags() (*flag.FlagSet, *scanFlags) {
	f := &scanFlags{}
	fs := flag.NewFlagSet("barge scan", flag.ContinueOnError)
	fs.IntVar(&f.top, "top", 25, "show at most N modules (0 = no limit)")
	fs.BoolVar(&f.all, "all", false, "list low-risk modules too")
	fs.BoolVar(&f.jsonOut, "json", false, "print the full report as JSON")
	fs.StringVar(&f.failOn, "fail-on", "", "exit with code 1 if any module is at this risk or worse: critical, high, medium")
	fs.BoolVar(&f.noColor, "no-color", false, "disable colours")
	fs.Var(&f.only, "only", "show only modules under directories matching this glob, e.g. 'pkg/*' or 'cmd/compose' (repeatable)")
	f.register(fs)
	return fs, f
}

func scan(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, f := newScanFlags()
	pos, code, done := parseCommand(fs, "scan", args, stdout, stderr)
	if done {
		return code
	}
	o, jsonOut, all, noColor, top, failOnFlag := f.options, f.jsonOut, f.all, f.noColor, f.top, f.failOn
	if len(pos) > 1 {
		extraArgs(stderr, "scan", pos[1:])
		return exitUsageErr
	}
	if top < 0 {
		fmt.Fprintln(stderr, "barge: --top must be >= 0")
		return exitUsageErr
	}
	var failOn dto.Risk
	if failOnFlag != "" {
		failOn = dto.Risk(strings.ToLower(failOnFlag))
		if failOn != dto.RiskCritical && failOn != dto.RiskHigh && failOn != dto.RiskMedium {
			fmt.Fprintf(stderr, "barge: --fail-on must be critical, high or medium, not %q\n", failOnFlag)
			return exitUsageErr
		}
	}

	t, code := prepare(ctx, argOr(pos, 0, "."), o, stderr)
	if code != exitOK {
		return code
	}
	started := time.Now()
	rep, err := service.Analyze(ctx, gitsource.New(t.repo, t.head), t.cfg)
	if err != nil {
		fmt.Fprintln(stderr, "barge:", err)
		return exitError
	}

	// A module rule that matches nothing is almost always a typo or a
	// directory this repository does not have: say so instead of ignoring it.
	if unused := service.UnusedRules(rep, t.cfg.ModuleRules); len(unused) > 0 {
		for _, r := range unused {
			fmt.Fprintf(stderr, "barge: module rule %q matches no directory in this repository, so it changed nothing.\n", r)
		}
		if len(f.only) == 0 {
			fmt.Fprintln(stderr, "       --module groups directories into modules; to show only some modules, use --only.")
		}
	}

	total := 0
	for _, m := range rep.Modules {
		if m.Kind == dto.KindModule {
			total++
		}
	}
	if len(f.only) > 0 {
		rep = service.Filter(rep, f.only)
		if rep.Summary.Modules == 0 {
			fmt.Fprintf(stderr, "barge: no module matches --only %s.\n", strings.Join(quoteAll(f.only), ", "))
			fmt.Fprintln(stderr, `       Module paths are directories such as "pkg/compose"; "barge scan --all --top 0" lists them all.`)
			return exitUsageErr
		}
	}

	if jsonOut {
		if err := writeJSON(stdout, rep); err != nil {
			fmt.Fprintln(stderr, "barge:", err)
			return exitError
		}
	} else {
		cli.Render(stdout, rep, t.header(started), cli.Options{
			All: all, Top: top, Color: !noColor && useColor(stdout),
			KnowledgeThreshold: t.cfg.KnowledgeThreshold,
			Only:               f.only, Total: total,
		})
	}

	if failOn != "" {
		for _, m := range rep.Modules {
			if m.Kind == dto.KindModule && m.Risk.AtLeast(failOn) {
				return exitRisk
			}
		}
	}
	return exitOK
}

// handoverFlags are the flags of "barge handover".
type handoverFlags struct {
	options
	jsonOut    bool
	outFile    string
	maxModules int
}

func newHandoverFlags() (*flag.FlagSet, *handoverFlags) {
	f := &handoverFlags{}
	fs := flag.NewFlagSet("barge handover", flag.ContinueOnError)
	fs.StringVar(&f.outFile, "o", "", "write to this file instead of the terminal, e.g. handover.md")
	fs.IntVar(&f.maxModules, "max-modules", 10, "include at most N modules, most at risk first (0 = all)")
	fs.BoolVar(&f.jsonOut, "json", false, "print the handover as JSON instead of Markdown")
	f.register(fs)
	return fs, f
}

func handover(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, f := newHandoverFlags()
	pos, code, done := parseCommand(fs, "handover", args, stdout, stderr)
	if done {
		return code
	}
	o, jsonOut, outFile, maxModules := f.options, f.jsonOut, f.outFile, f.maxModules
	if len(pos) > 2 && strings.Contains(pos[0], "@") {
		extraArgs(stderr, "handover", pos[2:])
		return exitUsageErr
	}
	if len(pos) < 1 || len(pos) > 2 || !strings.Contains(pos[0], "@") {
		fmt.Fprintln(stderr, "barge: handover needs the person's e-mail: barge handover <email> [path]")
		fmt.Fprintln(stderr, `Run "barge help handover" for details.`)
		return exitUsageErr
	}
	email := pos[0]

	t, code := prepare(ctx, argOr(pos, 1, "."), o, stderr)
	if code != exitOK {
		return code
	}
	h, err := service.Handover(ctx, gitsource.New(t.repo, t.head), t.cfg, email, maxModules)
	if errors.Is(err, service.ErrPersonNotFound) {
		fmt.Fprintf(stderr, "barge: nobody committed as %s on %s; run \"barge scan --json\" to see everyone's e-mails\n", email, t.label())
		return exitError
	}
	if err != nil {
		fmt.Fprintln(stderr, "barge:", err)
		return exitError
	}

	w := stdout
	if outFile != "" {
		f, err := os.Create(outFile)
		if err != nil {
			fmt.Fprintln(stderr, "barge:", err)
			return exitError
		}
		defer f.Close()
		w = f
	}
	if jsonOut {
		err = writeJSON(w, h)
	} else {
		cli.RenderHandover(w, h, t.header(time.Now()))
	}
	if err != nil {
		fmt.Fprintln(stderr, "barge:", err)
		return exitError
	}
	if outFile != "" {
		fmt.Fprintf(stderr, "Handover checklist for %s written to %s (%d modules).\n", h.Person.Name, outFile, len(h.Modules))
	}
	return exitOK
}

func buildConfig(o options) (service.Config, error) {
	cfg := service.DefaultConfig()
	if o.depth < 0 || o.halfLife <= 0 || o.activeDays <= 0 {
		return cfg, errors.New("--depth must be >= 0, --half-life and --active-days > 0")
	}
	cfg.Depth = o.depth
	cfg.HalfLife = time.Duration(o.halfLife) * 24 * time.Hour
	cfg.ActiveWindow = time.Duration(o.activeDays) * 24 * time.Hour
	cfg.ModuleRules = o.modules
	cfg.Exclude = o.exclude
	if o.asOf != "" {
		t, err := time.Parse(time.DateOnly, o.asOf)
		if err != nil {
			return cfg, fmt.Errorf("--as-of must be YYYY-MM-DD: %w", err)
		}
		cfg.AsOf = t.Add(24*time.Hour - time.Second) // end of that day
	}
	for _, e := range o.leaving {
		cfg.Statuses = append(cfg.Statuses, dto.PersonStatus{Email: e, Status: dto.StatusLeaving})
	}
	for _, e := range o.left {
		cfg.Statuses = append(cfg.Statuses, dto.PersonStatus{Email: e, Status: dto.StatusLeft})
	}
	return cfg, nil
}

// label describes what was analysed: "main @ 1a2b3c4".
func (t target) label() string {
	short := t.head
	if len(short) > 7 {
		short = short[:7]
	}
	if t.rev == "HEAD" {
		if b, err := t.repo.DefaultBranch(context.Background()); err == nil {
			return b + " @ " + short
		}
		return short
	}
	return t.rev + " @ " + short
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func argOr(pos []string, i int, def string) string {
	if len(pos) > i {
		return pos[i]
	}
	return def
}

// parseCommand parses a command's flags and positional arguments in any
// order ("barge scan ~/code --top 5"), which the flag package alone does
// not. On -h it prints the command's help; done is true when the command
// should return code right away.
func parseCommand(fs *flag.FlagSet, name string, args []string, stdout, stderr io.Writer) (pos []string, code int, done bool) {
	if placeholderTyped(stderr, name, args) {
		return nil, exitUsageErr, true
	}
	for {
		if code, done := parseOrHelp(fs, name, args, stdout, stderr); done {
			return nil, code, true
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, exitOK, false
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), exitOK, false
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return true // e.g. CI logs that render colours
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0 && enableColor(f)
}

// placeholderTyped catches a placeholder from the help typed literally,
// as in "--module GLOB 'internal/*'", and explains it.
func placeholderTyped(stderr io.Writer, cmd string, args []string) bool {
	for i, a := range args {
		if i == 0 || !strings.HasPrefix(args[i-1], "-") {
			continue
		}
		flagName := strings.TrimLeft(args[i-1], "-")
		if ph, ok := argNames[flagName]; ok && a == ph {
			example := "--" + flagName + " <your value>"
			if i+1 < len(args) {
				example = "--" + flagName + " " + shellQuote(args[i+1])
			}
			fmt.Fprintf(stderr, "barge: %s in the help stands for your own value; leave the word %s out.\n", ph, ph)
			fmt.Fprintf(stderr, "       Try: barge %s %s\n", cmd, example)
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	if strings.ContainsAny(s, "*?[] ") {
		return "'" + s + "'"
	}
	return s
}

// extraArgs explains arguments a command did not expect, usually a second
// value after a flag that takes one ("--only a b").
func extraArgs(stderr io.Writer, cmd string, extra []string) {
	fmt.Fprintf(stderr, "barge: unexpected %s: %s takes one path.\n", strings.Join(quoteAll(extra), " "), cmd)
	fmt.Fprintln(stderr, "       A flag takes one value: repeat it (--only a --only b) or use commas (--only a,b).")
}

func quoteAll(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = fmt.Sprintf("%q", v)
	}
	return out
}

// listFlag collects a repeatable string flag; commas also separate values.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			*l = append(*l, s)
		}
	}
	return nil
}
