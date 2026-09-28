// Package dto holds the data the ownership feature takes in and gives out.
// It has no behaviour and no dependencies, so the service, the CLI, the
// HTTP transport and the storage layer can all share it.
package dto

import "time"

// --- input ---

// Commit is a commit as the analyzer sees it. Sources must deliver commits
// newest first, which is git log's natural order.
type Commit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	Email   string    `json:"email"`
	Time    time.Time `json:"time"`
	Message string    `json:"message,omitempty"`
	Changes []Change  `json:"changes"`
}

// Change is one file touched by a commit.
type Change struct {
	Path    string `json:"path"`              // path after the commit
	OldPath string `json:"oldPath,omitempty"` // previous path for renames, else ""
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary,omitempty"`
}

// File is one file that exists at the analyzed revision.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Status says whether a person still works on the code.
type Status string

const (
	StatusActive  Status = "active"
	StatusLeaving Status = "leaving" // announced departure, still around
	StatusLeft    Status = "left"
	// StatusInactive is inferred: no commits within the active window.
	StatusInactive Status = "inactive"
)

// PersonStatus is an explicit status for one person, keyed by any of their
// email addresses. Without it, activity is inferred from commit dates.
type PersonStatus struct {
	Email  string    `json:"email"`
	Status Status    `json:"status"`
	Date   time.Time `json:"date,omitzero"` // departure date for leaving/left, optional
}

// --- output ---

// Risk levels, ordered from worst to best.
type Risk string

const (
	RiskCritical Risk = "critical"
	RiskHigh     Risk = "high"
	RiskMedium   Risk = "medium"
	RiskLow      Risk = "low"
)

// Rank orders risks: critical = 0 ... low = 3.
func (r Risk) Rank() int {
	switch r {
	case RiskCritical:
		return 0
	case RiskHigh:
		return 1
	case RiskMedium:
		return 2
	default:
		return 3
	}
}

// AtLeast reports whether r is as bad as or worse than other.
func (r Risk) AtLeast(other Risk) bool { return r.Rank() <= other.Rank() }

// Reason codes explain a module's risk. The UI turns them into sentences.
type Reason string

const (
	ReasonOrphaned     Reason = "orphaned"      // no active person knows the module
	ReasonOwnerLeaving Reason = "owner_leaving" // knowledge leaves with someone who is leaving
	ReasonSingleOwner  Reason = "single_owner"  // bus factor 1
	ReasonTwoOwners    Reason = "two_owners"    // bus factor 2
	ReasonAIHeavy      Reason = "ai_heavy"      // most recent changes came from AI agents
	ReasonLowHistory   Reason = "low_history"   // too few commits to be confident
)

// Person is one human contributor after merging email aliases.
type Person struct {
	ID         string    `json:"id"`     // stable key: the first normalised email seen
	Name       string    `json:"name"`   // most frequent name
	Email      string    `json:"email"`  // most frequent address
	Emails     []string  `json:"emails"` // all addresses, normalised
	Commits    int       `json:"commits"`
	LastCommit time.Time `json:"lastCommit"`
	Status     Status    `json:"status"`
	LeaveDate  time.Time `json:"leaveDate,omitzero"`
}

// Owner is a person's share of the knowledge of one module.
type Owner struct {
	PersonID  string    `json:"personId"`
	Name      string    `json:"name"`
	Share     float64   `json:"share"` // 0..1 within the module
	LastTouch time.Time `json:"lastTouch"`
	Status    Status    `json:"status"`
	// Confirmed is set when a team lead said this person knows the module,
	// whatever the history shows. A confirmed person who is present keeps
	// the module maintainable on their own (see OwnerOverride).
	Confirmed bool `json:"confirmed,omitempty"`
}

// OwnerOverride corrects what the history says about one module.
//
// Knows=true: the person knows the module even if they wrote little of it
// (they reviewed every change for years, or took it over in a handover).
// While present, they count as holding enough knowledge on their own.
//
// Knows=false: the person's changes there do not reflect knowledge (a mass
// refactoring, a one-off migration); their history in the module is ignored.
//
// Overrides apply to modules, not to the groups that aggregate them.
type OwnerOverride struct {
	Module string `json:"module"`
	Email  string `json:"email"`
	Knows  bool   `json:"knows"`
}

// Module kinds.
const (
	KindModule = "module" // a unit people own: a package or directory
	KindGroup  = "group"  // an ancestor directory, aggregated from modules
)

// Module is a unit of code with its owners and risk.
type Module struct {
	Path      string   `json:"path"`   // "internal/features/users/service", "." for the root
	Parent    string   `json:"parent"` // parent path, "" for the root group
	Kind      string   `json:"kind"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Commits   int      `json:"commits"`
	Owners    []Owner  `json:"owners,omitempty"` // sorted by share, highest first
	BusFactor int      `json:"busFactor"`        // people who must leave before too little knowledge remains
	AIShare   float64  `json:"aiShare"`          // share of recent changed lines from AI-assisted commits
	Risk      Risk     `json:"risk"`
	Reasons   []Reason `json:"reasons,omitempty"`
}

// Summary counts modules (kind "module" only) by risk.
type Summary struct {
	Modules     int `json:"modules"`
	Critical    int `json:"critical"`
	High        int `json:"high"`
	Medium      int `json:"medium"`
	Low         int `json:"low"`
	Orphaned    int `json:"orphaned"`
	SingleOwner int `json:"singleOwner"`
}

// Report is the result of one analysis.
type Report struct {
	AsOf    time.Time `json:"asOf"`
	Commits int       `json:"commits"` // commits that contributed
	People  []Person  `json:"people"`
	Modules []Module  `json:"modules"` // modules first (worst risk first), then groups
	Summary Summary   `json:"summary"`
	Actions []Action  `json:"actions"` // what to do next, most urgent first
}

// Action kinds.
const (
	ActionHandover    = "handover"         // someone leaving holds key knowledge
	ActionAssignOwner = "assign_owner"     // nobody present knows the module
	ActionSecondOwner = "add_second_owner" // one person knows the module
	ActionReviewAI    = "review_ai_code"   // mostly AI-written, few people understand it
)

// Action is one recommended next step. OwnerID is the person the knowledge
// sits with; CandidateID is who could take it over: someone present who
// already worked on the module or, failing that, on the code around it.
type Action struct {
	Kind          string  `json:"kind"`
	Module        string  `json:"module,omitempty"`
	Risk          Risk    `json:"risk"`
	OwnerID       string  `json:"ownerId,omitempty"`
	OwnerName     string  `json:"ownerName,omitempty"`
	OwnerEmail    string  `json:"ownerEmail,omitempty"`
	CandidateID   string  `json:"candidateId,omitempty"`
	CandidateName string  `json:"candidateName,omitempty"`
	Modules       int     `json:"modules,omitempty"`  // handover: key modules of the person
	Orphaned      int     `json:"orphaned,omitempty"` // handover: modules nobody would know afterwards
	AIShare       float64 `json:"aiShare,omitempty"`
	Bytes         int64   `json:"bytes,omitempty"`
}

// Handover is a checklist for passing on what one person knows.
type Handover struct {
	Person  Person           `json:"person"`
	AsOf    time.Time        `json:"asOf"`
	Modules []HandoverModule `json:"modules"` // most at risk first
}

// HandoverModule is one module the person is a key owner of.
type HandoverModule struct {
	Path           string      `json:"path"`
	Share          float64     `json:"share"` // the person's share of the module's knowledge
	Files          int         `json:"files"`
	Bytes          int64       `json:"bytes"`
	LastTouch      time.Time   `json:"lastTouch"`
	BusFactorAfter int         `json:"busFactorAfter"` // once the person is gone
	RiskAfter      Risk        `json:"riskAfter"`
	CandidateID    string      `json:"candidateId,omitempty"`
	CandidateName  string      `json:"candidateName,omitempty"`
	OwnFiles       []FileShare `json:"ownFiles,omitempty"` // files mostly written by the person
	KeyCommits     []CommitRef `json:"keyCommits,omitempty"`
	Reverts        []CommitRef `json:"reverts,omitempty"`
}

// FileShare is how much of a file's history one person wrote.
type FileShare struct {
	Path  string  `json:"path"`
	Share float64 `json:"share"`
	Lines int     `json:"lines"`
}

// Question kinds, in the order a handover asks them.
const (
	QuestionPurpose   = "purpose"    // what the module is for, what must never break
	QuestionCommit    = "commit"     // why a large change was made this way
	QuestionRevert    = "revert"     // what went wrong in a reverted change
	QuestionFragile   = "fragile"    // which of the person's own files is the most fragile
	QuestionOps       = "operations" // what breaks in production / which tests are flaky
	QuestionSuccessor = "successor"  // what the successor should learn first
	QuestionCustom    = "custom"     // written by a person
	QuestionAI        = "ai"         // suggested by a language model
)

// Question is one thing to ask the person who knows a module. Prompt is
// the question alone; Text adds the context (commit, files) in plain text,
// ready to show anywhere. Commit and Files point at what it is about.
type Question struct {
	Kind   string     `json:"kind"`
	Prompt string     `json:"prompt"`
	Text   string     `json:"text"`
	Commit *CommitRef `json:"commit,omitempty"`
	Files  []string   `json:"files,omitempty"`
}

// CommitRef points at a commit worth asking about.
type CommitRef struct {
	Hash    string    `json:"hash"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Time    time.Time `json:"time"`
	Lines   int       `json:"lines"`
}
