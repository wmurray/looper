# looper

`looper` is a Go CLI that runs iterative implement/review agent cycles against a plan file. An execution agent implements the plan, a reviewer agent checks the work, and the loop commits locally after each iteration. It exits early on reviewer approval ("Job's done!") or when safety guards detect thrashing or repeated failures — without ever pushing, rebasing, or rewriting git history.

## Disclaimer

This is an experimental proof of concept inspired by the [Ralph Loop](https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum) pattern from Claude Code's plugin ecosystem. Where Ralph runs a single self-referential agent loop via a Stop hook, `looper` is orchestrated externally as a Go CLI and adds a dedicated reviewer agent, explicit git checkpointing, safety guards, and progress tracking.

It was 100% vibe-coded, written for personal use under specific conditions — a Rails codebase, Claude Code as the backend, and a particular set of workflow skills. As a result, some configuration assumptions are baked in rather than fully generalised. Consider it a reference or starting point, not a polished general-purpose tool.

#### **Use at your own risk.**

## Prerequisites

- **Go 1.21+** — for building from source
- **`claude` CLI** (Claude Code) or **Cursor's `agent` CLI** — the backend agent that executes plans
- **Linear MCP** - Optional-ish. `looper start` will use the Linear MCP to query for the passed ticket number. You can use `looper plan` and `looper implement` to manually create/execute a plan.

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

### `init` — Initialize repository for looper

Set up a new repository for looper by creating the standard directory structure, configuring `.gitignore`, and optionally generating repo-specific settings.

```bash
looper init              # full setup: .looper/ directory, .gitignore, and .looper.json config
looper init --yes       # auto-accept all defaults without prompts
looper init --dry-run   # preview changes without applying
looper init --skip-gitignore  # skip .gitignore setup
looper init --config-only  # only create .looper.json, skip directory setup
looper init --migrate   # move existing root-level plan/progress/state files to .looper/:ticket/ structure
```

**What it does:**

- Creates a `.looper/` directory at the repository root for organized file storage
- Updates `.gitignore` to exclude `.looper/` from version control
- Detects your project stack (Go, Node.js, Python, Ruby/Rails, Rust, Java/Maven)
- Prompts to create a `.looper.json` configuration file with sensible defaults
- Scans for global config and available agents, offering setup guidance if needed
- Detects any existing root-level looper files and offers to migrate them

**Directory structure after `looper init`:**

```
.looper/
└── TICKET/
    ├── TICKET_PLAN.md      # Plan created by `looper plan`
    ├── TICKET_PROGRESS.md  # Progress tracked during runs
    └── TICKET_STATE.json   # Execution state (internal)
```

**Note on file locations:** Plan files are written to `.looper/:ticket/` by default. Progress and state files will move there too in an upcoming update — existing root-level files continue to work in the meantime. Use `looper init --migrate` to relocate existing files manually.

**Configuration (`looper.json`):**

Created by `looper init --config-only` or during full init, this file stores repo-specific overrides:

```json
{
  "defaults": {
    "cycles": 5,
    "timeout": 420
  },
  "reviewers": {
    "general": ""
  },
  "stack": "Node.js/JavaScript + Go"
}
```

These settings are merged with your global config, allowing per-project customization without affecting other repositories.

### `start` — Linear-integrated workflow (recommended)

Fetch a Linear ticket, create a branch, generate a plan, and run the implement loop in one command.

```bash
looper start IMP-123              # full workflow: branch → plan → implement
looper start IMP-123 --dry-run    # fetch and plan only, don't run agents
looper start IMP-123 --stream     # stream agent output to terminal
looper start IMP-123 --cycles 3   # override cycle count
looper start IMP-123 --retries 2  # retry each phase up to 2 times on transient errors
looper start IMP-123 --review-every 2  # run reviewer every N cycles
looper start IMP-123 --notify     # send desktop notification on completion
```

Requires `LINEAR_API_KEY` in the environment, `.env`, or `.env.local`:

```bash
# Copy the example file and add your key
cp .env.example .env.local
# Then edit .env.local and add your Linear API key

# Or set via environment variable
export LINEAR_API_KEY=lin_api_...
```

The `.env` file serves as a committed configuration template. Use `.env.local` for your local overrides and secrets — it is gitignored. Environment variables take precedence over both files.

Linear integration is optional — it is only used by `looper start`. All other commands work without it.

### `implement` — run the agent loop manually

Run the implement/review loop against an existing plan file.

```bash
looper implement                         # infer ticket from branch, find *_PLAN.md
looper implement --plan IMP-123_PLAN.md  # explicit plan file
looper implement --cycles 5              # override cycle count
looper implement --timeout 300           # override per-iteration timeout (seconds)
looper implement --retries 2             # retry each phase on transient errors
looper implement --review-every 2        # run reviewer every N cycles (1 = every cycle)
looper implement --stream                # stream agent output to terminal
looper implement --notify                # send desktop notification on completion
looper implement --dry-run               # print resolved config, don't run agents
looper implement --yes                   # skip git staging confirmation prompt
```

### `resume` — continue an interrupted loop

Continue an implement/review loop from the last completed cycle (after a network failure, timeout, or manual stop).

```bash
looper resume IMP-123  # resume from the last checkpoint
```

### `plan` — create a plan file

```bash
looper plan              # infer ticket from branch, create TICKET_PLAN.md
looper plan IMP-123      # create IMP-123_PLAN.md
looper plan --open       # open in $EDITOR after creation
looper plan --prompt "add user authentication"  # generate plan content via AI
```

### `polish` — post-implementation cleanup

Run a polish pass on the current branch before opening a PR: lint commands first, then an agent tidy pass.

```bash
looper polish             # lint + agent pass
looper polish --dry-run   # print resolved config without running
looper polish --yes       # skip confirmation prompt
```

Configure lint commands and the polish agent in settings (see below).

### `report` — show run history

Display a summary of recent looper runs with status, cycles, and outcomes.

```bash
looper report              # show last 20 runs
looper report --last 50    # show last 50 runs
looper report --ticket IMP-123  # filter by ticket ID
```

### `clean` — remove looper working files

```bash
looper clean        # remove *_PLAN.md, *_PROGRESS.md, *_STATE.json with confirmation
looper clean --yes  # skip confirmation prompt
```

### `settings` — view or edit configuration

```bash
looper settings                             # print all settings as JSON
looper settings get backend                 # get a single value
looper settings set defaults.cycles 5       # set a value
looper settings reset                       # reset to defaults
looper settings discover                    # scan ~/.claude/ for installed skills/agents
looper settings discover --apply            # auto-set keys with exactly one candidate
looper settings discover --ai               # use AI to recommend skill_path and reviewer_agent
looper settings discover --ai --yes         # apply AI recommendations without prompting
```

## Configuration

Global config is stored at `~/Library/Application Support/looper/config.json` (macOS). A per-repo `.looper.json` in the project root is merged on top, allowing per-project overrides that can be committed to the repository.

| Key | Default | Description |
|---|---|---|
| `backend` | `claude` | Agent backend: `claude` or `cursor` |
| `defaults.cycles` | `5` | Maximum implement/review iterations |
| `defaults.timeout` | `420` | Per-iteration timeout in seconds |
| `skill_path` | `~/.claude/skills/tdd-workflow/SKILL.md` | Workflow skill injected into the execution prompt |
| `reviewer_agent` | `~/.claude/agents/rails-code-reviewer.md` | Reviewer agent injected into the review prompt |
| `ticket_pattern` | `[A-Z]+-[0-9]+` | Regex for inferring ticket ID from branch name |
| `retries` | `0` | Max retries per phase on transient errors (rate limits, network) |
| `notify` | `false` | Send desktop notification on loop completion or abort |
| `notify_webhook` | — | Slack webhook URL to POST notifications to |
| `polish_agent` | — | Path to polish agent file (falls back to `reviewer_agent`) |
| `polish_cmds` | — | Comma-separated lint/format commands run before the polish agent |

### Per-repo config (`.looper.json`)

Place a `.looper.json` in your project root to override global settings for that repo:

```json
{
  "defaults": { "cycles": 3, "timeout": 300 },
  "retries": 2,
  "reviewer_agent": "~/.claude/agents/go-code-reviewer.md"
}
```

## Skills setup

`looper` requires two markdown files: a workflow skill for the execution agent and a reviewer agent definition. Use `looper settings discover` to find installed files automatically.

```bash
# Auto-discover and configure (recommended)
looper settings discover --ai
```

If either file is missing when you run `looper implement`, you will be warned and prompted to confirm before the loop runs.

The tool was originally built using patterns from the [dgalarza/claude-code-workflows](https://github.com/dgalarza/claude-code-workflows) collection as a baseline.

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
