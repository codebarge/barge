package service

import (
	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// Filter narrows a report to the modules matching any of patterns: a module
// matches when its path, or a directory above it, matches a pattern, so
// "pkg/compose" also keeps "pkg/compose/transform". Patterns are globs as in
// Config.ModuleRules ("internal/*", "cmd/compose", or a bare name such as
// "service" for that directory name anywhere).
//
// The summary is recounted for the kept modules and actions about other
// modules are dropped. Groups above kept modules stay, and so does the root,
// so the repository bus factor is still that of the whole repository.
func Filter(rep dto.Report, patterns []string) dto.Report {
	m := newMatcher(patterns)
	if len(m.base) == 0 && len(m.full) == 0 {
		return rep
	}
	matches := func(p string) bool {
		if m.match(p) {
			return true
		}
		for _, a := range ancestors(p) {
			if a != "." && m.match(a) {
				return true
			}
		}
		return false
	}

	kept := map[string]bool{}
	keepGroup := map[string]bool{".": true}
	owners := map[string]bool{}
	for _, mod := range rep.Modules {
		if mod.Kind != dto.KindModule || !matches(mod.Path) {
			continue
		}
		kept[mod.Path] = true
		for _, a := range ancestors(mod.Path) {
			keepGroup[a] = true
		}
		for _, o := range mod.Owners {
			owners[o.PersonID] = true
		}
	}

	out := rep
	out.Modules = nil
	out.Summary = dto.Summary{}
	for _, mod := range rep.Modules {
		switch {
		case mod.Kind == dto.KindModule && kept[mod.Path]:
			out.Modules = append(out.Modules, mod)
			countSummary(&out.Summary, mod)
		case mod.Kind == dto.KindGroup && (keepGroup[mod.Path] || matches(mod.Path)):
			out.Modules = append(out.Modules, mod)
		}
	}
	out.Actions = nil
	for _, a := range rep.Actions {
		if (a.Module != "" && kept[a.Module]) || (a.Module == "" && owners[a.OwnerID]) {
			out.Actions = append(out.Actions, a)
		}
	}
	return out
}

// UnusedRules returns the module rules that formed no module in rep: a
// typo, or a directory this repository does not have.
func UnusedRules(rep dto.Report, rules []string) []string {
	var unused []string
	for _, r := range rules {
		m := newMatcher([]string{r})
		if len(m.base) == 0 && len(m.full) == 0 {
			continue
		}
		used := false
		for _, mod := range rep.Modules {
			if mod.Kind == dto.KindModule && m.match(mod.Path) {
				used = true
				break
			}
		}
		if !used {
			unused = append(unused, r)
		}
	}
	return unused
}
