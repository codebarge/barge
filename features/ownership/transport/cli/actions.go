package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

const maxActions = 5

// renderActions prints "What to do next": the report's actions turned into
// a short numbered list. Similar actions are grouped so that one person
// holding 20 modules reads as one problem, not twenty.
func renderActions(w io.Writer, c palette, rep dto.Report) {
	var items []string
	var assign, ai []dto.Action
	second := make(map[string][]dto.Action)
	var owners []string

	for _, a := range rep.Actions {
		switch a.Kind {
		case dto.ActionHandover:
			items = append(items, handoverItem(c, a))
		case dto.ActionAssignOwner:
			assign = append(assign, a)
		case dto.ActionSecondOwner:
			if _, seen := second[a.OwnerID]; !seen {
				owners = append(owners, a.OwnerID)
			}
			second[a.OwnerID] = append(second[a.OwnerID], a)
		case dto.ActionReviewAI:
			ai = append(ai, a)
		}
	}

	if len(assign) > 0 {
		items = append(items, assignItem(c, assign))
	}
	sort.SliceStable(owners, func(i, j int) bool { return len(second[owners[i]]) > len(second[owners[j]]) })
	for i, id := range owners {
		items = append(items, secondOwnerItem(c, second[id], i == 0))
	}
	if len(ai) > 0 {
		items = append(items, aiItem(c, ai))
	}
	if len(items) == 0 {
		return
	}

	fmt.Fprintln(w, "\n"+c.bold("What to do next:"))
	for i, it := range items {
		if i == maxActions {
			fmt.Fprintln(w, c.dim(fmt.Sprintf("  …and %d more; see \"actions\" in --json.", len(items)-maxActions)))
			break
		}
		lines := strings.Split(it, "\n")
		fmt.Fprintf(w, "  %d. %s\n", i+1, lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(w, "     %s\n", l)
		}
	}
}

func handoverItem(c palette, a dto.Action) string {
	s := fmt.Sprintf("%s is leaving and is a key owner of %d module%s", clip(a.OwnerName, 30), a.Modules, plural(a.Modules))
	if a.Orphaned > 0 {
		s += fmt.Sprintf("; %s would have nobody who knows %s",
			c.risk(dto.RiskCritical, fmt.Sprint(a.Orphaned)), them(a.Orphaned))
	}
	return s + ".\nPrepare the handover now: " + c.bold("barge handover "+a.OwnerEmail)
}

func assignItem(c palette, as []dto.Action) string {
	if len(as) == 1 {
		a := as[0]
		s := fmt.Sprintf("%s: nobody active knows it any more", c.bold(shortPath(a.Module)))
		if a.OwnerName != "" {
			s += fmt.Sprintf(" (%s was the main author)", clip(a.OwnerName, 30))
		}
		s += ".\n"
		if a.CandidateName != "" {
			return s + fmt.Sprintf("Make %s its owner and ask for a short README, or delete it if it is no longer used.", clip(a.CandidateName, 30))
		}
		return s + "Pick an owner and ask for a short README, or delete it if it is no longer used."
	}
	names := make([]string, 0, 3)
	for _, a := range as[:min(3, len(as))] {
		names = append(names, c.bold(shortPath(a.Module)))
	}
	list := strings.Join(names, ", ")
	if len(as) > 3 {
		list += fmt.Sprintf(" and %d more", len(as)-3)
	}
	s := fmt.Sprintf("%d modules are known only by people who are no longer active: %s.\n", len(as), list)
	if a := as[0]; a.CandidateName != "" {
		return s + fmt.Sprintf("Give each an owner (%s → %s), and delete what is no longer used.", shortPath(a.Module), clip(a.CandidateName, 30))
	}
	return s + "Give each an owner, and delete what is no longer used."
}

func secondOwnerItem(c palette, as []dto.Action, first bool) string {
	owner := clip(as[0].OwnerName, 30)
	if len(as) == 1 {
		a := as[0]
		s := fmt.Sprintf("%s: only %s knows it.\n", c.bold(shortPath(a.Module)), owner)
		if a.CandidateName != "" {
			return s + fmt.Sprintf("Make %s a required reviewer there for the next month.", clip(a.CandidateName, 30))
		}
		return s + "Pick a second person and make them a required reviewer there for the next month."
	}
	pairs := make([]string, 0, 3)
	for _, a := range as[:min(3, len(as))] {
		p := c.bold(shortPath(a.Module))
		if a.CandidateName != "" {
			p += " (" + clip(a.CandidateName, 30) + ")"
		}
		pairs = append(pairs, p)
	}
	lead := fmt.Sprintf("%s is the only one who knows %d modules.", owner, len(as))
	if first {
		lead += " Don't wait for a resignation letter."
	}
	return lead + "\nRun " + c.bold("barge handover "+as[0].OwnerEmail) + " and pair someone on the largest ones:\n" +
		strings.Join(pairs, ", ") + "."
}

func aiItem(c palette, as []dto.Action) string {
	if len(as) == 1 {
		a := as[0]
		return fmt.Sprintf("%s: %d%% of recent changes came from AI agents.\nWalk the team through it in a review session so more than one person understands it.",
			c.bold(shortPath(a.Module)), pct(a.AIShare))
	}
	names := make([]string, 0, 3)
	for _, a := range as[:min(3, len(as))] {
		names = append(names, fmt.Sprintf("%s (%d%%)", c.bold(shortPath(a.Module)), pct(a.AIShare)))
	}
	return fmt.Sprintf("%d modules were written mostly by AI agents: %s.\nReview them as a team so more than one person understands them.",
		len(as), strings.Join(names, ", "))
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
