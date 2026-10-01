package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// ErrPersonNotFound means nobody with that e-mail appears in the history.
var ErrPersonNotFound = errors.New("no commits by this person")

// Handover prepares the hand-over of everything one person knows: the
// modules they are a key owner of, what happens to each when they leave,
// who could take over, the files only they wrote, and the commits worth
// asking them about. It reads the history twice: once to find who knows
// what, once to collect the facts about this person.
func Handover(ctx context.Context, src Source, cfg Config, email string, maxModules int) (dto.Handover, error) {
	files, err := src.Files(ctx)
	if err != nil {
		return dto.Handover{}, fmt.Errorf("list files: %w", err)
	}
	gen, err := src.Generated(ctx)
	if err != nil {
		return dto.Handover{}, fmt.Errorf("detect generated files: %w", err)
	}

	// Pass 1: the report as it will look once the person has left.
	cfg.Statuses = append(append([]dto.PersonStatus(nil), cfg.Statuses...),
		dto.PersonStatus{Email: email, Status: dto.StatusLeaving})
	a := NewAnalyzer(cfg, files, gen)
	if err := src.Commits(ctx, func(c dto.Commit) error { a.Add(c); return ctx.Err() }); err != nil {
		return dto.Handover{}, fmt.Errorf("read history: %w", err)
	}
	rep := a.Report()

	want := normalizeEmail(email)
	var person dto.Person
	for _, p := range rep.People {
		for _, e := range p.Emails {
			if e == want {
				person = p
			}
		}
	}
	if person.ID == "" {
		return dto.Handover{}, fmt.Errorf("%w: %s", ErrPersonNotFound, email)
	}
	mine := make(map[string]bool, len(person.Emails))
	for _, e := range person.Emails {
		mine[e] = true
	}

	ix := newIndex(rep)
	type acc struct {
		out        dto.HandoverModule
		total      map[string]int            // file -> lines changed by anyone
		own        map[string]int            // file -> lines changed by the person
		commits    map[string]*dto.CommitRef // by subject
		seenRevert map[string]bool
	}
	targets := make(map[string]*acc)
	var order []string
	for _, m := range rep.Modules {
		if m.Kind != dto.KindModule {
			continue
		}
		for i, o := range m.Owners {
			if o.PersonID != person.ID || (o.Share < cfg.OwnerShare && i > 0) {
				continue
			}
			h := dto.HandoverModule{
				Path: m.Path, Share: o.Share, Files: m.Files, Bytes: m.Bytes, LastTouch: o.LastTouch,
				BusFactorAfter: m.BusFactor, RiskAfter: m.Risk,
			}
			if c := ix.candidate(m, person.ID); c.PersonID != "" {
				h.CandidateID, h.CandidateName = c.PersonID, c.Name
			}
			targets[m.Path] = &acc{out: h, total: map[string]int{}, own: map[string]int{},
				commits: map[string]*dto.CommitRef{}, seenRevert: map[string]bool{}}
			order = append(order, m.Path)
		}
	}
	// Most at risk after the departure first, then where the person matters most.
	sort.SliceStable(order, func(i, j int) bool {
		x, y := targets[order[i]].out, targets[order[j]].out
		if x.RiskAfter.Rank() != y.RiskAfter.Rank() {
			return x.RiskAfter.Rank() < y.RiskAfter.Rank()
		}
		return x.Share*float64(x.Bytes) > y.Share*float64(y.Bytes)
	})
	if maxModules > 0 && len(order) > maxModules {
		for _, p := range order[maxModules:] {
			delete(targets, p)
		}
		order = order[:maxModules]
	}

	// Pass 2: facts about this person's work in those modules.
	b := NewAnalyzer(cfg, files, gen)
	err = src.Commits(ctx, func(c dto.Commit) error {
		items := b.items(c)
		if len(items) == 0 {
			return ctx.Err()
		}
		byMe := mine[normalizeEmail(c.Email)]
		subject, _, _ := strings.Cut(c.Message, "\n")
		revert := strings.HasPrefix(subject, "Revert ")
		perModule := make(map[string]int)
		for _, it := range items {
			t := targets[b.modules.moduleOf(it.path)]
			if t == nil {
				continue
			}
			t.total[it.path] += it.lines
			if byMe {
				t.own[it.path] += it.lines
			}
			perModule[t.out.Path] += it.lines
		}
		for path, lines := range perModule {
			t := targets[path]
			ref := dto.CommitRef{Hash: c.Hash, Subject: subject, Author: c.Author, Time: c.Time, Lines: lines}
			// Cherry-picks to release branches repeat a commit under a new
			// hash: one question per change is enough.
			key := strings.ToLower(strings.TrimSpace(subject))
			if revert && len(t.out.Reverts) < 3 && !t.seenRevert[key] {
				t.seenRevert[key] = true
				t.out.Reverts = append(t.out.Reverts, ref) // newest first
			}
			if byMe && !revert {
				if prev, ok := t.commits[key]; !ok || ref.Lines > prev.Lines {
					t.commits[key] = &ref
				}
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return dto.Handover{}, fmt.Errorf("read history: %w", err)
	}

	out := dto.Handover{Person: person, AsOf: rep.AsOf}
	for _, p := range order {
		t := targets[p]
		for f, n := range t.own {
			if share := float64(n) / float64(t.total[f]); share >= 0.6 && n >= 10 {
				t.out.OwnFiles = append(t.out.OwnFiles, dto.FileShare{Path: f, Share: share, Lines: n})
			}
		}
		sort.Slice(t.out.OwnFiles, func(i, j int) bool { return t.out.OwnFiles[i].Lines > t.out.OwnFiles[j].Lines })
		if len(t.out.OwnFiles) > 5 {
			t.out.OwnFiles = t.out.OwnFiles[:5]
		}
		for _, c := range t.commits {
			t.out.KeyCommits = append(t.out.KeyCommits, *c)
		}
		sort.Slice(t.out.KeyCommits, func(i, j int) bool { return t.out.KeyCommits[i].Lines > t.out.KeyCommits[j].Lines })
		if len(t.out.KeyCommits) > 3 {
			t.out.KeyCommits = t.out.KeyCommits[:3]
		}
		out.Modules = append(out.Modules, t.out)
	}
	return out, nil
}
