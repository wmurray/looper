# looper

`looper` is a Go CLI that runs iterative implement/review agent cycles against a plan file. An execution agent implements the plan, a reviewer agent checks the work, and the loop commits locally after each iteration. It exits early on reviewer approval ("Job's done!") or when safety guards detect thrashing or repeated failures — without ever pushing, rebasing, or rewriting git history.

## Disclaimer

This is an experimental proof of concept inspired by the [Ralph Loop](https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum) pattern from Claude Code's plugin ecosystem. Where Ralph runs a single self-referential agent loop via a Stop hook, `looper` is orchestrated externally as a Go CLI and adds a dedicated reviewer agent, explicit git checkpointing, safety guards, and progress tracking.

It was written for personal use under specific conditions — a Rails codebase, Claude Code as the backend, and a particular set of workflow skills. As a result, some configuration assumptions are baked in rather than fully generalised. Use it as a reference or starting point, not a polished general-purpose tool.

## Prerequisites

- **Go 1.21+** — for building from source
- **`claude` CLI** (Claude Code) or **Cursor's `agent` CLI** — the backend agent that executes plans

## Installation

```bash
git clone https://github.com/willmurray/looper
cd looper
make install       # builds binary and moves it to ~/.local/bin
```

Ensure `~/.local/bin` is on your `PATH`.

### Without Go (pre-built binary)

```bash
curl -L https://github.com/willmurray/looper/releases/latest/download/looper-darwin-arm64 \
  -o ~/.local/bin/looper && chmod +x ~/.local/bin/looper
```

Replace `darwin-arm64` with `darwin-amd64` or `linux-amd64` as needed.

## Commands

### `implement` — run the agent loop

```bash
looper implement                        # infer ticket from branch, find *_PLAN.md
looper implement --plan DX-123_PLAN.md  # explicit plan file
looper implement --cycles 5             # override cycle count
looper implement --timeout 300          # override per-iteration timeout (seconds)
looper implement --dry-run              # print resolved config, don't run agents
looper implement --yes                  # skip git staging confirmation prompt
```

### `plan` — create a plan file

```bash
looper plan              # infer ticket from branch, create TICKET_PLAN.md
looper plan DX-123       # create DX-123_PLAN.md
looper plan --open       # open in $EDITOR after creation
```

### `settings` — view or edit configuration

```bash
looper settings                             # print all settings as JSON
looper settings get backend                 # get a single value
looper settings set defaults.cycles 5       # set a value
looper settings set backend claude          # switch backend
looper settings reset                       # reset to defaults
```

## Configuration

Config is stored at `~/.config/looper/config.json`.

| Key | Default | Description |
|---|---|---|
| `backend` | `claude` | Agent backend: `claude` or `cursor` |
| `defaults.cycles` | `5` | Maximum implement/review iterations |
| `defaults.timeout` | `420` | Per-iteration timeout in seconds |
| `skill_path` | `~/.claude/skills/tdd-workflow/SKILL.md` | Workflow skill injected into the execution prompt |
| `reviewer_agent` | `~/.claude/agents/rails-code-reviewer.md` | Reviewer agent definition injected into the review prompt |
| `ticket_pattern` | `[A-Z]+-[0-9]+` | Regex for inferring ticket ID from branch name (e.g. `DX-123`) |

## Skills setup

`looper` requires two markdown files to function — a workflow skill for the execution agent and a reviewer agent definition. These are plain markdown files you can write yourself or source from a workflow library.

The recommended source is [dgalarza/claude-code-workflows](https://github.com/dgalarza/claude-code-workflows), which provides a TDD workflow skill and a Rails code reviewer agent:

```bash
# TDD workflow skill (requires Node.js)
npx skills add dgalarza/claude-code-workflows --skill "tdd-workflow"
looper settings set skill_path ~/.claude/skills/tdd-workflow/SKILL.md

# Rails code reviewer agent
curl -fsSL https://raw.githubusercontent.com/dgalarza/claude-code-workflows/main/plugins/rails-toolkit/agents/rails-code-reviewer.md \
  -o ~/.claude/agents/rails-code-reviewer.md
looper settings set reviewer_agent ~/.claude/agents/rails-code-reviewer.md
```

If either file is missing when you run `looper implement`, you will be warned and prompted to confirm before the loop runs.

## Safety guarantees

`looper` is designed to be safe to run on any git repository:

- **Never pushes** code to a remote
- **Never changes branches**
- **Never rebases, force-pushes, or rewrites history**
- **Never cherry-picks or resets HEAD**
- All commits are local and preserved for audit
- Only git operations used: `add`, `commit`, `diff`, `status`, `log`, `rev-parse`
- `git add` respects `.gitignore` — ignored files (e.g. `.env`) are never staged
- Progress and plan files are written only in the working directory

## Running tests

```bash
go test ./...
```
