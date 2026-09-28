package service

import (
	"math"
	"testing"
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

var base = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return base.Add(-time.Duration(n) * day) }

func commit(hash, author, email string, t time.Time, changes ...dto.Change) dto.Commit {
	return dto.Commit{Hash: hash, Author: author, Email: email, Time: t, Changes: changes}
}

func change(p string, lines int) dto.Change { return dto.Change{Path: p, Added: lines} }

func files(paths ...string) []dto.File {
	out := make([]dto.File, len(paths))
	for i, p := range paths {
		out[i] = dto.File{Path: p, Size: 100}
	}
	return out
}

func analyze(t *testing.T, cfg Config, fs []dto.File, commits ...dto.Commit) dto.Report {
	t.Helper()
	a := NewAnalyzer(cfg, fs, nil)
	for _, c := range commits {
		a.Add(c)
	}
	return a.Report()
}

func module(t *testing.T, r dto.Report, kind, p string) dto.Module {
	t.Helper()
	for _, m := range r.Modules {
		if m.Path == p && m.Kind == kind {
			return m
		}
	}
	t.Fatalf("%s %q not in report: %+v", kind, p, r.Modules)
	return dto.Module{}
}

func share(m dto.Module, personID string) float64 {
	for _, o := range m.Owners {
		if o.PersonID == personID {
			return o.Share
		}
	}
	return 0
}

func TestBusFactor(t *testing.T) {
	cases := []struct {
		shares []float64
		want   int
	}{
		{nil, 0},
		{[]float64{1}, 1},
		{[]float64{0.5, 0.5}, 2},
		{[]float64{0.86, 0.09, 0.05}, 1},
		{[]float64{0.4, 0.3, 0.3}, 2},
		{[]float64{0.25, 0.25, 0.25, 0.25}, 3},
		{[]float64{0.3}, 0}, // the rest of the knowledge already left
		{[]float64{0.1, 0.45}, 1},
	}
	for _, tc := range cases {
		if got := BusFactor(tc.shares, 0.5); got != tc.want {
			t.Errorf("BusFactor(%v) = %d, want %d", tc.shares, got, tc.want)
		}
	}
	// With the default threshold a near-even split is two owners, not one.
	th := DefaultConfig().KnowledgeThreshold
	for _, tc := range []struct {
		shares []float64
		want   int
	}{
		{[]float64{0.52, 0.48}, 2},
		{[]float64{0.7, 0.3}, 1},
		{[]float64{0.86, 0.09, 0.05}, 1},
	} {
		if got := BusFactor(tc.shares, th); got != tc.want {
			t.Errorf("default threshold: BusFactor(%v) = %d, want %d", tc.shares, got, tc.want)
		}
	}
}

func TestMatcher(t *testing.T) {
	m := newMatcher(append(append([]string{}, DefaultExclude...), "docs/", "internal/gen/**"))
	excluded := []string{
		"vendor/github.com/x/y.go", "a/b/vendor/z.go", "web/node_modules/react/index.js",
		"go.sum", "sub/Cargo.lock", "api/v1/user.pb.go", "web/dist/app.js", "x.min.js",
		"docs/index.md", "internal/gen/deep/file.go",
		"tests/fixtures/env/docker-compose.yml", "pkg/parser/testdata/in.txt",
	}
	kept := []string{
		"main.go", "internal/core/git/log.go", "vendors.go", "docs.go", "internal/generator/x.go",
	}
	for _, p := range excluded {
		if !m.match(p) {
			t.Errorf("%q should be excluded", p)
		}
	}
	for _, p := range kept {
		if m.match(p) {
			t.Errorf("%q should be kept", p)
		}
	}
}

func TestModuleOf(t *testing.T) {
	plain := newModuler(nil, 0)
	if got := plain.moduleOf("internal/features/users/service/user.go"); got != "internal/features/users/service" {
		t.Errorf("leaf dir: %q", got)
	}
	if got := plain.moduleOf("main.go"); got != "." {
		t.Errorf("root: %q", got)
	}
	ruled := newModuler([]string{"internal/features/*"}, 0)
	if got := ruled.moduleOf("internal/features/users/service/user.go"); got != "internal/features/users" {
		t.Errorf("rule: %q", got)
	}
	if got := ruled.moduleOf("internal/core/git/log.go"); got != "internal/core/git" {
		t.Errorf("no rule falls back to dir: %q", got)
	}
	deep := newModuler(nil, 2)
	if got := deep.moduleOf("internal/core/git/log.go"); got != "internal/core" {
		t.Errorf("depth: %q", got)
	}
}

func TestIdentities(t *testing.T) {
	ids := newIdentities()
	ids.observe("Oleg Koval", "oleg@dispatcher.dev", daysAgo(10))
	ids.observe("Oleg Koval", "O.Koval@Gmail.com", daysAgo(5))
	ids.observe("oleg koval", "12345+oleg-k@users.noreply.github.com", daysAgo(1))
	ids.observe("root", "root@server1", daysAgo(3))
	ids.observe("root", "root@server2", daysAgo(3))
	people, _ := ids.people()

	var oleg *dto.Person
	for i := range people {
		if people[i].Name == "Oleg Koval" {
			oleg = &people[i]
		}
	}
	if oleg == nil || len(oleg.Emails) != 3 {
		t.Fatalf("aliases not merged: %+v", people)
	}
	if oleg.Commits != 3 || !oleg.LastCommit.Equal(daysAgo(1)) {
		t.Errorf("merged stats wrong: %+v", oleg)
	}
	if len(people) != 3 {
		t.Errorf("generic name 'root' must not merge different emails: %+v", people)
	}
}

func TestNameKeyMergesSpellings(t *testing.T) {
	same := [][]string{
		{"Daniil Kuharenko", "DaniilKukharenko", "Danil Kukharenko", "Даниил Кухаренко", "daniil.kukharenko"},
		{"Oleksandr Shevchenko", "Olexandr Shevchenko", "Олександр Шевченко"},
		{"Yurii Kovalenko", "Iurii Kovalenko", "Juri Kovalenko"},
	}
	for _, group := range same {
		want := nameKey(group[0])
		if want == "" {
			t.Fatalf("empty key for %q", group[0])
		}
		for _, n := range group[1:] {
			if got := nameKey(n); got != want {
				t.Errorf("nameKey(%q) = %q, want %q (same as %q)", n, got, want, group[0])
			}
		}
	}
	if nameKey("Anna") != "" || nameKey("Ivan P") != "" {
		t.Error("short names must not be used for merging")
	}
	if nameKey("Anna Melnyk") == nameKey("Anna Shevchenko") {
		t.Error("different surnames merged")
	}

	ids := newIdentities()
	ids.observe("Daniil Kuharenko", "ladanielworkua@gmail.com", daysAgo(2))
	ids.observe("DaniilKukharenko", "d.kukharenko@users.noreply.gitlab.com", daysAgo(1))
	if people, _ := ids.people(); len(people) != 1 {
		t.Errorf("spelling variants not merged: %+v", people)
	}
}

func TestRootGroupForFlatRepository(t *testing.T) {
	r := analyze(t, DefaultConfig(), files("main.go", "go.mod"),
		commit("1", "Ann", "ann@x.io", daysAgo(1), change("main.go", 10), change("go.mod", 3)))
	root := module(t, r, dto.KindGroup, ".")
	if root.BusFactor != 1 || root.Files != 2 {
		t.Errorf("root group of a flat repository: %+v", root)
	}
}

func TestClassifyAuthor(t *testing.T) {
	cases := []struct {
		name, email string
		want        authorKind
	}{
		{"Anna", "anna@x.io", kindHuman},
		{"dependabot[bot]", "49699333+dependabot[bot]@users.noreply.github.com", kindBot},
		{"renovate-bot", "bot@renovateapp.com", kindBot},
		{"Copilot", "198982749+Copilot@users.noreply.github.com", kindAgent},
		{"devin-ai-integration[bot]", "158243242+devin-ai-integration[bot]@users.noreply.github.com", kindAgent},
		{"Claude", "noreply@anthropic.com", kindAgent},
	}
	for _, tc := range cases {
		if got := classifyAuthor(tc.name, tc.email); got != tc.want {
			t.Errorf("classifyAuthor(%q, %q) = %v, want %v", tc.name, tc.email, got, tc.want)
		}
	}
}

func TestIsAIAssisted(t *testing.T) {
	yes := []dto.Commit{
		{Message: "fix\n\nCo-Authored-By: Claude Opus <noreply@anthropic.com>"},
		{Message: "feat\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)"},
		{Message: "x\n\nCo-authored-by: Copilot <175728472+Copilot@users.noreply.github.com>"},
		{Author: "Anna (aider)", Message: "refactor"},
	}
	no := []dto.Commit{
		{Message: "fix cursor position in editor"},
		{Message: "x\n\nCo-authored-by: Anna <anna@x.io>"},
	}
	for _, c := range yes {
		if !isAIAssisted(c) {
			t.Errorf("should be AI-assisted: %q", c.Message)
		}
	}
	for _, c := range no {
		if isAIAssisted(c) {
			t.Errorf("should not be AI-assisted: %q", c.Message)
		}
	}
}

// The scenario from the product description: Oleg wrote most of the
// payments service and is leaving.
func TestOlegIsLeaving(t *testing.T) {
	const svc = "internal/features/payments/service/"
	fs := files(svc+"capture.go", svc+"webhook.go", "internal/features/users/service/user.go")
	var commits []dto.Commit
	for i := 0; i < 20; i++ {
		commits = append(commits, commit("o"+string(rune('a'+i)), "Oleg Koval", "oleg@x.io", daysAgo(10+i*5),
			change(svc+"capture.go", 40), change(svc+"webhook.go", 30)))
	}
	commits = append(commits,
		commit("a1", "Anna", "anna@x.io", daysAgo(14), change(svc+"capture.go", 5)),
		commit("i1", "Irina", "irina@x.io", daysAgo(120), change(svc+"webhook.go", 4)),
		commit("u1", "Anna", "anna@x.io", daysAgo(3), change("internal/features/users/service/user.go", 50)),
		commit("u2", "Irina", "irina@x.io", daysAgo(4), change("internal/features/users/service/user.go", 50)),
	)
	sortNewestFirst(commits)

	cfg := DefaultConfig()
	cfg.AsOf = base
	before := analyze(t, cfg, fs, commits...)
	m := module(t, before, dto.KindModule, "internal/features/payments/service")
	if m.BusFactor != 1 || m.Risk != dto.RiskHigh || !hasReason(m, dto.ReasonSingleOwner) {
		t.Fatalf("before departure: bf=%d risk=%s reasons=%v", m.BusFactor, m.Risk, m.Reasons)
	}
	if s := share(m, "oleg@x.io"); s < 0.8 {
		t.Errorf("Oleg should hold most of the module, got %.2f", s)
	}

	cfg.Statuses = []dto.PersonStatus{{Email: "OLEG@x.io", Status: dto.StatusLeaving, Date: base.Add(14 * day)}}
	after := analyze(t, cfg, fs, commits...)
	m = module(t, after, dto.KindModule, "internal/features/payments/service")
	if m.Risk != dto.RiskCritical || !hasReason(m, dto.ReasonOwnerLeaving) || m.BusFactor != 0 {
		t.Errorf("after announcement: bf=%d risk=%s reasons=%v", m.BusFactor, m.Risk, m.Reasons)
	}
	users := module(t, after, dto.KindModule, "internal/features/users/service")
	if users.BusFactor != 2 || users.Risk != dto.RiskMedium {
		t.Errorf("users module: bf=%d risk=%s", users.BusFactor, users.Risk)
	}
	if after.Modules[0].Path != "internal/features/payments/service" {
		t.Errorf("worst module must come first, got %s", after.Modules[0].Path)
	}
}

func TestDecayFavoursRecentWork(t *testing.T) {
	fs := files("svc/a.go")
	cfg := DefaultConfig()
	cfg.AsOf = base
	r := analyze(t, cfg, fs,
		commit("new", "Bob", "bob@x.io", daysAgo(1), change("svc/a.go", 100)),
		commit("old", "Ann", "ann@x.io", daysAgo(720), change("svc/a.go", 100)),
	)
	m := module(t, r, dto.KindModule, "svc")
	ann, bob := share(m, "ann@x.io"), share(m, "bob@x.io")
	// Four half-lives apart: Ann keeps about 1/16 of Bob's weight.
	if math.Abs(bob/ann-16) > 0.5 {
		t.Errorf("decay ratio bob/ann = %.2f, want ~16", bob/ann)
	}
	// Ann has not committed for two years: inactive, so she does not count.
	if m.BusFactor != 1 {
		t.Errorf("bus factor %d, want 1", m.BusFactor)
	}
}

func TestRenamesDeletesAndExcludes(t *testing.T) {
	fs := files("pkg/b.go", "go.sum", "vendor/lib/x.go")
	gen := map[string]bool{}
	a := NewAnalyzer(DefaultConfig(), append(fs, dto.File{Path: "pkg/api.pb.go", Size: 10}, dto.File{Path: "pkg/z_gen.txt", Size: 1}), gen)
	gen["pkg/z_gen.txt"] = true
	for _, c := range []dto.Commit{
		// newest first: the move to pkg/, the rename, the original file
		commit("3", "Cid", "cid@x.io", daysAgo(1), dto.Change{Path: "pkg/b.go", OldPath: "src/b.go"}),
		commit("2", "Bob", "bob@x.io", daysAgo(2), dto.Change{Path: "src/b.go", OldPath: "src/a.go", Added: 10}),
		commit("1", "Ann", "ann@x.io", daysAgo(3), change("src/a.go", 100), change("gone.go", 500),
			change("go.sum", 900), change("vendor/lib/x.go", 900), change("pkg/api.pb.go", 900), change("pkg/z_gen.txt", 5)),
	} {
		a.Add(c)
	}
	r := a.Report()
	if len(r.Modules) == 0 {
		t.Fatal("no modules")
	}
	m := module(t, r, dto.KindModule, "pkg")
	if share(m, "ann@x.io") == 0 || share(m, "bob@x.io") == 0 {
		t.Errorf("history before renames must follow the file: %+v", m.Owners)
	}
	if share(m, "ann@x.io") < share(m, "bob@x.io") {
		t.Errorf("Ann wrote the file and should lead: %+v", m.Owners)
	}
	if m.Files != 1 {
		t.Errorf("excluded files counted: files=%d", m.Files)
	}
	for _, mod := range r.Modules {
		if mod.Kind == dto.KindModule && mod.Path != "pkg" {
			t.Errorf("unexpected module %q (deleted or excluded files leaked)", mod.Path)
		}
	}
}

func TestMassChangeAndBots(t *testing.T) {
	var fs []dto.File
	var many []dto.Change
	for i := 0; i < 300; i++ {
		p := "lib/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".go"
		fs = append(fs, dto.File{Path: p, Size: 1})
		many = append(many, change(p, 2))
	}
	cfg := DefaultConfig()
	r := analyze(t, cfg, fs,
		commit("fmt", "Fmt Bot Person", "fmt@x.io", daysAgo(1), many...),
		commit("dep", "dependabot[bot]", "dependabot[bot]@users.noreply.github.com", daysAgo(1), many...),
		commit("own", "Ann", "ann@x.io", daysAgo(2), change("lib/faa.go", 2)),
	)
	m := module(t, r, dto.KindModule, "lib")
	for _, o := range m.Owners {
		if o.Name == "dependabot[bot]" {
			t.Errorf("bots must not own code")
		}
	}
	if share(m, "ann@x.io") == 0 {
		t.Errorf("Ann lost her credit to a mass change: %+v", m.Owners)
	}
}

func TestAIAgentsAndAssistedCommits(t *testing.T) {
	fs := files("mw/a.go", "svc/a.go")
	cfg := DefaultConfig()
	r := analyze(t, cfg, fs,
		commit("1", "Copilot", "198982749+Copilot@users.noreply.github.com", daysAgo(1), change("mw/a.go", 300)),
		dto.Commit{Hash: "2", Author: "Ann", Email: "ann@x.io", Time: daysAgo(2),
			Message: "feat\n\nCo-authored-by: Claude <noreply@anthropic.com>", Changes: []dto.Change{change("mw/a.go", 10)}},
		commit("3", "Ann", "ann@x.io", daysAgo(3), change("svc/a.go", 10)),
	)
	mw := module(t, r, dto.KindModule, "mw")
	if mw.AIShare < 0.99 {
		t.Errorf("AI share %.2f, want ~1", mw.AIShare)
	}
	if !hasReason(mw, dto.ReasonAIHeavy) {
		t.Errorf("ai_heavy missing: %v", mw.Reasons)
	}
	if len(mw.Owners) != 1 || mw.Owners[0].PersonID != "ann@x.io" {
		t.Errorf("only the human reviewer can own AI code: %+v", mw.Owners)
	}
	for _, p := range r.People {
		if p.Name == "Copilot" {
			t.Errorf("agents are not people: %+v", p)
		}
	}
}

func TestGroupsAggregateChildren(t *testing.T) {
	fs := files("internal/core/git/log.go", "internal/core/logger/l.go", "internal/core/core.go", "main.go")
	r := analyze(t, DefaultConfig(), fs,
		commit("1", "Ann", "ann@x.io", daysAgo(1), change("internal/core/git/log.go", 10)),
		commit("2", "Bob", "bob@x.io", daysAgo(2), change("internal/core/logger/l.go", 10)),
		commit("3", "Cid", "cid@x.io", daysAgo(3), change("internal/core/core.go", 10), change("main.go", 1)),
	)
	core := module(t, r, dto.KindGroup, "internal/core")
	if len(core.Owners) != 3 || core.Files != 3 {
		t.Errorf("internal/core group should aggregate 3 people and 3 files: %+v", core)
	}
	root := module(t, r, dto.KindGroup, ".")
	if root.Files != 4 || root.Parent != "" {
		t.Errorf("root group: %+v", root)
	}
	for _, m := range r.Modules {
		if m.Kind == dto.KindGroup && m.Path == "internal/core/git" {
			t.Error("a leaf module must not repeat as a group")
		}
	}
	if r.Summary.Modules != 4 {
		t.Errorf("summary counts modules only: %+v", r.Summary)
	}
}

func sortNewestFirst(cs []dto.Commit) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].Time.After(cs[j-1].Time); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

func TestActions(t *testing.T) {
	fs := files("pay/a.go", "users/a.go", "old/a.go", "mw/a.go")
	cfg := DefaultConfig()
	cfg.AsOf = base
	commits := []dto.Commit{
		commit("1", "Copilot", "198982749+Copilot@users.noreply.github.com", daysAgo(1), change("mw/a.go", 300)),
		commit("2", "Oleg Koval", "oleg@x.io", daysAgo(2), change("pay/a.go", 100)),
		commit("3", "Anna Melnyk", "anna@x.io", daysAgo(3), change("users/a.go", 50), change("pay/a.go", 2)),
		commit("4", "Irina Savchuk", "irina@x.io", daysAgo(4), change("users/a.go", 50), change("mw/a.go", 5)),
		commit("5", "Dmitry Lysenko", "dmitry@x.io", daysAgo(400), change("old/a.go", 80)),
	}
	r := analyze(t, cfg, fs, commits...)
	kinds := map[string]dto.Action{}
	for _, a := range r.Actions {
		kinds[a.Kind+":"+a.Module] = a
	}
	if a, ok := kinds[dto.ActionSecondOwner+":pay"]; !ok || a.OwnerName != "Oleg Koval" || a.CandidateName != "Anna Melnyk" {
		t.Errorf("pay: want Anna as second owner next to Oleg: %+v", r.Actions)
	}
	if a, ok := kinds[dto.ActionAssignOwner+":old"]; !ok || a.OwnerName != "Dmitry Lysenko" || a.CandidateName == "" {
		t.Errorf("old: want an owner suggested for Dmitry's module: %+v", a)
	}
	if _, ok := kinds[dto.ActionReviewAI+":mw"]; !ok {
		t.Errorf("mw: want an AI review: %+v", r.Actions)
	}
	if r.Actions[0].Kind != dto.ActionAssignOwner {
		t.Errorf("orphaned modules come first: %+v", r.Actions[0])
	}

	cfg.Statuses = []dto.PersonStatus{{Email: "oleg@x.io", Status: dto.StatusLeaving}}
	r = analyze(t, cfg, fs, commits...)
	if a := r.Actions[0]; a.Kind != dto.ActionHandover || a.OwnerEmail != "oleg@x.io" || a.Modules != 1 || a.Orphaned != 1 {
		t.Errorf("leaving owner: want a handover first: %+v", r.Actions)
	}
	for _, a := range r.Actions {
		if a.Module == "pay" && a.Kind != dto.ActionHandover {
			t.Errorf("pay is covered by the handover, not %s", a.Kind)
		}
	}
}
