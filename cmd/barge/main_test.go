package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// newRepo creates a small repository where Oleg owns payments alone and
// Anna and Irina share users.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	commit := func(name, email, date, file, content string) {
		t.Helper()
		p := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(content)
		_ = f.Close()
		git(nil, "add", "-A")
		env := []string{
			"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_AUTHOR_DATE=" + date,
			"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_COMMITTER_DATE=" + date,
		}
		git(env, "commit", "-q", "-m", "change "+file)
	}
	git(nil, "init", "-q", "-b", "main")
	for i, d := range []string{"2026-06-01", "2026-07-01", "2026-08-01", "2026-09-01"} {
		commit("Oleg Koval", "oleg@x.io", d+"T10:00:00Z", "payments/service/capture.go", strings.Repeat("line\n", 20+i))
	}
	commit("Anna", "anna@x.io", "2026-09-02T10:00:00Z", "users/service/user.go", strings.Repeat("a\n", 30))
	commit("Irina", "irina@x.io", "2026-09-03T10:00:00Z", "users/service/user.go", strings.Repeat("b\n", 30))
	commit("Anna", "anna@x.io", "2026-09-04T10:00:00Z", "users/service/user.go", "c\n")
	return dir
}

func scanJSON(t *testing.T, args ...string) (dto.Report, int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), append([]string{"scan", "--json"}, args...), &out, &errb)
	var rep dto.Report
	if code != exitError && code != exitUsageErr {
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("bad JSON: %v\n%s", err, out.String())
		}
	}
	return rep, code, errb.String()
}

func find(rep dto.Report, p string) *dto.Module {
	for i, m := range rep.Modules {
		if m.Path == p && m.Kind == dto.KindModule {
			return &rep.Modules[i]
		}
	}
	return nil
}

func TestScanJSON(t *testing.T) {
	dir := newRepo(t)
	rep, code, stderr := scanJSON(t, dir)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	pay := find(rep, "payments/service")
	if pay == nil || pay.BusFactor != 1 || pay.Risk != dto.RiskHigh {
		t.Fatalf("payments: %+v", pay)
	}
	users := find(rep, "users/service")
	if users == nil || users.BusFactor != 2 {
		t.Fatalf("users: %+v", users)
	}

	rep, _, _ = scanJSON(t, "--leaving", "oleg@x.io", dir)
	if pay := find(rep, "payments/service"); pay.Risk != dto.RiskCritical {
		t.Errorf("after --leaving: %+v", pay)
	}
}

func TestScanFailOn(t *testing.T) {
	dir := newRepo(t)
	if _, code, _ := scanJSON(t, "--fail-on", "high", dir); code != exitRisk {
		t.Errorf("--fail-on high: exit %d, want %d", code, exitRisk)
	}
	if _, code, _ := scanJSON(t, "--fail-on", "critical", dir); code != exitOK {
		t.Errorf("--fail-on critical: exit %d, want %d", code, exitOK)
	}
}

func TestScanConfigFile(t *testing.T) {
	dir := newRepo(t)
	cfg := `{"modules": ["payments", "users"], "people": [{"email": "anna@x.io", "status": "left"}]}`
	if err := os.WriteFile(filepath.Join(dir, ".barge.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, code, stderr := scanJSON(t, dir)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	users := find(rep, "users")
	if users == nil || users.BusFactor != 1 {
		t.Fatalf("module rule or status ignored: %+v", users)
	}

	// Flags override the file.
	rep, _, _ = scanJSON(t, "--leaving", "anna@x.io", dir)
	for _, p := range rep.People {
		if p.ID == "anna@x.io" && p.Status != dto.StatusLeaving {
			t.Errorf("flag should win over file, got %s", p.Status)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, ".barge.json"), []byte(`{"modulez": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code, stderr := scanJSON(t, dir); code != exitUsageErr || !strings.Contains(stderr, "modulez") {
		t.Errorf("typo in config: exit %d, stderr %q", code, stderr)
	}
}

func TestScanText(t *testing.T) {
	dir := newRepo(t)
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"scan", "--no-color", dir}, &out, &errb); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	s := out.String()
	for _, want := range []string{"payments/service", "Oleg Koval 100%", "Repository bus factor", "What to do next", "Make Anna a required reviewer"} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "\x1b[") {
		t.Error("colour codes despite --no-color")
	}
}

func TestUsageErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"frobnicate"}, &out, &errb); code != exitUsageErr {
		t.Errorf("unknown command: exit %d", code)
	}
	if code := run(context.Background(), []string{"scan", t.TempDir()}, &out, &errb); code != exitError {
		t.Errorf("not a repo: exit %d", code)
	}
}

func TestHandoverCommand(t *testing.T) {
	dir := newRepo(t)
	// A revert in Oleg's module is worth a question.
	cmd := exec.Command("git", "revert", "--no-edit", "HEAD~3")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Oleg Koval", "GIT_AUTHOR_EMAIL=oleg@x.io", "GIT_AUTHOR_DATE=2026-09-05T10:00:00Z",
		"GIT_COMMITTER_NAME=Oleg Koval", "GIT_COMMITTER_EMAIL=oleg@x.io", "GIT_COMMITTER_DATE=2026-09-05T10:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git revert: %v\n%s", err, out)
	}

	var out, errb bytes.Buffer
	// Flags after positional arguments must work too.
	code := run(context.Background(), []string{"handover", "oleg@x.io", dir, "--max-modules", "5"}, &out, &errb)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	md := out.String()
	for _, want := range []string{
		"# Handover: Oleg Koval",
		"would have nobody who knows them",
		"## `payments/service`",
		"Files mostly written by Oleg",
		"`payments/service/capture.go`",
		"### Questions for Oleg",
		"- [ ] What is this module responsible for",
		"what went wrong, and what should someone know before trying again?",
		"Suggested successor: **Anna**",
		"### For Anna",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("handover lacks %q:\n%s", want, md)
		}
	}

	// Unknown person: a helpful error, not an empty checklist.
	out.Reset()
	errb.Reset()
	if code := run(context.Background(), []string{"handover", "nobody@x.io", dir}, &out, &errb); code != exitError ||
		!strings.Contains(errb.String(), "nobody committed as nobody@x.io") {
		t.Errorf("unknown person: exit %d, %q", code, errb.String())
	}

	// Written to a file.
	file := filepath.Join(t.TempDir(), "handover.md")
	if code := run(context.Background(), []string{"handover", "-o", file, "oleg@x.io", dir}, &out, &errb); code != exitOK {
		t.Fatalf("-o: exit %d", code)
	}
	if b, err := os.ReadFile(file); err != nil || !strings.Contains(string(b), "# Handover") {
		t.Errorf("-o file: %v", err)
	}
}

func TestScanSuggestsHandoverWhenLeaving(t *testing.T) {
	dir := newRepo(t)
	rep, code, stderr := scanJSON(t, "--leaving", "oleg@x.io", dir)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(rep.Actions) == 0 || rep.Actions[0].Kind != dto.ActionHandover || rep.Actions[0].Orphaned != 1 ||
		rep.Actions[0].OwnerEmail != "oleg@x.io" {
		t.Fatalf("first action should be Oleg's handover: %+v", rep.Actions)
	}
	var out, errb bytes.Buffer
	run(context.Background(), []string{"scan", "--no-color", "--leaving", "oleg@x.io", dir}, &out, &errb)
	if !strings.Contains(out.String(), "barge handover oleg@x.io") {
		t.Errorf("text output lacks the handover command:\n%s", out.String())
	}
}

func TestURLInsteadOfPath(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"scan", "https://gitlab.com/a/b"}, &out, &errb); code != exitError ||
		!strings.Contains(errb.String(), "clone it first") {
		t.Errorf("URL: exit %d, %q", code, errb.String())
	}
}

func TestHelpAndVersion(t *testing.T) {
	ctx := context.Background()
	call := func(args ...string) (string, string, int) {
		var out, errb bytes.Buffer
		code := run(ctx, args, &out, &errb)
		return out.String(), errb.String(), code
	}

	// "barge", "barge help" and "barge -h" show the main page, uncoloured in a buffer.
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		out, _, code := call(args...)
		if code != exitOK || !strings.Contains(out, "COMMANDS") || !strings.Contains(out, "handover") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
		if strings.Contains(out, "\x1b[") {
			t.Errorf("%v: colour codes outside a terminal", args)
		}
	}

	// Command pages, by "help <cmd>" and "<cmd> -h", list the command's flags.
	for _, args := range [][]string{{"help", "scan"}, {"scan", "-h"}, {"scan", "--help"}} {
		out, _, code := call(args...)
		if code != exitOK || !strings.Contains(out, "--fail-on LEVEL") || !strings.Contains(out, "--leaving EMAIL") ||
			!strings.Contains(out, "(default 25)") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	if out, _, _ := call("handover", "-h"); !strings.Contains(out, "-o FILE") || !strings.Contains(out, "barge handover [flags] <email> [path]") {
		t.Errorf("handover -h:\n%s", out)
	}

	// Typos get a suggestion; bad flags a pointer to the help.
	if _, errb, code := call("scna"); code != exitUsageErr || !strings.Contains(errb, `Did you mean "barge scan"?`) {
		t.Errorf("typo: exit %d, %q", code, errb)
	}
	if _, errb, code := call("help", "nope"); code != exitUsageErr || !strings.Contains(errb, "unknown command") {
		t.Errorf("help nope: exit %d, %q", code, errb)
	}
	if _, errb, code := call("scan", "--nope"); code != exitUsageErr || !strings.Contains(errb, `barge help scan`) {
		t.Errorf("bad flag: exit %d, %q", code, errb)
	}
	if _, errb, code := call("handover"); code != exitUsageErr || !strings.Contains(errb, "needs the person's e-mail") {
		t.Errorf("handover without e-mail: exit %d, %q", code, errb)
	}

	// Version: a screen, one line, and JSON.
	if out, _, code := call("version"); code != exitOK || !strings.Contains(out, "Barge") || !strings.Contains(out, "Apache 2.0") {
		t.Errorf("version: exit %d\n%s", code, out)
	}
	if out, _, _ := call("--version"); strings.Count(out, "\n") != 1 || !strings.HasPrefix(out, "barge ") {
		t.Errorf("--version: %q", out)
	}
	if out, _, _ := call("version", "--short"); strings.TrimSpace(out) == "" || strings.Contains(out, " ") {
		t.Errorf("version --short: %q", out)
	}
	var v map[string]any
	out, _, _ := call("version", "--json")
	if err := json.Unmarshal([]byte(out), &v); err != nil || v["version"] == "" || v["platform"] == "" {
		t.Errorf("version --json: %v %q", err, out)
	}
}

func TestHelpFitsTerminal(t *testing.T) {
	pages := []string{"help"}
	for _, c := range commands {
		pages = append(pages, "help "+c.name)
	}
	for _, p := range pages {
		var out, errb bytes.Buffer
		run(context.Background(), strings.Fields(p), &out, &errb)
		for _, line := range strings.Split(out.String(), "\n") {
			if n := len([]rune(line)); n > 80 {
				t.Errorf("%s: line of %d columns: %q", p, n, line)
			}
		}
	}
}

func TestOnlyAndUnusedModuleRules(t *testing.T) {
	dir := newRepo(t)
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"scan", "--no-color", "--only", "payments/*", dir}, &out, &errb)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Only modules under payments/*: 1 of") ||
		!strings.Contains(out.String(), "payments/service") || strings.Contains(out.String(), "users/") {
		t.Errorf("--only output:\n%s", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := run(context.Background(), []string{"scan", "--only", "nope", dir}, &out, &errb); code != exitUsageErr ||
		!strings.Contains(errb.String(), `no module matches --only "nope"`) {
		t.Errorf("--only nope: exit %d, %q", code, errb.String())
	}

	out.Reset()
	errb.Reset()
	if code := run(context.Background(), []string{"scan", "--module", "internal/features/*", dir}, &out, &errb); code != exitOK ||
		!strings.Contains(errb.String(), `module rule "internal/features/*" matches no directory`) {
		t.Errorf("unused rule: exit %d, %q", code, errb.String())
	}
}

func TestExtraArgumentsAreExplained(t *testing.T) {
	dir := newRepo(t)
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"scan", dir, "--module", "GLOB", "internal/features/*"}, &out, &errb); code != exitUsageErr ||
		!strings.Contains(errb.String(), "GLOB in the help stands for your own value") || !strings.Contains(errb.String(), "--module 'internal/features/*'") {
		t.Errorf("placeholder: exit %d, %q", code, errb.String())
	}
	errb.Reset()
	if code := run(context.Background(), []string{"scan", dir, "--only", "payments", "users"}, &out, &errb); code != exitUsageErr ||
		!strings.Contains(errb.String(), `unexpected "users"`) || !strings.Contains(errb.String(), "--only a,b") {
		t.Errorf("second value: exit %d, %q", code, errb.String())
	}
}

// The newest entry in CHANGELOG.md must be the version in the source, so a
// release cannot ship with one bumped and not the other.
func TestVersionMatchesChangelog(t *testing.T) {
	b, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "## v") {
			if got := strings.Fields(line)[1]; got != releaseVersion {
				t.Errorf("newest CHANGELOG.md entry is %s, releaseVersion is %s", got, releaseVersion)
			}
			return
		}
	}
	t.Error("CHANGELOG.md has no version entry")
}

func TestPseudoVersionsAreIgnored(t *testing.T) {
	for v, pseudo := range map[string]bool{
		"v0.0.0-20261006200248-c9bbbe3cb1f7":   true,
		"v0.3.1-0.20261006200248-c9bbbe3cb1f7": true,
		"v0.3.0":                               false,
		"v0.3.0-rc.1":                          false,
	} {
		if got := pseudoVersion.MatchString(v); got != pseudo {
			t.Errorf("%s: pseudo = %v, want %v", v, got, pseudo)
		}
	}
}
