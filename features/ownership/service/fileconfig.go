package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// ErrInvalidConfig wraps every problem found in a configuration file.
var ErrInvalidConfig = errors.New("invalid configuration")

// ConfigFileName is read from the root of the repository, when present.
const ConfigFileName = ".barge.json"

// FileConfig is the repository-level configuration a team commits next to
// its code:
//
//	{
//	  "modules": ["internal/features/*"],
//	  "exclude": ["docs/**", "*.sql"],
//	  "depth": 0,
//	  "halfLifeDays": 180,
//	  "activeDays": 180,
//	  "people": [{"email": "oleg@example.com", "status": "leaving", "date": "2026-10-14"}],
//	  "owners": [{"module": "internal/billing", "email": "anna@example.com", "knows": true}]
//	}
type FileConfig struct {
	Modules      []string     `json:"modules"`
	Exclude      []string     `json:"exclude"`
	Depth        int          `json:"depth"`
	HalfLifeDays int          `json:"halfLifeDays"`
	ActiveDays   int          `json:"activeDays"`
	People       []FilePerson `json:"people"`
	Owners       []FileOwner  `json:"owners"`
}

// FileOwner corrects the history for one module: "knows" must be given
// explicitly, true or false, so a missing key cannot silently mean false.
type FileOwner struct {
	Module string `json:"module"`
	Email  string `json:"email"`
	Knows  *bool  `json:"knows"`
}

// FilePerson sets the status of one contributor.
type FilePerson struct {
	Email  string `json:"email"`
	Status string `json:"status"`
	Date   string `json:"date"` // YYYY-MM-DD, optional
}

// ParseFileConfig decodes and validates a .barge.json file. Unknown keys
// are rejected so typos do not silently do nothing.
func ParseFileConfig(data []byte) (FileConfig, error) {
	var fc FileConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fc); err != nil {
		return fc, fmt.Errorf("%s: %w: %v", ConfigFileName, ErrInvalidConfig, err)
	}
	if fc.Depth < 0 || fc.HalfLifeDays < 0 || fc.ActiveDays < 0 {
		return fc, fmt.Errorf("%s: %w: depth and day counts must not be negative", ConfigFileName, ErrInvalidConfig)
	}
	for i, p := range fc.People {
		if strings.TrimSpace(p.Email) == "" {
			return fc, fmt.Errorf("%s: people[%d]: %w: email is required", ConfigFileName, i, ErrInvalidConfig)
		}
		if _, err := ParseStatus(p.Status); err != nil {
			return fc, fmt.Errorf("%s: people[%d]: %w", ConfigFileName, i, err)
		}
		if p.Date != "" {
			if _, err := time.Parse(time.DateOnly, p.Date); err != nil {
				return fc, fmt.Errorf("%s: people[%d]: %w: date must be YYYY-MM-DD", ConfigFileName, i, ErrInvalidConfig)
			}
		}
	}
	for i, o := range fc.Owners {
		switch {
		case strings.TrimSpace(o.Module) == "":
			return fc, fmt.Errorf("%s: owners[%d]: %w: module is required", ConfigFileName, i, ErrInvalidConfig)
		case strings.TrimSpace(o.Email) == "":
			return fc, fmt.Errorf("%s: owners[%d]: %w: email is required", ConfigFileName, i, ErrInvalidConfig)
		case o.Knows == nil:
			return fc, fmt.Errorf("%s: owners[%d]: %w: knows must be true or false", ConfigFileName, i, ErrInvalidConfig)
		}
	}
	return fc, nil
}

// Apply merges the file into cfg. Values already set in cfg by flags win
// for scalars; lists are combined.
func (fc FileConfig) Apply(cfg *Config) {
	cfg.ModuleRules = append(cfg.ModuleRules, fc.Modules...)
	cfg.Exclude = append(cfg.Exclude, fc.Exclude...)
	if cfg.Depth == 0 && fc.Depth > 0 {
		cfg.Depth = fc.Depth
	}
	if fc.HalfLifeDays > 0 && cfg.HalfLife == DefaultConfig().HalfLife {
		cfg.HalfLife = time.Duration(fc.HalfLifeDays) * day
	}
	if fc.ActiveDays > 0 && cfg.ActiveWindow == DefaultConfig().ActiveWindow {
		cfg.ActiveWindow = time.Duration(fc.ActiveDays) * day
	}
	for _, p := range fc.People {
		st, _ := ParseStatus(p.Status)
		ps := dto.PersonStatus{Email: p.Email, Status: st}
		if p.Date != "" {
			ps.Date, _ = time.Parse(time.DateOnly, p.Date)
		}
		cfg.Statuses = append(cfg.Statuses, ps)
	}
	// File first: overrides set by the caller (flags, the app) come later
	// and win, as later entries do.
	file := make([]dto.OwnerOverride, 0, len(fc.Owners)+len(cfg.Owners))
	for _, o := range fc.Owners {
		file = append(file, dto.OwnerOverride{Module: o.Module, Email: o.Email, Knows: *o.Knows})
	}
	cfg.Owners = append(file, cfg.Owners...)
}

// ParseStatus validates a status name.
func ParseStatus(s string) (dto.Status, error) {
	switch st := dto.Status(strings.ToLower(strings.TrimSpace(s))); st {
	case dto.StatusActive, dto.StatusLeaving, dto.StatusLeft, dto.StatusInactive:
		return st, nil
	}
	return "", fmt.Errorf("%w: unknown status %q (want active, leaving, left or inactive)", ErrInvalidConfig, s)
}
