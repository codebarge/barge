// Package git is a thin adapter over the git command line. It runs git
// through os/exec and parses its output; it knows nothing about modules,
// owners or risk. Shelling out to git is faster than any pure-Go
// implementation on large repositories and adds no dependencies.
package git

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Commit is one non-merge commit with its per-file line changes.
type Commit struct {
	Hash    string
	Author  string // mailmap-aware name (%aN)
	Email   string // mailmap-aware email (%aE)
	Time    time.Time
	Message string // full message, including trailers
	Changes []Change
}

// Change is one file touched by a commit, as reported by --numstat.
type Change struct {
	Path    string // path after the commit
	OldPath string // previous path when git detected a rename, else ""
	Added   int
	Removed int
	Binary  bool // numstat prints "-" for binary files
}

const (
	recordSep = '\x1e'
	fieldSep  = "\x1f"
)

// logFormat must stay in sync with parseRecord.
const logFormat = "%x1e%H%x1f%aN%x1f%aE%x1f%at%x1f%B%x1f"

// ParseLog reads the output of
//
//	git log --numstat --format=<logFormat>
//
// and calls fn for every commit, newest first (git's order). It stops at
// the first error returned by fn.
func ParseLog(r io.Reader, fn func(Commit) error) error {
	br := bufio.NewReaderSize(r, 1<<16)
	first := true
	for {
		rec, err := br.ReadString(recordSep)
		if len(rec) > 0 && rec[len(rec)-1] == recordSep {
			rec = rec[:len(rec)-1]
		}
		if first {
			// Everything before the first separator is empty.
			first = false
		} else if strings.TrimSpace(rec) != "" {
			c, perr := parseRecord(rec)
			if perr != nil {
				return perr
			}
			if ferr := fn(c); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read git log: %w", err)
		}
	}
}

func parseRecord(rec string) (Commit, error) {
	parts := strings.SplitN(rec, fieldSep, 6)
	if len(parts) < 5 {
		return Commit{}, fmt.Errorf("git log: malformed record %q", truncate(rec, 80))
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("git log: bad timestamp in %s: %w", parts[0], err)
	}
	c := Commit{
		Hash:    strings.TrimSpace(parts[0]),
		Author:  strings.TrimSpace(parts[1]),
		Email:   strings.TrimSpace(parts[2]),
		Time:    time.Unix(sec, 0).UTC(),
		Message: strings.TrimRight(parts[4], "\n"),
	}
	if len(parts) == 6 {
		for _, line := range strings.Split(parts[5], "\n") {
			if ch, ok := parseNumstat(line); ok {
				c.Changes = append(c.Changes, ch)
			}
		}
	}
	return c, nil
}

// parseNumstat parses "added<TAB>removed<TAB>path".
func parseNumstat(line string) (Change, bool) {
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return Change{}, false
	}
	f := strings.SplitN(line, "\t", 3)
	if len(f) != 3 {
		return Change{}, false
	}
	var ch Change
	if f[0] == "-" || f[1] == "-" {
		ch.Binary = true
	} else {
		a, err1 := strconv.Atoi(f[0])
		r, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			return Change{}, false
		}
		ch.Added, ch.Removed = a, r
	}
	p := unquote(f[2])
	if oldP, newP, ok := SplitRename(p); ok {
		ch.OldPath, ch.Path = oldP, newP
	} else {
		ch.Path = p
	}
	if ch.Path == "" {
		return Change{}, false
	}
	return ch, true
}

// SplitRename understands the two rename forms numstat prints:
//
//	src/{old => new}/file.go
//	old.go => new.go
func SplitRename(p string) (oldPath, newPath string, ok bool) {
	if i := strings.Index(p, "{"); i >= 0 {
		j := strings.Index(p[i:], "}")
		if j > 0 {
			inner := p[i+1 : i+j]
			if a, b, found := strings.Cut(inner, " => "); found {
				prefix, suffix := p[:i], p[i+j+1:]
				return cleanPath(prefix + a + suffix), cleanPath(prefix + b + suffix), true
			}
		}
	}
	if a, b, found := strings.Cut(p, " => "); found {
		return cleanPath(a), cleanPath(b), true
	}
	return "", "", false
}

// cleanPath fixes the "//" left by renames such as "src/{ => sub}/a.go".
func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean(p), "./")
}

// unquote handles paths git wraps in C-style quotes (tabs, quotes,
// newlines). With core.quotepath=off non-ASCII names arrive unquoted.
func unquote(p string) string {
	if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
		if s, err := strconv.Unquote(p); err == nil {
			return s
		}
	}
	return p
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
