package service

import (
	"fmt"
	"path"
	"strings"
	"time"

	"gitlab.com/codebarge/barge/features/ownership/dto"
)

// Questions turns what the history says about one module into questions
// for the person who knows it: what it is for, why the largest changes
// were made the way they were, what went wrong in reverts, which of their
// files is the most fragile, what breaks in production, and what the
// successor should learn first. The CLI prints them as a checklist; the
// server stores them as an interview.
func Questions(m dto.HandoverModule) []dto.Question {
	tests := IsTestPath(m.Path)
	var qs []dto.Question
	add := func(q dto.Question) {
		if q.Text == "" {
			q.Text = q.Prompt
		}
		qs = append(qs, q)
	}

	if tests {
		add(dto.Question{Kind: dto.QuestionPurpose,
			Prompt: "What do these tests protect, and which code would break unnoticed without them?"})
	} else {
		add(dto.Question{Kind: dto.QuestionPurpose,
			Prompt: "What is this module responsible for, and what must never break in it?"})
	}
	for _, c := range m.KeyCommits {
		c := c
		add(dto.Question{
			Kind:   dto.QuestionCommit,
			Prompt: "Why was it done this way, and what would you do differently today?",
			Text: fmt.Sprintf("%s %q (%s, %d lines): why was it done this way, and what would you do differently today?",
				ShortHash(c.Hash), CleanSubject(c.Subject), c.Time.Format(time.DateOnly), c.Lines),
			Commit: &c,
		})
	}
	for _, c := range m.Reverts {
		c := c
		add(dto.Question{
			Kind:   dto.QuestionRevert,
			Prompt: "What went wrong, and what should someone know before trying again?",
			Text: fmt.Sprintf("%s %q (%s): what went wrong, and what should someone know before trying again?",
				ShortHash(c.Hash), CleanSubject(c.Subject), c.Time.Format(time.DateOnly)),
			Commit: &c,
		})
	}
	if len(m.OwnFiles) > 0 {
		files := make([]string, 0, len(m.OwnFiles))
		names := make([]string, 0, len(m.OwnFiles))
		for _, f := range m.OwnFiles {
			files = append(files, f.Path)
			names = append(names, path.Base(f.Path))
		}
		add(dto.Question{
			Kind:   dto.QuestionFragile,
			Prompt: fmt.Sprintf("Which of %s is the most fragile, and how do you test changes to it?", strings.Join(names, ", ")),
			Files:  files,
		})
	}
	if tests {
		add(dto.Question{Kind: dto.QuestionOps, Prompt: "Which tests are flaky or slow, and how do you run them locally?"})
	} else {
		add(dto.Question{Kind: dto.QuestionOps, Prompt: "What breaks most often here in production, and how do you notice?"})
	}
	if m.CandidateName != "" {
		add(dto.Question{Kind: dto.QuestionSuccessor,
			Prompt: fmt.Sprintf("What should %s learn first to own this module?", m.CandidateName)})
	}
	return qs
}

// IsTestPath reports whether a module holds tests rather than product code.
func IsTestPath(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		switch strings.ToLower(seg) {
		case "test", "tests", "spec", "specs", "__tests__", "e2e", "integration_tests":
			return true
		}
	}
	return false
}

// ShortHash is the 7-character form of a commit hash.
func ShortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// CleanSubject keeps a commit subject short and free of backticks, so it
// does not break Markdown or a prompt around it.
func CleanSubject(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "`", "'"))
	if r := []rune(s); len(r) > 100 {
		s = string(r[:99]) + "…"
	}
	return s
}
