package service

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// authorKind tells humans from automation.
type authorKind int

const (
	kindHuman authorKind = iota
	kindBot              // dependency bots, CI: ignored entirely
	kindAgent            // AI agents committing on their own: count as AI code, nobody's knowledge
)

// botNames are automation accounts that are not "[bot]"-suffixed everywhere.
var botNames = []string{
	"dependabot", "renovate", "github-actions", "gitlab-ci", "semantic-release-bot",
	"greenkeeper", "snyk-bot", "pre-commit-ci", "imgbot", "allcontributors",
}

// agentEmails are addresses AI agents commit with as the author.
var agentEmails = map[string]bool{
	"noreply@anthropic.com":            true,
	"copilot@users.noreply.github.com": true,
	"cursoragent@cursor.com":           true,
}

// aiHints name AI tools in bot accounts and co-author trailers.
var aiHints = []string{
	"claude", "anthropic", "copilot", "cursor", "codex", "openai", "chatgpt", "gemini",
	"devin", "aider", "windsurf", "codeium", "jules", "amazon q", "tabnine", "sourcegraph cody",
}

var (
	trailerRe   = regexp.MustCompile(`(?im)^\s*(co-authored-by|assisted-by|generated-by)\s*:\s*(.+)$`)
	generatedRe = regexp.MustCompile(`(?i)generated (with|by) \[?(claude|cursor|copilot|codex|aider|devin|gemini|windsurf)`)
	spacesRe    = regexp.MustCompile(`\s+`)
)

func classifyAuthor(name, email string) authorKind {
	n, e := strings.ToLower(name), normalizeEmail(email)
	if agentEmails[e] {
		return kindAgent
	}
	isBot := strings.Contains(n, "[bot]") || strings.Contains(e, "[bot]")
	if !isBot {
		local, _, _ := strings.Cut(e, "@")
		for _, b := range botNames {
			if strings.Contains(n, b) || strings.Contains(local, b) {
				isBot = true
				break
			}
		}
	}
	if !isBot {
		return kindHuman
	}
	for _, h := range aiHints {
		if strings.Contains(n, h) || strings.Contains(e, h) {
			return kindAgent
		}
	}
	return kindBot
}

// isAIAssisted detects commits made with an AI tool: co-author trailers,
// "Generated with ..." footers and aider's "(aider)" author suffix.
func isAIAssisted(c dto.Commit) bool {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(c.Author)), "(aider)") {
		return true
	}
	if generatedRe.MatchString(c.Message) {
		return true
	}
	for _, m := range trailerRe.FindAllStringSubmatch(c.Message, -1) {
		v := strings.ToLower(m[2])
		for _, h := range aiHints {
			if strings.Contains(v, h) {
				return true
			}
		}
	}
	return false
}

// normalizeEmail lower-cases an address and folds GitHub's numbered
// noreply form "123+login@users.noreply.github.com" into "login@...".
func normalizeEmail(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	e = strings.Trim(e, "<>")
	if local, domain, ok := strings.Cut(e, "@"); ok && domain == "users.noreply.github.com" {
		if _, login, ok := strings.Cut(local, "+"); ok {
			e = login + "@" + domain
		}
	}
	return e
}

// genericNames are too common to merge identities on.
var genericNames = map[string]bool{
	"root": true, "admin": true, "user": true, "unknown": true, "your name": true,
	"ubuntu": true, "developer": true, "dev": true, "test": true, "git": true,
}

func normalizeName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, "(aider)")
	return strings.TrimSpace(spacesRe.ReplaceAllString(n, " "))
}

// cyrillic transliterates Ukrainian and Russian letters, so "Даниил
// Кухаренко" and "Daniil Kukharenko" meet on the same key.
var cyrillic = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "h", 'ґ': "g", 'д': "d", 'е': "e", 'є': "ie", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'і': "i", 'ї': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh",
	'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "iu", 'я': "ia",
}

// nameKey folds a full name so that spelling variants of the same person
// match: case, spaces and punctuation are ignored ("DaniilKukharenko"),
// Cyrillic is transliterated, and the usual transliteration variants are
// folded together (kh/h, y/i/j, x/ks, w/v, doubled letters), so
// "Daniil Kuharenko" and "Danil Kukharenko" share a key. Short keys are
// not used: too many different people share a short name.
func nameKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "(aider)")) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case cyrillic[r] != "" || r == 'ъ' || r == 'ь':
			b.WriteString(cyrillic[r])
		}
	}
	k := b.String()
	for _, f := range [][2]string{{"x", "ks"}, {"kh", "h"}, {"y", "i"}, {"j", "i"}, {"w", "v"}} {
		k = strings.ReplaceAll(k, f[0], f[1])
	}
	var out []byte
	for i := 0; i < len(k); i++ {
		if i == 0 || k[i] != k[i-1] {
			out = append(out, k[i])
		}
	}
	if len(out) < 6 {
		return ""
	}
	return string(out)
}

// identities merges the addresses one person commits with. Two addresses
// belong to the same person when their names fold to the same nameKey;
// .mailmap is already applied by git before we get here.
type identities struct {
	parent  map[string]string
	names   map[string]map[string]int // email key -> name -> commits
	last    map[string]time.Time
	commits map[string]int
}

func newIdentities() *identities {
	return &identities{
		parent:  make(map[string]string),
		names:   make(map[string]map[string]int),
		last:    make(map[string]time.Time),
		commits: make(map[string]int),
	}
}

// observe records one commit and returns the key the author is tracked under.
func (ids *identities) observe(name, email string, t time.Time) string {
	key := normalizeEmail(email)
	nn := normalizeName(name)
	if key == "" {
		key = "name:" + nn
	}
	ids.add(key)
	if ids.names[key] == nil {
		ids.names[key] = make(map[string]int)
	}
	ids.names[key][strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "(aider)"))]++
	ids.commits[key]++
	if t.After(ids.last[key]) {
		ids.last[key] = t
	}
	nk := nameKey(name)
	if nk == "" && len([]rune(nn)) >= 3 {
		nk = nn // short or non-Latin names: exact match only
	}
	if nk != "" && !genericNames[nn] {
		nk = "name:" + nk
		ids.add(nk)
		ids.union(key, nk)
	}
	return key
}

func (ids *identities) add(k string) {
	if _, ok := ids.parent[k]; !ok {
		ids.parent[k] = k
	}
}

func (ids *identities) find(k string) string {
	for ids.parent[k] != k {
		ids.parent[k] = ids.parent[ids.parent[k]]
		k = ids.parent[k]
	}
	return k
}

func (ids *identities) union(a, b string) {
	ra, rb := ids.find(a), ids.find(b)
	if ra == rb {
		return
	}
	// Keep the lexicographically smaller root for deterministic IDs.
	if rb < ra {
		ra, rb = rb, ra
	}
	ids.parent[rb] = ra
}

// people builds one Person per merged identity and returns them with a
// map from every tracked key to its person ID.
func (ids *identities) people() ([]dto.Person, map[string]string) {
	groups := make(map[string][]string)
	for k := range ids.parent {
		if strings.HasPrefix(k, "name:") && ids.names[k] == nil {
			continue // pure name nodes only link addresses
		}
		r := ids.find(k)
		groups[r] = append(groups[r], k)
	}
	keyToID := make(map[string]string)
	people := make([]dto.Person, 0, len(groups))
	for _, keys := range groups {
		sort.Strings(keys)
		p := dto.Person{ID: personID(keys)}
		nameCount := make(map[string]int)
		emailCommits := -1
		for _, k := range keys {
			keyToID[k] = p.ID
			if !strings.HasPrefix(k, "name:") {
				p.Emails = append(p.Emails, k)
				if ids.commits[k] > emailCommits {
					p.Email, emailCommits = k, ids.commits[k]
				}
			}
			for n, c := range ids.names[k] {
				nameCount[n] += c
			}
			p.Commits += ids.commits[k]
			if ids.last[k].After(p.LastCommit) {
				p.LastCommit = ids.last[k]
			}
		}
		p.Name = mostFrequent(nameCount)
		people = append(people, p)
	}
	sort.Slice(people, func(i, j int) bool { return people[i].ID < people[j].ID })
	return people, keyToID
}

// personID prefers the first email (keys are sorted); "name:" keys are a
// fallback for authors without an address.
func personID(sortedKeys []string) string {
	for _, k := range sortedKeys {
		if !strings.HasPrefix(k, "name:") {
			return k
		}
	}
	return sortedKeys[0]
}

func mostFrequent(m map[string]int) string {
	best, bestN := "", -1
	for n, c := range m {
		if c > bestN || (c == bestN && n < best) {
			best, bestN = n, c
		}
	}
	return best
}
