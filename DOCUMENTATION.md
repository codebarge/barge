# Barge CLI documentation

Barge reads the git history of a repository and shows, for every module,
who knows it, how many people would have to leave before nobody does (the
*bus factor*), and what to do about it. Before someone leaves, it builds a
handover checklist of what only they know.

Everything runs on your machine. Barge reads the local git history and
nothing else: no code, names or history are sent anywhere, and there is no
telemetry.

**Contents**

1. [Install](#1-install)
2. [Quick start](#2-quick-start)
3. [Commands](#3-commands)
   - [barge scan](#barge-scan)
   - [barge handover](#barge-handover)
   - [barge version](#barge-version)
   - [barge help](#barge-help)
4. [Reading the results](#4-reading-the-results)
5. [Analysis flags](#5-analysis-flags)
6. [Modules: how files are grouped](#6-modules-how-files-are-grouped)
7. [People: aliases, bots, AI and departures](#7-people-aliases-bots-ai-and-departures)
8. [The .barge.json file](#8-the-bargejson-file)
9. [How the analysis works](#9-how-the-analysis-works)
10. [JSON output](#10-json-output)
11. [Continuous integration](#11-continuous-integration)
12. [Exit codes and environment variables](#12-exit-codes-and-environment-variables)
13. [Troubleshooting](#13-troubleshooting)
14. [FAQ](#14-faq)

---

## 1. Install

You need **git** in your `PATH`.

### With Go

Go 1.24 or newer:

```sh
go install gitlab.com/codebarge/barge/cmd/barge@latest
```

`go install` puts the program in `$(go env GOPATH)/bin`, usually `~/go/bin`.
If your shell cannot find `barge` afterwards, add that directory to `PATH`
once:

```sh
# bash
echo 'export PATH="$PATH:$HOME/go/bin"' >> ~/.bashrc && source ~/.bashrc
# zsh
echo 'export PATH="$PATH:$HOME/go/bin"' >> ~/.zshrc && source ~/.zshrc
```

### Ready-made binaries

Download the archive for your system from the
[releases page](https://gitlab.com/codebarge/barge/-/releases) or its copy on
[GitHub](https://github.com/codebarge/barge/releases): Linux (x86-64,
ARM64), macOS (Intel, Apple Silicon) and Windows (x86-64). Each release has a
`checksums.txt`; check the download with:

```sh
sha256sum -c checksums.txt --ignore-missing
```

Unpack it and put `barge` (`barge.exe` on Windows) somewhere in your `PATH`,
for example `/usr/local/bin` or `~/.local/bin`.

### From source

```sh
git clone https://gitlab.com/codebarge/barge.git
cd barge
go build -o barge ./cmd/barge   # a binary in this folder: run it as ./barge
go install ./cmd/barge          # or install it into ~/go/bin
```

### Check the installation

```sh
barge version
```

### Uninstall

Barge is a single file and keeps no data. Delete it:

```sh
rm "$(command -v barge)"
```

---

## 2. Quick start

```sh
cd ~/code/your-project
barge scan
```

```
Barge your-project · main @ 72dd0fb · 2,619 commits · 12 people, 5 active · as of 2026-10-03

RISK      BUS    AI  MODULE                               WHO KNOWS IT
critical    0     –  internal/core/logger                 Dmitry Lysenko 92% (inactive)
high        1     –  internal/features/payments/service   Oleg Koval 86%, Anna Melnyk 9%
high        1   62%  internal/core/transport/http/mw      Max Bondarenko 71%
...

What to do next:
  1. internal/core/logger: nobody active knows it any more (Dmitry Lysenko was the main author).
     Make Anna Melnyk its owner and ask for a short README, or delete it if it is no longer used.
  ...
```

Then try:

```sh
barge scan --leaving oleg@example.com            # what if Oleg leaves?
barge handover oleg@example.com -o handover.md   # his handover checklist
barge scan --only 'internal/features/*'          # only part of the repository
```

---

## 3. Commands

```
barge <command> [flags] [path]
```

| Command | What it does |
| --- | --- |
| `barge scan [flags] [path]` | Who knows each module, the risk, and what to do next |
| `barge handover [flags] <email> [path]` | A handover checklist for one person, in Markdown |
| `barge version` | Version and build details |
| `barge help [command]` | Help for all commands or for one |

`path` is the repository folder and defaults to the current folder. Any
folder inside a repository works too. Barge needs a local clone: for a remote
repository, run `git clone` first and pass the folder.

Flags and the path can come in any order (`barge scan ~/code --top 5` and
`barge scan --top 5 ~/code` are the same). Flags take one value each; to give
several, repeat the flag (`--only a --only b`) or separate the values with
commas (`--only a,b`). Both `-flag` and `--flag` work.

### barge scan

Analyses one repository and prints its modules from the riskiest, then
concrete next steps.

```sh
barge scan [flags] [path]
```

**Output flags**

| Flag | Default | What it does |
| --- | --- | --- |
| `--only GLOB` | | Show only modules under directories matching the glob, e.g. `'pkg/*'` or `cmd/compose`. Repeatable |
| `--top N` | `25` | Show at most N modules; `0` shows all |
| `--all` | off | Also list low-risk modules |
| `--json` | off | Print the full report as JSON (see [JSON output](#10-json-output)) |
| `--fail-on LEVEL` | | Exit with code 1 if any module is at this risk or worse: `critical`, `high` or `medium` |
| `--no-color` | off | No colours |

`scan` also takes every [analysis flag](#5-analysis-flags).

**Reading the output.** The header, the table (`RISK`, `BUS`, `AI`,
`MODULE`, `WHO KNOWS IT`), the percentages, the summary and the next steps
are explained line by line in [Reading the results](#4-reading-the-results).

By default the table shows modules at `medium` risk or worse, at most 25. A
hint under the table says how many more there are.

**Examples**

```sh
barge scan                                       # the repository you are in
barge scan ~/code/shop --all --top 0             # every module, low risk included
barge scan --only 'pkg/*'                        # only modules under pkg/
barge scan --only pkg/api,cmd/compose            # two parts of the repository
barge scan --leaving oleg@example.com            # simulate Oleg leaving
barge scan --module 'internal/features/*'        # each feature counts as one module
barge scan --rev origin/develop --as-of 2026-07-01   # develop as it was on 1 July
barge scan --json > report.json                  # everything, for scripts
barge scan --fail-on high                        # for CI: fail on high risk
```

### barge handover

Builds a Markdown checklist for one person: what only they know and how to
pass it on. Run it before someone leaves, or better, long before anyone does.

```sh
barge handover [flags] <email> [path]
```

`<email>` is any address the person committed with. Not sure which one? Run
`barge scan --json` and look under `people`: each person lists all their
addresses in `emails`.

| Flag | Default | What it does |
| --- | --- | --- |
| `-o FILE` | the terminal | Write the checklist to a file, e.g. `handover.md` |
| `--max-modules N` | `10` | Include at most N modules, most at risk first; `0` includes all |
| `--json` | off | JSON instead of Markdown |

`handover` also takes every [analysis flag](#5-analysis-flags).

**What the checklist contains**

1. A table of every module the person is a **key owner** of (holds at least
   20% of its knowledge): their share, the bus factor and risk once they are
   gone, and a suggested successor.
2. For each module:
   - the files mostly written by the person;
   - **questions for the person**, as checkboxes:
     - what the module is responsible for and what must never break;
     - for each of their largest commits: why it was done this way;
     - for each revert in their code: what went wrong;
     - which of their files is the most fragile and how to test it;
     - what breaks most often in production (for tests: which are flaky);
     - what the successor should learn first;
   - **tasks for the successor**: pair on the next change, review every change
     until the person leaves, write the answers into the module's README.

If the person is not a key owner of anything, the checklist says that no
handover is needed.

**Examples**

```sh
barge handover oleg@example.com                          # print to the terminal
barge handover oleg@example.com -o handover.md           # write to a file
barge handover oleg@example.com ~/code/shop --max-modules 0
```

### barge version

```sh
barge version            # logo, version, commit, build date, Go version, platform
barge version --short    # only the version number, for scripts: v0.3.0
barge version --json     # build details as JSON
barge --version          # one line: barge v0.3.0  (also -v)
```

The version is written in the source, so every build reports it: a release
binary, `go install …@vX.Y.Z` and a build from a source folder alike. A build
from a git checkout also shows the commit and build date (`(modified)` when
the checkout had uncommitted changes). What changed in each version is in
[CHANGELOG.md](CHANGELOG.md).

### barge help

```sh
barge help               # all commands (also: barge, barge -h, barge --help)
barge help scan          # one command with all its flags (also: barge scan -h)
```

Words in capitals in the help, such as `GLOB`, `EMAIL` or `N`, stand for your
own value: `--module GLOB` means `--module 'internal/features/*'`. Do not type
the word itself.

---

## 4. Reading the results

This section walks through everything `barge scan` prints, line by line,
using a real scan of [docker/compose](https://github.com/docker/compose)
as the example.

```
Barge compose · main @ 42f4807 · 3,169 commits · 257 people, 26 active · as of 2026-10-02

RISK      BUS    AI  MODULE                  WHO KNOWS IT
critical    0     –  internal/paths          tensorworker 95% (inactive), Milas Bowman 5% (inactive)
high        1   30%  pkg/compose             Nicolas De Loof 56%, Guillaume Lours 27%, Sebastiaan van Stijn 8%
high        1   28%  relay                   Nicolas De Loof 100%
high        1     –  internal/pidfile        Guillaume Lours 100%
medium      2   20%  cmd/compose             Nicolas De Loof 52%, Guillaume Lours 26%, Sebastiaan van Stijn 12%

38 modules: 1 critical · 23 high · 14 medium · 0 low
Repository bus factor: 1 (people who must leave before the rest know less than 40% of the code)

What to do next:
  1. internal/paths: nobody active knows it any more (tensorworker was the main author).
     Make Endika Iglesias its owner and ask for a short README, or delete it if it is no longer used.
  2. Nicolas De Loof is the only one who knows 14 modules. Don't wait for a resignation letter.
     Run barge handover nicolas.deloof@gmail.com and pair someone on the largest ones:
     pkg/compose (hiroto.toyoda), pkg/e2e (hiroto.toyoda), pkg/api (Max Proske).
  ...

Showing 25 of 38 modules; --top 0 shows all.
Simulate a departure: barge scan --leaving <email>  ·  Handover checklist: barge handover <email>
Scanned in 800ms. Nothing left this machine.
```

### The header line

`Barge compose · main @ 42f4807 · 3,169 commits · 257 people, 26 active · as of 2026-10-02`

| Part | Meaning |
| --- | --- |
| `compose` | The repository folder's name |
| `main @ 42f4807` | The branch (or the revision given with `--rev`) and the commit analysed |
| `3,169 commits` | Commits up to that commit that changed at least one analysed file. Commits by bots (Dependabot, Renovate and the like) and commits touching only [ignored files](#6-modules-how-files-are-grouped) are not counted |
| `257 people` | Everyone who ever committed, after merging the addresses of one person into one |
| `26 active` | People with a commit in the last 180 days before the reference date (`--active-days`), plus anyone marked `active` in `.barge.json` |
| `as of 2026-10-02` | The reference date: the date of the newest commit, or `--as-of`. "Last 180 days" and the fading of old changes are counted back from this date, not from today |

### The table, column by column

**`RISK`**: how urgent the module is: `critical`, `high`, `medium` or `low`.
It follows from the bus factor, with one extra rule for AI-written code. See
[risk levels](#risk-levels) and the table below.

**`BUS`**: the module's **bus factor**: how many of the people who are still
here would have to leave before nobody who remains knows the module well
enough to maintain it. The smaller, the worse.

| BUS | Means | Risk |
| --- | --- | --- |
| `0` | **Already nobody.** Everyone who knew the module is inactive or gone | `critical` |
| `1` | **One person.** If they leave, it becomes `0` | `high` |
| `2` | **Two people** must both leave before it is lost | `medium` |
| `3` or more | Knowledge is spread out | `low` |

**`AI`**: the share of the module's recent changes that came from commits made
with an AI assistant or by an AI agent. Changes are weighted the same way as
knowledge: recent ones count more (see [how the analysis works](#9-how-the-analysis-works)).
`–` means less than 5%. At 50% or more a module is at least `medium`, whatever
its bus factor, because code an agent wrote is understood less deeply by the
people around it.

**`MODULE`**: the module's path from the repository root: usually a directory
(see [Modules](#6-modules-how-files-are-grouped)). `.` is the files at the
root. Very long paths are shortened with `…` at the start; `--json` has them
in full.

**`WHO KNOWS IT`**: the people who know the module best, with their **share**
of its knowledge. Shown are up to three people with at least 5%, highest
first; anyone confirmed in `.barge.json` is always shown. `nobody` means no
one has a share worth showing.

### What the percentages mean

A share is how much of the module's knowledge one person holds, compared with
everyone who ever worked on it. It is not lines of code:

- every change counts by its size, but large changes count less than
  proportionally (`log(1 + lines)`), so one huge generated or moved file does
  not outweigh years of small careful changes;
- old changes fade: a change loses half its weight every 180 days
  (`--half-life`), because people forget;
- changes made with an AI assistant count half for the person;
- changes touching more than 200 files at once (formatting, licence headers,
  renames) count a tenth.

So `Nicolas De Loof 56%` means: of everything people know about
`pkg/compose`, judged by the size and age of their changes, 56% sits with
Nicolas.

The percentages in a row **do not add up to 100%**: only the top three with
5% or more are shown, and the rest is spread over people below that. People
who are inactive or have left still have a share (it shows what they took
with them), but they do not count towards the bus factor.

### How the bus factor is counted, step by step

1. Take the people who are **present**: active, not marked as leaving or left.
2. Remove them one at a time, the largest share first.
3. After each removal, add up the shares of the present people who remain.
4. The bus factor is how many had to go before that sum fell **below 40%**.
   If the present people hold less than 40% from the start, it is `0`.

Why 40% and not 50%: two people who split a module 52/48 both know it well;
with 50% the second person would not be enough, and the module would wrongly
look like a one-person module.

Three rows from the example:

**`internal/paths`: tensorworker 95% (inactive), Milas Bowman 5% (inactive) → BUS 0, critical**

Both people who knew it are inactive. The present people hold 0%, which is
already below 40%, so nobody needs to leave: the knowledge is gone. Someone
has to take this module over, or it should be deleted if nothing uses it.

**`relay`: Nicolas De Loof 100% → BUS 1, high**

| Step | Who leaves | Present people still hold | Known? |
| --- | --- | --- | --- |
| now | — | 100% | yes |
| 1 | Nicolas | 0% | no |

One departure is enough, so the bus factor is 1.

**`cmd/compose`: Nicolas 52%, Guillaume 26%, Sebastiaan 12%, others ≈10% → BUS 2, medium**

| Step | Who leaves | Present people still hold | Known? |
| --- | --- | --- | --- |
| now | — | ≈100% | yes |
| 1 | Nicolas | ≈48% | yes: 48% is not below 40% |
| 2 | Guillaume | ≈22% | no |

Two people have to leave, so the bus factor is 2.

### Why 100% is "high" and not "critical"

`critical` means the knowledge is **already lost**: nobody who is still here
knows the module. `high` means it is **one departure away** from that.

`relay` is known 100% by Nicolas, but Nicolas is active: today someone can
answer questions about it and fix it. That is a risk (`high`), not yet a loss
(`critical`). If Nicolas leaves, every module where he is the only one becomes
`critical`. You can see this before it happens:

```sh
barge scan --leaving nicolas.deloof@gmail.com
```

Marking someone as leaving turns their `high` modules `critical` with the
reason `owner_leaving`, so the list shows exactly what to hand over first.

### The marks after names

| Mark | Meaning |
| --- | --- |
| `(inactive)` | No commits in the 180 days before the reference date (`--active-days`). Not counted as present |
| `(leaving)` | Marked as leaving with `--leaving` or in `.barge.json`. Not counted as present: the scan shows the situation after they are gone |
| `(left)` | Marked as gone with `--left` or in `.barge.json` |
| `(confirmed)` | Confirmed in `.barge.json` (`"knows": true`) as knowing the module, whatever the history says. While present, they count as holding at least 40% |
| no mark | Active |

### The summary

`38 modules: 1 critical · 23 high · 14 medium · 0 low` counts **every**
module of the repository, also those not shown in the table. With `--only`,
it counts only the modules that match.

`Repository bus factor: 1` is the same count for the repository as a whole:
how many people must leave before those who remain know less than 40% of all
the code. When it reads `(people active today know less than 40% of the code)`,
the bus factor is 0: most of the code was written by people who are no longer
active.

A repository bus factor of 1 or 2 is common in teams with one or two core
authors. It is a reason to spread knowledge, not a judgement of the people.

### What to do next

Up to five concrete steps, most urgent first. There are four kinds:

| Step | When it appears | What it asks |
| --- | --- | --- |
| **Handover** | Someone marked as leaving holds at least 20% of some modules | Run `barge handover <email>` now; it tells how many modules would have nobody |
| **Assign an owner** | A module is `critical` because nobody active knows it (when it is `critical` because its owner is leaving, the handover step covers it) | Make the suggested person its owner and ask for a short README, or delete the module if it is no longer used |
| **Add a second owner** | A module is `high`: one person knows it | Make the suggested person a required reviewer there for a month. When one person is the only one for several modules, these are grouped into one step: run their handover and pair someone on the largest modules |
| **Review AI-written code** | At least half of a module's recent changes came from AI | Walk the team through it so more than one person understands it |

**Who is suggested.** For each module, Barge suggests a present person who
already worked on that module or, failing that, on the code around it, since
they learn it fastest. People who are already the only one to know three or
more modules are skipped while anyone else fits, so the bottleneck is not
made worse. When nobody fits, the step says "pick a second person".

`…and 1 more; see "actions" in --json` means there are more steps than
shown; `barge scan --json` lists them all.

### The lines at the bottom

| Line | Meaning |
| --- | --- |
| `Showing 25 of 38 modules; --top 0 shows all.` | The table stopped at `--top` (25). The summary still counts all |
| `5 low-risk modules hidden; --all shows them.` | `low` modules are not listed unless you pass `--all` |
| `Only modules under …: 2 of 38.` | Shown under the header when `--only` is used |
| `Simulate a departure: …` | A reminder of the two most useful next commands |
| `Scanned in 800ms. Nothing left this machine.` | How long the scan took, and a reminder that nothing was sent anywhere |

Colours follow the risk: red for `critical`, orange for `high`, yellow for
`medium`, grey for `low`. They are a help, never the only signal: the risk
is always written as a word.

### What to do with the results

1. **`critical` first.** Find an owner for each, or delete the module if it is
   dead code. A short README written by the new owner is the cheapest fix.
2. **Then the people who hold many `high` modules.** Run their handover
   before anyone is leaving, and pair a second person on the largest modules.
3. **Simulate departures** of your key people with `--leaving` to see where
   you are most exposed.
4. **Correct the history where it misleads** with `owners` in `.barge.json`:
   a reviewer who knows a module without writing it, or a mass refactoring
   that does not mean knowledge.
5. **Watch it in CI** with `--fail-on critical`, so new `critical` modules do
   not appear unnoticed.

---

## 5. Analysis flags

`scan` and `handover` share these flags. Flags win over `.barge.json`.

| Flag | Default | What it does |
| --- | --- | --- |
| `--rev REV` | `HEAD` | Revision to analyse: a branch, tag or commit, e.g. `main` or `origin/develop` |
| `--as-of DATE` | date of the newest commit | Reference date `YYYY-MM-DD` for decay and activity: "the repository as it was on that day" |
| `--module GLOB` | | Count each directory matching the glob as one module. Repeatable. See [Modules](#6-modules-how-files-are-grouped) |
| `--depth N` | `0` | Group files by their first N directories; `0` groups by the directory that contains each file |
| `--exclude GLOB` | | Ignore matching files, e.g. `'docs/**'` or `'*.sql'`. Repeatable |
| `--leaving EMAIL` | | Someone who is leaving: see what happens. Repeatable |
| `--left EMAIL` | | Someone who already left. Repeatable |
| `--half-life DAYS` | `180` | Days for a change to lose half its weight |
| `--active-days DAYS` | `180` | People without commits for this many days count as inactive |
| `--no-config` | off | Ignore `.barge.json` |

**Globs** work the same everywhere (`--module`, `--only`, `--exclude`,
`.barge.json`):

- A pattern **without** a slash matches a name anywhere: `service` matches
  every directory called `service`, `*.pb.go` every file ending in `.pb.go`.
- A pattern **with** a slash matches a path from the repository root: `pkg/*`,
  `internal/features/*`.
- `*` matches within one path segment, `**` any number of segments:
  `docs/**` is everything under `docs`.
- A trailing slash means "everything beneath": `legacy/` is `legacy/**`.

Quote patterns with `*` so the shell does not expand them: `'pkg/*'`.

---

## 6. Modules: how files are grouped

A **module** is the unit people own. Each file belongs to exactly one module,
decided in this order:

1. The **shallowest directory matching a `--module` rule**: with
   `--module 'internal/features/*'`, everything under
   `internal/features/users/` is the module `internal/features/users`.
2. Else, with `--depth N`, the **first N directories** of the path:
   `--depth 2` turns `internal/core/git/log.go` into `internal/core`.
3. Else, the **directory that contains the file**, so every package is its
   own module. Files at the root form the module `.`.

Directories above modules are **groups**: they add up the knowledge of the
modules beneath them. Groups appear in `--json`, and the root group gives the
repository bus factor.

**`--module`, `--depth` and `--only` do different things**

| Flag | Changes | Example |
| --- | --- | --- |
| `--module` | how files are **grouped** into modules | `--module 'internal/features/*'`: each feature, with all its subdirectories, is one module |
| `--depth` | how deep the default grouping goes | `--depth 2`: `pkg/compose/transform` and `pkg/compose` both become `pkg/compose` |
| `--only` | which modules are **shown** | `--only 'pkg/*'`: the analysis is the same, only the modules under `pkg/` are listed |

If a `--module` rule matches no directory, Barge says so: usually it is a typo
or a directory this repository does not have.

`--only` keeps a module when its path, or a directory above it, matches: so
`--only pkg/compose` shows `pkg/compose` and `pkg/compose/transform`. The
summary and the next steps cover only the shown modules; the repository bus
factor stays that of the whole repository. `--fail-on` checks only the shown
modules, which lets CI watch one part of a monorepo.

**Ignored files.** Besides your `--exclude` patterns, Barge always ignores
files that carry no human knowledge:

- dependencies: `vendor/`, `node_modules/`, `third_party/`;
- lock files: `go.sum`, `*.lock`, `package-lock.json`, `pnpm-lock.yaml`,
  `npm-shrinkwrap.json`, `gradle.lockfile`;
- generated code: `*.pb.go`, `*.pb.gw.go`, `*_gen.go`, `*.gen.go`,
  `*_generated.go`, `zz_generated*`, `*.generated.*`, `*_pb2.py`,
  `*_pb2_grpc.py`, and any file marked `Code generated ... DO NOT EDIT` or
  `@generated`;
- build output: `dist/`, `*.min.js`, `*.min.css`, `*.map`;
- test data: `testdata/`, `fixtures/`, `__fixtures__/`, `test-fixtures/`,
  `__snapshots__/`, `*.snap`;
- binary files and files deleted before the analysed revision.

Renamed files keep their history.

---

## 7. People: aliases, bots, AI and departures

**One person, many addresses.** Barge merges a person's identities by:

- your repository's `.mailmap`;
- GitHub noreply addresses (`123+login@users.noreply.github.com` is `login`);
- identical names.

The person's name is the one they used most.

**Bots** are ignored entirely: accounts with `[bot]` in the name or address,
and known automation such as Dependabot, Renovate, GitHub Actions, GitLab CI,
semantic-release, Snyk and pre-commit.ci.

**AI.** Commits made by an AI agent on its own (for example authored as
Claude, Copilot or Cursor) give knowledge to nobody. Commits a person made
with an AI assistant count **half** for that person: code an agent wrote is
understood less deeply than code typed by hand. Barge recognises AI help from
`Co-authored-by`, `Assisted-by` and `Generated-by` trailers naming an AI tool,
"Generated with …" footers, and aider's `(aider)` author suffix. The `AI`
column shows the share of recent changed lines from such commits.

**Who is present.**

| Status | How a person gets it | Counts towards the bus factor |
| --- | --- | --- |
| `active` | committed within `--active-days` (180) before the reference date | yes |
| `inactive` | no commits within that window | no |
| `leaving` | `--leaving EMAIL` or `"status": "leaving"` in `.barge.json` | no: this is the "what if" |
| `left` | `--left EMAIL` or `"status": "left"` | no |

You can also mark someone `active` in `.barge.json`: for example a lead who
reviews everything but rarely commits.

---

## 8. The .barge.json file

Commit a `.barge.json` at the repository root to set things once for
everyone. The CLI and Barge for teams read it the same way. Unknown keys are
an error, so a typo does not silently do nothing. Pass `--no-config` to
ignore the file.

```json
{
  "modules": ["internal/features/*"],
  "exclude": ["docs/**", "*.sql"],
  "depth": 0,
  "halfLifeDays": 180,
  "activeDays": 180,
  "people": [
    { "email": "oleg@example.com", "status": "leaving", "date": "2026-10-14" },
    { "email": "lead@example.com", "status": "active" }
  ],
  "owners": [
    { "module": "internal/billing", "email": "anna@example.com", "knows": true },
    { "module": "internal/billing", "email": "max@example.com", "knows": false }
  ]
}
```

| Key | Type | Same as | Meaning |
| --- | --- | --- | --- |
| `modules` | list of globs | `--module` | Directories that each form one module |
| `exclude` | list of globs | `--exclude` | Files to ignore |
| `depth` | number | `--depth` | Group by the first N directories |
| `halfLifeDays` | number | `--half-life` | Days for a change to lose half its weight |
| `activeDays` | number | `--active-days` | Days without commits before someone counts as inactive |
| `people` | list | `--leaving`, `--left` | Statuses: `email` (required), `status` (`active`, `leaving`, `left`, `inactive`), `date` (`YYYY-MM-DD`, optional) |
| `owners` | list | | Corrections to the history, see below |

**How it combines with flags.** Lists (`modules`, `exclude`, `people`) add to
the flags; a person given both in the file and by a flag gets the flag's
status. Numbers from the file apply only when the flag is left at its
default.

**`owners`: when the history misleads.** Every entry needs `module`, `email`
and `knows`.

- `"knows": true`: this person knows the module even though they wrote little
  of it (they review every change, or took it over in a handover). While they
  are present, the module counts as maintainable by them alone, and the output
  marks them `(confirmed)`.
- `"knows": false`: ignore this person's changes in the module, for example a
  one-off formatting run or a mass rename.

`module` is a module path as Barge shows it, e.g. `internal/billing`.

---

## 9. How the analysis works

**Knowledge.** Every change a person makes to a file adds
`log(1 + changed lines)` to their weight in the file's module. The logarithm
stops one huge generated or moved file from outweighing years of careful work.

**Memory fades.** A change loses half its weight every 180 days
(`--half-life`): people remember what they changed last month better than
what they wrote three years ago.

**Mass changes** touching more than 200 files (formatting, licence headers,
renames) count with a tenth of the weight.

**Share.** A person's share of a module is their weight divided by everyone's.

**Bus factor.** Take the people who are present, from the largest share down,
and remove them one by one. The bus factor is how many must leave before the
rest hold **less than 40%** of the module's knowledge. 40% rather than 50%
keeps two people who split a module 52/48 at bus factor 2.

**Key owner.** Someone with at least 20% of a module's knowledge.

**Reproducible.** Without `--as-of`, the reference date is the date of the
newest commit, not today: the same revision always gives the same result, and
a repository nobody touched for a year is not reported as abandoned by all.

### Risk levels

| Risk | When |
| --- | --- |
| `critical` | Nobody present knows the module (bus factor 0), or its knowledge leaves with someone marked as leaving |
| `high` | Bus factor 1: one person holds it |
| `medium` | Bus factor 2, or at least half of recent changes came from AI-assisted commits |
| `low` | Everything else |

### Reasons

Each module in `--json` lists why it got its risk:

| Reason | Meaning |
| --- | --- |
| `orphaned` | No present person knows the module |
| `owner_leaving` | Someone leaving holds at least 20% of it |
| `single_owner` | Bus factor 1 |
| `two_owners` | Bus factor 2 |
| `ai_heavy` | At least half of recent changed lines came from AI-assisted commits |
| `low_history` | Fewer than 3 commits: too little history to be sure |

---

## 10. JSON output

`barge scan --json` prints the whole report; nothing is cut by `--top` or
`--all`. `--only` does narrow it.

```json
{
  "asOf": "2026-10-03T14:12:00Z",
  "commits": 2619,
  "people": [
    {
      "id": "oleg@example.com",
      "name": "Oleg Koval",
      "email": "oleg@example.com",
      "emails": ["oleg@example.com", "oleg@old-company.com"],
      "commits": 812,
      "lastCommit": "2026-10-01T09:30:00Z",
      "status": "active"
    }
  ],
  "modules": [
    {
      "path": "internal/features/payments/service",
      "parent": "internal/features/payments",
      "kind": "module",
      "files": 14,
      "bytes": 48210,
      "commits": 133,
      "owners": [
        { "personId": "oleg@example.com", "name": "Oleg Koval", "share": 0.86,
          "lastTouch": "2026-09-28T11:00:00Z", "status": "active" }
      ],
      "busFactor": 1,
      "aiShare": 0.02,
      "risk": "high",
      "reasons": ["single_owner"]
    }
  ],
  "summary": { "modules": 38, "critical": 1, "high": 23, "medium": 14, "low": 0,
               "orphaned": 1, "singleOwner": 23 },
  "actions": [
    { "kind": "add_second_owner", "module": "internal/features/payments/service", "risk": "high",
      "ownerId": "oleg@example.com", "ownerName": "Oleg Koval",
      "candidateId": "anna@example.com", "candidateName": "Anna Melnyk", "bytes": 48210 }
  ]
}
```

- `modules` lists modules first (worst risk first), then groups (`"kind": "group"`).
  The group with path `.` is the whole repository.
- `share` and `aiShare` are fractions from 0 to 1.
- `actions[].kind` is one of `handover`, `assign_owner`, `add_second_owner`,
  `review_ai_code`.

`barge handover --json` prints the person and, for each module, `share`,
`busFactorAfter`, `riskAfter`, the suggested successor, `ownFiles`,
`keyCommits` and `reverts`.

Field names are stable within a major version; new fields may be added.

---

## 11. Continuous integration

Barge needs the **full history**: CI systems often clone only the last
commit, which makes everyone look like a newcomer. Turn that off.

**GitLab CI**

```yaml
bus-factor:
  image: golang:1.26
  variables:
    GIT_DEPTH: 0            # full history
  script:
    - go install gitlab.com/codebarge/barge/cmd/barge@latest
    - barge scan --fail-on critical --no-color
```

**GitHub Actions**

```yaml
name: bus factor
on: [pull_request]
jobs:
  barge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0    # full history
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go install gitlab.com/codebarge/barge/cmd/barge@latest
      - run: barge scan --fail-on critical --no-color
```

Tips:

- Start with `--fail-on critical`; tighten to `high` once the critical
  modules are dealt with, or the job fails from day one.
- In a monorepo, watch only your part: `--only 'services/payments/**'`.
- Keep the full report as an artifact: `barge scan --json > barge.json`.
- Set `CLICOLOR_FORCE=1` if your CI log shows colours.

---

## 12. Exit codes and environment variables

| Exit code | Meaning |
| --- | --- |
| `0` | Done |
| `1` | `scan --fail-on` threshold reached |
| `2` | Error: bad flags, invalid `.barge.json`, not a git repository, unknown revision, no module matching `--only` |

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | Any value turns colours off |
| `CLICOLOR_FORCE` | `1` keeps colours even when output is not a terminal, e.g. in CI logs |
| `COLORTERM` | `truecolor` or `24bit` tells Barge the terminal shows 24-bit colour (most set it themselves); otherwise the logo uses the nearest of 256 colours |
| `TERM` | `dumb` turns colours off |

Colours are also off when the output goes to a file or a pipe, and the logo
is shown only in a terminal.

---

## 13. Troubleshooting

**`barge: command not found`**
- After `go build -o barge`, the program is in the current folder: run
  `./barge`.
- After `go install`, add `~/go/bin` to `PATH` (see [Install](#1-install)).

**`"GLOB" in the help stands for your own value`**
You typed the placeholder from the help. Write `--module 'internal/features/*'`,
not `--module GLOB 'internal/features/*'`.

**`unexpected "…": scan takes one path`**
A flag takes one value. Repeat the flag (`--only a --only b`) or use commas
(`--only a,b`).

**`module rule "…" matches no directory in this repository`**
The `--module` pattern (or a `modules` entry in `.barge.json`) matches
nothing, so it changed nothing. Check the path with
`barge scan --all --top 0`. If you wanted to *show* only some modules, use
`--only`.

**`no module matches --only "…"`**
Module paths are directories, such as `pkg/compose`. List them all with
`barge scan --all --top 0`.

**`… is not inside a git repository` / `… is a URL; clone it first`**
Barge works on a local clone: `git clone <url>`, then `barge scan <folder>`.

**`cannot resolve "develop"`**
The branch exists only on the remote: use `--rev origin/develop`, or
`git fetch` first.

**`nobody committed as … on …`**
The e-mail does not appear in the history. Find the right one with
`barge scan --json` under `people[].emails`.

**Everyone shows as a newcomer, or there are very few commits**
The clone is shallow. Fetch the full history: `git fetch --unshallow`.

**`.barge.json: … unknown field`**
A key is misspelt or not supported. See [the file reference](#8-the-bargejson-file).

**The same person appears twice**
Add a `.mailmap` to the repository:
`Oleg Koval <oleg@example.com> <oleg@old-company.com>`.

**Building from source: `module … found, but does not contain package …`**
The first line of `go.mod` must be `module gitlab.com/codebarge/barge`.

---

## 14. FAQ

**Does Barge send anything anywhere?**
No. It runs `git` on your machine and prints the result. There is no
network access and no telemetry.

**How long does a scan take?**
Usually seconds: about one second for 3,000 commits. Most of the time is
spent in `git log`.

**Is a high bus factor score a judgement of people?**
No. Barge shows where knowledge sits so a team can spread it before it is
lost. It never ranks people; it only answers "who knows this module".

**Does it work with monorepos?**
Yes. Use `--module` to define the modules, `--only` to look at one part, and
`--depth` for a coarser view.

**Can it scan several repositories at once?**
Scan them one by one:
`for d in ~/code/*/; do barge scan "$d"; done`. Barge for teams shows all of a
team's repositories on one screen.

**What is Barge for teams?**
A hosted or self-hosted version that adds a shared dashboard across
repositories, history over time, alerts when a module becomes risky, short
AI-assisted handover interviews, a knowledge base people can ask questions
of, and access for AI coding agents over MCP. The analysis is this same open
source library. Interested in a pilot?
[Request one](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Pilot%20request): the request is confidential, only you and
the maintainers see it.

**Can I use the analysis from Go?**
Yes, it is a plain package with no dependencies:

```go
import (
    "gitlab.com/codebarge/barge/core/git"
    "gitlab.com/codebarge/barge/features/ownership/repository/gitsource"
    "gitlab.com/codebarge/barge/features/ownership/service"
)

repo, _ := git.Open(ctx, ".")
head, _ := repo.ResolveRev(ctx, "HEAD")
report, _ := service.Analyze(ctx, gitsource.New(repo, head), service.DefaultConfig())
report = service.Filter(report, []string{"pkg/*"}) // optional, like --only
```

**Where do I report a bug?**
Open a [bug report](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Bug%20report): the form asks for the command, its
output, `barge version --short` and your system. Replace your colleagues'
names before posting. Ideas go in a [feature request](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Feature%20request),
questions in a [question](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Question). See also [CONTRIBUTING.md](CONTRIBUTING.md).
