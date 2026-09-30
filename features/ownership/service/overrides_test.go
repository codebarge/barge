package service

import (
	"errors"
	"testing"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

func TestOwnerOverrides(t *testing.T) {
	const svc = "internal/billing/"
	fs := files(svc+"invoice.go", "internal/users/user.go")
	var commits []dto.Commit
	for i := 0; i < 10; i++ {
		commits = append(commits, commit("o"+string(rune('a'+i)), "Oleg", "oleg@x.io", daysAgo(5+i*7),
			change(svc+"invoice.go", 40)))
	}
	commits = append(commits,
		// Max ran a formatter over billing once: not knowledge.
		commit("m1", "Max", "max@x.io", daysAgo(2), change(svc+"invoice.go", 300)),
		commit("a1", "Anna", "anna@x.io", daysAgo(3), change("internal/users/user.go", 20)),
	)
	sortNewestFirst(commits)
	cfg := DefaultConfig()
	cfg.AsOf = base

	before := module(t, analyze(t, cfg, fs, commits...), dto.KindModule, "internal/billing")
	if share(before, "max@x.io") == 0 {
		t.Fatal("setup: Max should have a share before the override")
	}

	cfg.Owners = []dto.OwnerOverride{
		{Module: "./internal/billing/", Email: "MAX@x.io", Knows: false},
		{Module: "internal/billing", Email: "anna@x.io", Knows: true},  // reviews every billing change
		{Module: "internal/billing", Email: "ghost@x.io", Knows: true}, // never committed: ignored
	}
	rep := analyze(t, cfg, fs, commits...)
	m := module(t, rep, dto.KindModule, "internal/billing")
	if s := share(m, "max@x.io"); s != 0 {
		t.Errorf("Max is excluded, got share %.2f", s)
	}
	if s := share(m, "oleg@x.io"); s < 0.999 {
		t.Errorf("Oleg holds all remaining history, got %.2f", s)
	}
	var anna *dto.Owner
	for i := range m.Owners {
		if m.Owners[i].PersonID == "anna@x.io" {
			anna = &m.Owners[i]
		}
	}
	if anna == nil || !anna.Confirmed || anna.Share != 0 {
		t.Fatalf("Anna must be listed as a confirmed owner with no history share: %+v", m.Owners)
	}
	if m.BusFactor != 2 || m.Risk != dto.RiskMedium {
		t.Errorf("Oleg plus confirmed Anna: want bf 2 / medium, got bf %d / %s", m.BusFactor, m.Risk)
	}
	for _, a := range rep.Actions {
		if a.Module == "internal/billing" && a.Kind == dto.ActionSecondOwner {
			t.Errorf("billing already has a second owner, got action %+v", a)
		}
	}

	// A confirmed owner who is leaving does not keep the module going.
	cfg.Statuses = []dto.PersonStatus{{Email: "anna@x.io", Status: dto.StatusLeaving}}
	m = module(t, analyze(t, cfg, fs, commits...), dto.KindModule, "internal/billing")
	if m.BusFactor != 1 {
		t.Errorf("Anna leaving: want bf 1, got %d", m.BusFactor)
	}
}

func TestFileConfigOwners(t *testing.T) {
	fc, err := ParseFileConfig([]byte(`{"owners": [{"module": "internal/billing", "email": "anna@x.io", "knows": true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Owners = []dto.OwnerOverride{{Module: "internal/billing", Email: "anna@x.io", Knows: false}}
	fc.Apply(&cfg)
	if len(cfg.Owners) != 2 || !cfg.Owners[0].Knows || cfg.Owners[1].Knows {
		t.Errorf("file overrides go first so the caller's win: %+v", cfg.Owners)
	}
	for _, bad := range []string{
		`{"owners": [{"module": "a", "email": "b@x.io"}]}`,
		`{"owners": [{"email": "b@x.io", "knows": true}]}`,
		`{"owners": [{"module": "a", "knows": false}]}`,
	} {
		if _, err := ParseFileConfig([]byte(bad)); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: want ErrInvalidConfig, got %v", bad, err)
		}
	}
}

func TestCleanModulePath(t *testing.T) {
	for in, want := range map[string]string{
		"": ".", ".": ".", "./": ".", "/internal/core/": "internal/core", "./internal": "internal", ` internal\core `: "internal/core",
	} {
		if got := CleanModulePath(in); got != want {
			t.Errorf("CleanModulePath(%q) = %q, want %q", in, got, want)
		}
	}
}
