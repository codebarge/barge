package service

import (
	"strings"
	"testing"
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

func TestQuestions(t *testing.T) {
	m := dto.HandoverModule{
		Path:          "internal/billing",
		CandidateName: "Anna",
		OwnFiles:      []dto.FileShare{{Path: "internal/billing/invoice.go", Share: 0.9, Lines: 120}},
		KeyCommits:    []dto.CommitRef{{Hash: "0123456789abcdef", Subject: "Rewrite `invoice` rounding", Time: base, Lines: 300}},
		Reverts:       []dto.CommitRef{{Hash: "fedcba9876543210", Subject: "Revert \"Use new tax API\"", Time: base.Add(-time.Hour)}},
	}
	qs := Questions(m)
	kinds := make([]string, len(qs))
	for i, q := range qs {
		kinds[i] = q.Kind
		if q.Text == "" || q.Prompt == "" {
			t.Errorf("question %d has empty text: %+v", i, q)
		}
	}
	want := []string{dto.QuestionPurpose, dto.QuestionCommit, dto.QuestionRevert, dto.QuestionFragile, dto.QuestionOps, dto.QuestionSuccessor}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	if c := qs[1]; c.Commit == nil || !strings.HasPrefix(c.Text, "0123456 ") || strings.Contains(c.Text, "`") {
		t.Errorf("commit question: %+v", c)
	}
	if f := qs[3]; len(f.Files) != 1 || !strings.Contains(f.Prompt, "invoice.go") {
		t.Errorf("fragile question: %+v", f)
	}
	if !strings.Contains(qs[5].Prompt, "Anna") {
		t.Errorf("successor question: %+v", qs[5])
	}

	tests := Questions(dto.HandoverModule{Path: "server/tests/e2e"})
	if len(tests) != 2 || !strings.Contains(tests[0].Prompt, "tests protect") || !strings.Contains(tests[1].Prompt, "flaky") {
		t.Errorf("test module questions: %+v", tests)
	}
}
