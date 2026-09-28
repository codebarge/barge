package service

import (
	"context"
	"fmt"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// Source supplies the history of one repository at one revision.
// The CLI reads a local checkout; the server reads its bare clones.
type Source interface {
	// Files lists the files present at the analysed revision.
	Files(ctx context.Context) ([]dto.File, error)
	// Generated lists files whose content marks them as generated.
	Generated(ctx context.Context) (map[string]bool, error)
	// Commits streams the history up to the revision, newest first.
	Commits(ctx context.Context, fn func(dto.Commit) error) error
}

// Analyze reads a whole history from src and returns the report.
func Analyze(ctx context.Context, src Source, cfg Config) (dto.Report, error) {
	files, err := src.Files(ctx)
	if err != nil {
		return dto.Report{}, fmt.Errorf("list files: %w", err)
	}
	gen, err := src.Generated(ctx)
	if err != nil {
		return dto.Report{}, fmt.Errorf("detect generated files: %w", err)
	}
	a := NewAnalyzer(cfg, files, gen)
	err = src.Commits(ctx, func(c dto.Commit) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.Add(c)
		return nil
	})
	if err != nil {
		return dto.Report{}, fmt.Errorf("read history: %w", err)
	}
	return a.Report(), nil
}
