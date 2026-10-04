// Package cli renders an ownership report for the terminal. It is the
// transport layer of the ownership feature for the command-line tool, as
// transport/http is for the server.
package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// Header describes what was analysed.
type Header struct {
	Repo    string
	Rev     string
	Elapsed time.Duration
}

// Options control what Render prints.
type Options struct {
	All                bool
	Top                int
	Color              bool
	KnowledgeThreshold float64
	// Only are the --only patterns the report was narrowed to, and Total
	// the number of modules before that.
	Only  []string
	Total int
}

type palette struct{ on bool }

func (p palette) wrap(code, s string) string {
	if !p.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) risk(r dto.Risk, s string) string {
	switch r {
	case dto.RiskCritical:
		return p.wrap("1;31", s)
	case dto.RiskHigh:
		return p.wrap("38;5;208", s)
	case dto.RiskMedium:
		return p.wrap("33", s)
	default:
		return p.wrap("90", s)
	}
}

func (p palette) dim(s string) string  { return p.wrap("2", s) }
func (p palette) bold(s string) string { return p.wrap("1", s) }

const maxModuleWidth = 56

// Render prints the report as a table with a summary and hints.
func Render(w io.Writer, rep dto.Report, h Header, o Options) {
	c := palette{on: o.Color}
	people := make(map[string]dto.Person, len(rep.People))
	active := 0
	for _, p := range rep.People {
		people[p.ID] = p
		if p.Status == dto.StatusActive {
			active++
		}
	}

	fmt.Fprintf(w, "%s %s · %s · %s commits · %d people, %d active · as of %s\n\n",
		c.bold("Barge"), h.Repo, h.Rev, thousands(rep.Commits), len(rep.People), active,
		rep.AsOf.Format(time.DateOnly))
	if len(o.Only) > 0 {
		fmt.Fprintf(w, "%s\n\n", c.dim(fmt.Sprintf("Only modules under %s: %d of %d.",
			strings.Join(o.Only, ", "), rep.Summary.Modules, o.Total)))
	}

	var modules []dto.Module
	var root *dto.Module
	for i, m := range rep.Modules {
		switch {
		case m.Kind == dto.KindModule:
			modules = append(modules, m)
		case m.Path == ".":
			root = &rep.Modules[i]
		}
	}
	if len(modules) == 0 {
		fmt.Fprintln(w, "No source files with history found. Check --rev and --exclude.")
		return
	}

	shown := modules[:0:0]
	for _, m := range modules {
		if o.All || m.Risk.AtLeast(dto.RiskMedium) {
			shown = append(shown, m)
		}
	}
	matching := len(shown)
	if o.Top > 0 && len(shown) > o.Top {
		shown = shown[:o.Top]
	}

	if len(shown) == 0 {
		fmt.Fprintln(w, c.risk(dto.RiskLow, "Every module is known by at least three active people. Nice."))
	} else {
		width := len("MODULE")
		for _, m := range shown {
			if n := utf8.RuneCountInString(shortPath(m.Path)); n > width {
				width = n
			}
		}
		fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
			c.dim(pad("RISK", 8)), c.dim(padLeft("BUS", 3)), c.dim(padLeft("AI", 4)),
			c.dim(pad("MODULE", width)), c.dim("WHO KNOWS IT"))
		for _, m := range shown {
			ai := "–"
			if m.AIShare >= 0.05 {
				ai = fmt.Sprintf("%d%%", pct(m.AIShare))
			}
			fmt.Fprintf(w, "%s  %s  %s  %s  %s\n",
				c.risk(m.Risk, pad(string(m.Risk), 8)),
				padLeft(fmt.Sprint(m.BusFactor), 3),
				padLeft(ai, 4),
				pad(shortPath(m.Path), width),
				owners(c, m, people))
		}
	}

	s := rep.Summary
	fmt.Fprintf(w, "\n%d module%s: %s · %s · %s · %s\n", s.Modules, plural(s.Modules),
		c.risk(dto.RiskCritical, fmt.Sprintf("%d critical", s.Critical)),
		c.risk(dto.RiskHigh, fmt.Sprintf("%d high", s.High)),
		c.risk(dto.RiskMedium, fmt.Sprintf("%d medium", s.Medium)),
		c.risk(dto.RiskLow, fmt.Sprintf("%d low", s.Low)))
	if root != nil {
		th := pct(o.KnowledgeThreshold)
		explain := fmt.Sprintf("(people who must leave before the rest know less than %d%% of the code)", th)
		if root.BusFactor == 0 {
			explain = fmt.Sprintf("(people active today know less than %d%% of the code)", th)
		}
		fmt.Fprintf(w, "Repository bus factor: %s %s\n", c.bold(fmt.Sprint(root.BusFactor)), c.dim(explain))
	}

	renderActions(w, c, rep)

	var hints []string
	if len(shown) < matching {
		hints = append(hints, fmt.Sprintf("Showing %d of %d modules; --top 0 shows all.", len(shown), matching))
	}
	if !o.All && s.Low > 0 {
		hints = append(hints, fmt.Sprintf("%d low-risk module%s hidden; --all shows them.", s.Low, plural(s.Low)))
	}
	hints = append(hints, "Simulate a departure: barge scan --leaving <email>  ·  Handover checklist: barge handover <email>")
	fmt.Fprintln(w)
	for _, ht := range hints {
		fmt.Fprintln(w, c.dim(ht))
	}
	fmt.Fprintln(w, c.dim(fmt.Sprintf("Scanned in %s. Nothing left this machine.", h.Elapsed.Round(10*time.Millisecond))))
}

func owners(c palette, m dto.Module, people map[string]dto.Person) string {
	var parts []string
	for _, o := range m.Owners {
		if len(parts) == 3 {
			break
		}
		var s string
		switch {
		case o.Share >= 0.05:
			s = fmt.Sprintf("%s %d%%", clip(o.Name, 30), pct(o.Share))
			if o.Confirmed {
				s += " " + c.dim("(confirmed)")
			}
		case o.Confirmed:
			s = clip(o.Name, 30) + " " + c.dim("(confirmed)")
		default:
			continue
		}
		if st := people[o.PersonID].Status; st != dto.StatusActive {
			s += " " + c.dim("("+string(st)+")")
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return c.dim("nobody")
	}
	return strings.Join(parts, ", ")
}

func shortPath(p string) string {
	if p == "." {
		return "(root)"
	}
	if utf8.RuneCountInString(p) <= maxModuleWidth {
		return p
	}
	r := []rune(p)
	return "…" + string(r[len(r)-maxModuleWidth+1:])
}

func pad(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func padLeft(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

func pct(f float64) int { return int(f*100 + 0.5) }

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n-1])) + "…"
}
