package service

import (
	"path"
	"strings"
)

// matcher matches file paths against glob patterns.
//
//   - A pattern without "/" matches the base name: "*.pb.go", "go.sum".
//   - A pattern with "/" matches the whole path; "**" matches any number of
//     directories: "**/vendor/**", "internal/gen/**".
//   - A trailing "/" means everything beneath: "docs/" == "docs/**".
type matcher struct {
	base []string   // name-only patterns
	full [][]string // path patterns split into segments
}

func newMatcher(patterns []string) matcher {
	var m matcher
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, "/") {
			p += "**"
		}
		p = strings.TrimPrefix(p, "./")
		if !strings.Contains(p, "/") {
			m.base = append(m.base, p)
			continue
		}
		m.full = append(m.full, strings.Split(strings.TrimPrefix(p, "/"), "/"))
	}
	return m
}

func (m matcher) match(p string) bool {
	if len(m.base) > 0 {
		name := path.Base(p)
		for _, b := range m.base {
			if ok, _ := path.Match(b, name); ok {
				return true
			}
		}
	}
	if len(m.full) > 0 {
		segs := strings.Split(p, "/")
		for _, f := range m.full {
			if matchSegments(f, segs) {
				return true
			}
		}
	}
	return false
}

func matchSegments(pattern, segs []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			rest := pattern[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegments(rest, segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pattern[0], segs[0]); !ok {
			return false
		}
		pattern, segs = pattern[1:], segs[1:]
	}
	return len(segs) == 0
}

// moduler maps a file path to the module it belongs to.
type moduler struct {
	rules matcher
	depth int
	cache map[string]string
}

func newModuler(rules []string, depth int) *moduler {
	return &moduler{rules: newMatcher(rules), depth: depth, cache: make(map[string]string)}
}

// moduleOf returns the module path of file p:
//  1. the shallowest ancestor directory matching a module rule, else
//  2. the first depth directories when depth > 0, else
//  3. the directory that contains the file ("." for the root).
func (m *moduler) moduleOf(p string) string {
	dir := path.Dir(p)
	if mod, ok := m.cache[dir]; ok {
		return mod
	}
	mod := m.compute(dir)
	m.cache[dir] = mod
	return mod
}

func (m *moduler) compute(dir string) string {
	if dir == "." || dir == "/" {
		return "."
	}
	segs := strings.Split(dir, "/")
	if len(m.rules.base) > 0 || len(m.rules.full) > 0 {
		for i := 1; i <= len(segs); i++ {
			cand := strings.Join(segs[:i], "/")
			if m.rules.match(cand) {
				return cand
			}
		}
	}
	if m.depth > 0 && len(segs) > m.depth {
		return strings.Join(segs[:m.depth], "/")
	}
	return dir
}

// ancestors returns every directory above p, nearest first, ending with ".".
// ancestors("a/b/c") = ["a/b", "a", "."]; ancestors(".") = [].
func ancestors(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	var out []string
	for {
		p = path.Dir(p)
		out = append(out, p)
		if p == "." || p == "/" {
			return out
		}
	}
}

// parentOf returns the parent directory of a module path, "" for the root.
func parentOf(p string) string {
	if p == "." || p == "" {
		return ""
	}
	return path.Dir(p)
}
