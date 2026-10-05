# Changelog

All notable changes to the Barge CLI. Versions follow
[semantic versioning](https://semver.org/): the second number grows with new
features, the third with fixes.

## v0.3.0 — Unreleased

The first public release.

### Commands

- `barge scan`: who knows each module, its bus factor, risk and share of
  AI-written code, the repository bus factor, and concrete next steps.
- `barge handover <email>`: a Markdown checklist for one person: the modules
  only they know, a successor for each, the files they wrote, and questions
  about their largest commits, reverts, fragile files and operations.
- `barge version` (`--short`, `--json`, `barge --version`) and `barge help`
  (`barge help <command>`, `-h` on any command) with the Barge logo.

### Analysis

- Knowledge weighted by change size (`log(1 + lines)`) and fading with a
  half-life of 180 days; mass changes count a tenth.
- Bus factor at a 40% knowledge threshold; risk levels `critical`, `high`,
  `medium`, `low` with reasons.
- People merged across addresses (`.mailmap`, GitHub noreply, names); bots
  ignored; AI agents own nothing and AI-assisted commits count half.
- Dependencies, lock files, generated code, build output and test data
  ignored by default.

### Options

- `--only GLOB` shows only the modules under matching directories.
- `--module`, `--depth`, `--exclude` shape the modules; a `--module` rule that
  matches nothing is reported.
- `--leaving` and `--left` simulate departures; `--rev` and `--as-of` analyse
  any revision and date.
- `--json` for scripts, `--fail-on` for CI.
- `.barge.json` sets all of this once per repository, including `owners`
  corrections (`"knows": true` / `false`).

### Documentation

- [DOCUMENTATION.md](DOCUMENTATION.md): every command and flag, how to read
  the output, `.barge.json`, the analysis model, JSON, CI and troubleshooting.
