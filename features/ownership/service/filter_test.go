package service

import (
	"testing"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

func filterReport() dto.Report {
	mod := func(p string, r dto.Risk, bf int) dto.Module {
		return dto.Module{Path: p, Parent: parentOf(p), Kind: dto.KindModule, Risk: r, BusFactor: bf,
			Owners: []dto.Owner{{PersonID: "p-" + p}}}
	}
	group := func(p string) dto.Module { return dto.Module{Path: p, Parent: parentOf(p), Kind: dto.KindGroup} }
	return dto.Report{
		Modules: []dto.Module{
			mod("pkg/compose", dto.RiskHigh, 1),
			mod("pkg/compose/transform", dto.RiskHigh, 1),
			mod("pkg/api", dto.RiskMedium, 2),
			mod("cmd/compose", dto.RiskMedium, 2),
			group("."), group("pkg"), group("cmd"),
		},
		Summary: dto.Summary{Modules: 4},
		Actions: []dto.Action{
			{Kind: dto.ActionSecondOwner, Module: "pkg/compose"},
			{Kind: dto.ActionSecondOwner, Module: "cmd/compose"},
			{Kind: dto.ActionHandover, OwnerID: "p-pkg/compose"},
			{Kind: dto.ActionHandover, OwnerID: "p-cmd/compose"},
		},
	}
}

func TestFilter(t *testing.T) {
	rep := Filter(filterReport(), []string{"pkg/compose"})
	var paths []string
	for _, m := range rep.Modules {
		paths = append(paths, m.Path)
	}
	want := []string{"pkg/compose", "pkg/compose/transform", ".", "pkg"}
	if len(paths) != len(want) {
		t.Fatalf("modules %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("modules %v, want %v", paths, want)
		}
	}
	if rep.Summary.Modules != 2 || rep.Summary.High != 2 || rep.Summary.SingleOwner != 2 {
		t.Errorf("summary %+v", rep.Summary)
	}
	if len(rep.Actions) != 2 || rep.Actions[0].Module != "pkg/compose" || rep.Actions[1].OwnerID != "p-pkg/compose" {
		t.Errorf("actions %+v", rep.Actions)
	}

	// Globs and bare names work as in module rules.
	if got := Filter(filterReport(), []string{"pkg/*"}).Summary.Modules; got != 3 {
		t.Errorf("pkg/*: %d modules, want 3", got)
	}
	if got := Filter(filterReport(), []string{"compose"}).Summary.Modules; got != 3 {
		t.Errorf("compose: %d modules, want 3 (both compose dirs and what is under them)", got)
	}
	if got := Filter(filterReport(), []string{"nothing/here"}).Summary.Modules; got != 0 {
		t.Errorf("no match: %d modules", got)
	}
	if got := Filter(filterReport(), nil).Summary.Modules; got != 4 {
		t.Errorf("no patterns should keep everything: %d", got)
	}
}

func TestUnusedRules(t *testing.T) {
	got := UnusedRules(filterReport(), []string{"cmd/compose", "internal/features/*", "pkg/*"})
	if len(got) != 1 || got[0] != "internal/features/*" {
		t.Errorf("unused = %v", got)
	}
}
