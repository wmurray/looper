# Plan: agent-loop CLI (Go)

## Context

`loop.sh` is a bash script that runs an automated implement/review agent loop against a plan file. It works but is hard to extend — adding subcommands, config persistence, and future features (Claude support, settings) would push bash past its practical limits. This converts it to a proper Go CLI using cobra, with `implement` as the first subcommand and a foundation for `plan` and `settings`.

The tool targets Cursor's `agent` CLI (not Claude Code). A future `backend` config setting will allow swapping in `claude`.

---

## File Structure

```
/Users/willmurray/Projects/agent_loop/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go          # cobra root, global --backend flag
│   ├── implement.go     # `agent-loop implement` subcommand
│   ├── plan.go          # `agent-loop plan` subcommand
│   └── settings.go      # `agent-loop settings` subcommand
└── internal/
    ├── config/
    │   └── config.go    # load/save ~/.config/agent-loop/config.json
    ├── git/
    │   └── git.go       # ticket inference, git validation, commit
    ├── runner/
    │   └── runner.go    # subprocess wrapper for agent/claude
    ├── guards/
    │   └── guards.go    # thrash + stuck-issue guard state
    └── progress/
        └── progress.go  # progress file writer (markdown)
```

---

## Config File

**Location:** `~/.config/agent-loop/config.json`

```json
{
  "backend": "cursor",
  "defaults": {
    "cycles": 5,
    "timeout": 420
  },
  "skill_path": "~/.cursor/skills/tdd-workflow/SKILL.md",
  "reviewer_agent": "~/.cursor/agents/rails-code-reviewer.md"
}
```

`backend` values: `"cursor"` (calls `agent -p ...`) or `"claude"` (calls `claude -p ...`). Both use `--output-format text`.

---

## Commands

### `agent-loop implement`

```
agent-loop implement [--cycles N] [--plan FILE] [--timeout SECONDS] [--dry-run]
```

Ports `loop.sh` logic exactly:
1. Assert git repo + clean working tree
2. Infer ticket from branch (`[A-Z]+-[0-9]+` regex) or plan file `Ticket:` header
3. Resolve plan file: `--plan` flag → `{TICKET}_PLAN.md` → error
4. Load config defaults (overridden by flags)
5. Init `{TICKET}_PROGRESS.md`
6. Loop up to N cycles:
   - Phase 1 (execute): run agent with plan + previous context
   - Guard 1: no-changes detection (2 strikes → abort)
   - Phase 2 (review): run agent with plan + execution output
   - Guard 2: repeated-issues detection (2 strikes → abort)
   - Guard 3: log iteration duration
   - Commit if changes exist
   - Exit 0 if review contains "job.*s done" (case-insensitive)
7. Exit 1 on max cycles reached

`--dry-run`: resolves all params, prints config table, exits without calling agent.

### `agent-loop plan [TICKET]`

Creates `{TICKET}_PLAN.md` from a template if it doesn't exist. Infers ticket from branch if not provided. Prints the file path. `--open` flag opens in `$EDITOR`.

Template includes: `Ticket:`, Objective, Context, Implementation Steps, Acceptance Criteria, Out of Scope.

### `agent-loop settings`

```
agent-loop settings                      # print all settings as JSON
agent-loop settings get <key>            # dot-notation: defaults.cycles
agent-loop settings set <key> <value>
agent-loop settings reset
```

---

## Key Implementation Details

### `internal/runner/runner.go`

```go
func Run(prompt string, timeout int, backend string) (string, int, error)
```

- `cursor` backend: `exec.Command("agent", "-p", prompt, "--output-format", "text")`
- `claude` backend: `exec.Command("claude", "-p", prompt, "--output-format", "text")`
- Uses `exec.CommandContext` with a `time.Duration` deadline
- Returns `(stdout, exitCode, error)` — exit code 124 on timeout (mirrors bash `timeout` behavior)

### `internal/guards/guards.go`

```go
type State struct {
    ThrashCount int
    StuckCount  int
    PrevIssues  string
}

func (s *State) CheckNoChanges(gitDiff string) GuardResult
func (s *State) CheckRepeatedIssues(reviewOutput string) GuardResult
```

Pure logic, no I/O. `GuardResult` carries `{Triggered bool, Warning bool, Message string}`.

### `internal/git/git.go`

- `AssertRepo()`, `AssertClean()` — exit with message on failure
- `InferTicketFromBranch()` — `git rev-parse --abbrev-ref HEAD` + regex
- `InferTicketFromPlan(path)` — scan first 10 lines for `Ticket: ...`
- `Diff()`, `StatusShort()` — for guard and progress output
- `CommitIteration(n int)` — `git add -A && git commit -m "..."` (no-op if diff empty)

### Prompts

Exact prompts from `loop.sh` are preserved as Go string templates in `implement.go`. The skill path and reviewer agent path are injected from config.

---

## Installation

```bash
go build -o agent-loop .
mv agent-loop ~/.local/bin/
```

Or via `Makefile`:
```makefile
install:
    go build -o agent-loop . && mv agent-loop ~/.local/bin/
```

---

## Verification

1. `go build ./...` — no errors
2. `agent-loop --help` — shows subcommands
3. `agent-loop settings set defaults.cycles 3` → `agent-loop settings get defaults.cycles` returns `3`
4. `agent-loop plan` (in a git branch `wm/DX-123-desc`) → creates `DX-123_PLAN.md`
5. `agent-loop implement --dry-run --plan DX-123_PLAN.md --cycles 2` — prints resolved config, exits 0
6. Full loop test: `agent-loop implement --plan DX-123_PLAN.md --cycles 1` in a clean git repo with Cursor's `agent` CLI available
