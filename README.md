# Barge

**Find the code only one person knows — before they leave.**

`barge` reads your git history and shows, for every module, who knows it,
how many people would have to leave before nobody does (the *bus factor*), and
how much of it was written by AI agents. It runs locally: no code, names or
history leave your machine.

Full reference: **[DOCUMENTATION.md](DOCUMENTATION.md)**: every command and
flag, `.barge.json`, how the analysis works, JSON output, CI and
troubleshooting. What changed: [CHANGELOG.md](CHANGELOG.md).

## Install and run

```sh
go install gitlab.com/codebarge/barge/cmd/barge@latest
barge scan ~/code/your-project
```

Or download a binary for Linux, macOS or Windows from the
[releases page](https://gitlab.com/codebarge/barge/-/releases) (the same
releases are on [GitHub](https://github.com/codebarge/barge/releases)).

`barge help` lists the commands, `barge help scan` (or `barge scan -h`) the
flags of one, and `barge version` shows the version and build details
(`--short` for scripts, `--json` for tools). Colours turn off in pipes and with
`NO_COLOR`; `CLICOLOR_FORCE=1` keeps them, for example in CI logs.

The output looks like this (names and numbers are illustrative):

```
Barge dispatcher · main @ 72dd0fb · 2,619 commits · 12 people, 5 active · as of 2026-10-03

RISK      BUS    AI  MODULE                                  WHO KNOWS IT
critical    0     –  internal/core/logger                    Dmitry Lysenko 92% (inactive)
high        1     –  internal/features/payments/service      Oleg Koval 86%, Anna Melnyk 9%
high        1   62%  internal/core/transport/http/middleware Max Bondarenko 71%
...
```

Every scan ends with **What to do next**: concrete steps such as *make Anna a
required reviewer of `payments/service` for a month*, or *run a handover for
the person who is the only one to know 21 modules*. Successors are people who
already worked on the code or next to it, and people who are already the
bottleneck are not suggested.

### Handover checklist

```sh
barge handover oleg@example.com ~/code/your-project -o handover.md
```

Builds a Markdown checklist for one person: every module they are a key owner
of, what its bus factor becomes when they leave, a suggested successor, the
files they wrote, and questions about their largest commits and the reverts
in their code. Go through it together before someone leaves, or better, long
before anyone does.

Useful flags:

| Flag | Effect |
| --- | --- |
| `--leaving oleg@example.com` | What happens if Oleg leaves (repeatable) |
| `--fail-on high` | Exit code 1 if any module is high risk or worse: use it in CI |
| `--json` | Full report, including people and directory groups |
| `--only 'pkg/*'` | Show only the modules under matching directories |
| `--module 'internal/features/*'` | Treat each feature directory as one module |
| `--depth 2` | Group files by their first two directories |
| `--all`, `--top 0` | Show everything |

Exit codes: `0` fine, `1` `--fail-on` threshold reached, `2` error.

### `.barge.json`

Commit this file at the repository root to configure the CLI and the server
the same way. Unknown keys are rejected.

```json
{
  "modules": ["internal/features/*"],
  "exclude": ["docs/**", "*.sql"],
  "halfLifeDays": 180,
  "activeDays": 180,
  "people": [{ "email": "oleg@example.com", "status": "leaving", "date": "2026-10-14" }],
  "owners": [
    { "module": "internal/billing", "email": "anna@example.com", "knows": true },
    { "module": "internal/billing", "email": "max@example.com", "knows": false }
  ]
}
```

`owners` corrects the history where it misleads. `"knows": true` marks
someone who knows a module without having written much of it (they review
every change, or took it over in a handover): while they are active, they
count as able to keep the module going on their own. `"knows": false` ignores
someone's changes in a module, for example a one-off formatting run.

## How the analysis works

- Every change a person makes to a file adds `log(1 + changed lines)` to their
  weight in the file's module. The logarithm stops one huge generated or moved
  file from outweighing years of careful work.
- Weight halves every 180 days: people remember recent work better.
- Renames are followed, deleted files are ignored, and dependencies, lock
  files, generated code (`Code generated ... DO NOT EDIT`, `@generated`) and
  build output are excluded.
- Bots are ignored. AI agents committing on their own own nothing; commits
  made with an AI assistant count half for the human. The share of AI-written
  lines is reported per module.
- Commits touching more than 200 files (formatting, licence headers) count
  with a tenth of the weight.
- Aliases are merged: `.mailmap`, GitHub noreply addresses and identical names.
- **Bus factor**: how many present people must leave before those who remain
  hold less than 40% of the module's knowledge. People without commits for
  180 days, or marked as left, are not present.

Risk levels: **critical** — nobody present knows the module (or its knowledge
leaves with someone marked as leaving); **high** — bus factor 1; **medium** —
bus factor 2, or more than half of recent changes came from AI; **low** — the
rest.

## Library

The analysis is a plain Go package with no dependencies, usable on its own:

```go
import (
    "gitlab.com/codebarge/barge/core/git"
    "gitlab.com/codebarge/barge/features/ownership/repository/gitsource"
    "gitlab.com/codebarge/barge/features/ownership/service"
)

repo, _ := git.Open(ctx, ".")
report, _ := service.Analyze(ctx, gitsource.New(repo, "HEAD"), service.DefaultConfig())
```

## Team version

Barge for teams adds a shared dashboard, alerts when a module becomes risky,
and short AI-assisted interviews that capture what someone knows before they
leave. Interested in a pilot?
**[Request one](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Pilot%20request)**: the request is confidential, only
you and the maintainers see it.

## Code layout

Same three layers as the server: transport → service → repository.

```
cmd/barge/                           flags, help pages, exit codes
core/git/                            git via os/exec: log parser, files, clone, fetch
features/ownership/
  dto/                               input and output types
  service/                           the analysis: pure functions, no I/O
  repository/gitsource/              reads history from a git repository
  transport/cli/                     terminal output
```

## Contributing and licence

[Report a bug](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Bug%20report) ·
[suggest a feature](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Feature%20request) ·
[ask a question](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Question). Merge requests are welcome; see
[CONTRIBUTING.md](CONTRIBUTING.md). Issues and merge requests live on GitLab;
the GitHub repository is a read-only mirror.
Licensed under the [Apache License 2.0](LICENSE).
