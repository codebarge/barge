// Package service computes who knows which part of a codebase.
//
// Everything here is a pure function of the commit history, the list of
// files at the analysed revision and a Config: no database, no network, no
// clock. The CLI and the server run exactly the same code.
//
// The model, in short:
//
//   - Every change a person makes to a file adds log(1 + changed lines) to
//     their weight in the file's module. The logarithm stops one huge
//     generated or moved file from outweighing years of careful work.
//   - Weight decays with a half-life (180 days by default): people remember
//     what they changed last month better than what they wrote three years ago.
//   - A person's share of a module is their weight divided by everyone's.
//   - The bus factor is how many present people must leave before the people
//     who remain hold less than 40% of the knowledge. 40% rather than 50%
//     keeps two people who split a module 52/48 at bus factor 2.
package service

import (
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// Config tunes the analysis. The zero value is not useful: start from
// DefaultConfig.
type Config struct {
	// AsOf is the reference time for decay and activity. Zero means the
	// time of the newest commit, which makes results reproducible and
	// meaningful for repositories nobody touched recently.
	AsOf time.Time

	// HalfLife is how long it takes a change to lose half of its weight.
	HalfLife time.Duration

	// ActiveWindow: people without commits in this window before AsOf are
	// treated as inactive unless a status says otherwise.
	ActiveWindow time.Duration

	// KnowledgeThreshold is the share of a module's knowledge that present
	// people must hold for the module to be maintainable (0.4).
	KnowledgeThreshold float64

	// OwnerShare is the share from which a person counts as a key owner
	// of a module (0.2). Used to flag departures that matter.
	OwnerShare float64

	// AIWeight scales a person's credit for AI-assisted commits (0.5):
	// code an agent wrote is understood less deeply than code typed by hand.
	AIWeight float64

	// AIShareThreshold flags modules where at least this share of recent
	// changed lines came from AI-assisted commits (0.5).
	AIShareThreshold float64

	// Commits touching more than MassChangeFiles files (formatting runs,
	// licence headers, mass renames) count with MassChangeWeight.
	MassChangeFiles  int
	MassChangeWeight float64

	// MinCommits below which a module is flagged as low history.
	MinCommits int

	// Exclude lists extra glob patterns of files to ignore. Patterns
	// without a slash match the file name anywhere ("*.pb.go"); patterns
	// with a slash match the whole path and support "**".
	Exclude []string

	// NoDefaultExcludes turns off DefaultExclude.
	NoDefaultExcludes bool

	// ModuleRules are glob patterns of directories that form one module
	// with everything beneath them, e.g. "internal/features/*".
	ModuleRules []string

	// Depth, when > 0, groups files by their first Depth directories
	// instead of by the directory that contains them.
	Depth int

	// Statuses override inferred activity for specific people.
	Statuses []dto.PersonStatus

	// Owners correct the history for specific modules. Later entries for
	// the same module and person win.
	Owners []dto.OwnerOverride
}

const day = 24 * time.Hour

// DefaultConfig returns the settings Barge ships with.
func DefaultConfig() Config {
	return Config{
		HalfLife:           180 * day,
		ActiveWindow:       180 * day,
		KnowledgeThreshold: 0.4,
		OwnerShare:         0.2,
		AIWeight:           0.5,
		AIShareThreshold:   0.5,
		MassChangeFiles:    200,
		MassChangeWeight:   0.1,
		MinCommits:         3,
	}
}

// DefaultExclude are files that carry no human knowledge: dependencies,
// lock files, generated code and build output.
var DefaultExclude = []string{
	// vendored dependencies
	"**/vendor/**", "**/node_modules/**", "**/third_party/**",
	// lock files and checksums
	"go.sum", "*.lock", "package-lock.json", "pnpm-lock.yaml", "npm-shrinkwrap.json", "gradle.lockfile",
	// generated code
	"*.pb.go", "*.pb.gw.go", "*_gen.go", "*.gen.go", "*_generated.go", "zz_generated*",
	"*.generated.*", "*_pb2.py", "*_pb2_grpc.py",
	// build output and minified assets
	"**/dist/**", "*.min.js", "*.min.css", "*.map",
	// test data: fixtures are sample inputs, not code anyone maintains
	"**/__snapshots__/**", "*.snap",
	"**/testdata/**", "**/fixtures/**", "**/__fixtures__/**", "**/test-fixtures/**",
}
