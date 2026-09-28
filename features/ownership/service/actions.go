package service

import (
	"sort"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// index answers "who could take this over?" for a report.
type index struct {
	people map[string]dto.Person
	groups map[string]dto.Module
	load   map[string]int // modules a person is already the only one to know
}

// overloaded people are not suggested as successors when someone else
// fits: asking the bottleneck to also learn more modules makes it worse.
const overloaded = 3

func newIndex(rep dto.Report) index {
	ix := index{
		people: make(map[string]dto.Person, len(rep.People)),
		groups: make(map[string]dto.Module),
		load:   make(map[string]int),
	}
	for _, p := range rep.People {
		ix.people[p.ID] = p
	}
	for _, m := range rep.Modules {
		if m.Kind == dto.KindGroup {
			ix.groups[m.Path] = m
			continue
		}
		if m.BusFactor == 1 {
			for _, o := range m.Owners {
				if ix.present(o.PersonID) {
					ix.load[o.PersonID]++
					break
				}
			}
		}
	}
	return ix
}

func (ix index) present(id string) bool { return ix.people[id].Status == dto.StatusActive }

// candidate picks who could take over module m from exclude: the present
// person with the largest share of the module itself, or else of the
// nearest enclosing directory. Someone who has worked next to the code
// learns it fastest. It returns an empty owner when nobody fits.
//
// People who are already the only one to know several modules are passed
// over while anyone else fits.
func (ix index) candidate(m dto.Module, exclude string) dto.Owner {
	if c := ix.find(m, exclude, true); c.PersonID != "" {
		return c
	}
	return ix.find(m, exclude, false)
}

func (ix index) find(m dto.Module, exclude string, spareOnly bool) dto.Owner {
	fits := func(o dto.Owner) bool {
		return o.PersonID != exclude && ix.present(o.PersonID) && (!spareOnly || ix.load[o.PersonID] < overloaded)
	}
	for _, o := range m.Owners {
		if fits(o) {
			return o
		}
	}
	for p := m.Parent; p != ""; p = parentOf(p) {
		g, ok := ix.groups[p]
		if !ok {
			continue
		}
		for _, o := range g.Owners {
			if fits(o) {
				return o
			}
		}
	}
	return dto.Owner{}
}

// recommend turns risks into next steps, most urgent first:
//  1. hand over what people who are leaving know;
//  2. give modules nobody knows an owner;
//  3. add a second person where only one knows the module;
//  4. walk the team through modules written mostly by AI.
func recommend(rep dto.Report, ownerShare float64) []dto.Action {
	ix := newIndex(rep)

	type leaving struct {
		modules, orphaned int
		risk              dto.Risk
	}
	departures := make(map[string]*leaving)
	var order []string
	var rest []dto.Action

	for _, m := range rep.Modules { // already sorted: worst risk, biggest first
		if m.Kind != dto.KindModule {
			continue
		}
		for _, o := range m.Owners {
			if ix.people[o.PersonID].Status != dto.StatusLeaving || o.Share < ownerShare {
				continue
			}
			d := departures[o.PersonID]
			if d == nil {
				d = &leaving{risk: dto.RiskHigh}
				departures[o.PersonID] = d
				order = append(order, o.PersonID)
			}
			d.modules++
			if m.BusFactor == 0 {
				d.orphaned++
				d.risk = dto.RiskCritical
			}
		}

		switch {
		case m.BusFactor == 0 && hasReason(m, dto.ReasonOwnerLeaving):
			// covered by the handover of the person leaving
		case m.BusFactor == 0:
			a := dto.Action{Kind: dto.ActionAssignOwner, Module: m.Path, Risk: m.Risk, Bytes: m.Bytes}
			if len(m.Owners) > 0 {
				a.OwnerID, a.OwnerName = m.Owners[0].PersonID, m.Owners[0].Name
			}
			if c := ix.candidate(m, ""); c.PersonID != "" {
				a.CandidateID, a.CandidateName = c.PersonID, c.Name
			}
			rest = append(rest, a)
		case m.BusFactor == 1:
			owner := ix.find(m, "", false) // the one present person who knows it
			a := dto.Action{Kind: dto.ActionSecondOwner, Module: m.Path, Risk: m.Risk, Bytes: m.Bytes,
				OwnerID: owner.PersonID, OwnerName: owner.Name, OwnerEmail: ix.people[owner.PersonID].Email}
			if c := ix.candidate(m, owner.PersonID); c.PersonID != "" {
				a.CandidateID, a.CandidateName = c.PersonID, c.Name
			}
			rest = append(rest, a)
		}
		if hasReason(m, dto.ReasonAIHeavy) {
			rest = append(rest, dto.Action{Kind: dto.ActionReviewAI, Module: m.Path, Risk: m.Risk,
				AIShare: m.AIShare, Bytes: m.Bytes})
		}
	}

	actions := make([]dto.Action, 0, len(order)+len(rest))
	for _, id := range order {
		d := departures[id]
		p := ix.people[id]
		actions = append(actions, dto.Action{Kind: dto.ActionHandover, Risk: d.risk,
			OwnerID: id, OwnerName: p.Name, OwnerEmail: p.Email, Modules: d.modules, Orphaned: d.orphaned})
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].Orphaned > actions[j].Orphaned })

	kindRank := map[string]int{dto.ActionAssignOwner: 0, dto.ActionSecondOwner: 1, dto.ActionReviewAI: 2}
	sort.SliceStable(rest, func(i, j int) bool { return kindRank[rest[i].Kind] < kindRank[rest[j].Kind] })
	return append(actions, rest...)
}

func hasReason(m dto.Module, r dto.Reason) bool {
	for _, x := range m.Reasons {
		if x == r {
			return true
		}
	}
	return false
}
