#!/bin/bash
# loop.sh - Automated plan execution with review cycles
#
# Usage (named):
#   ./loop.sh --plan <file> --cycles <n> [--timeout <seconds>]
#
# Usage (positional):
#   ./loop.sh <plan-file> <max-cycles> [timeout-seconds]
#
# SAFETY GUARANTEES:
# - Never pushes code
# - Never changes branches
# - Never rebases or rewrites history
# - All commits are preserved for audit trail
# - Only uses: git add, commit, diff, status, checkout

set -e

# === PARSE ARGUMENTS ===
PLAN_FILE=""
MAX_CYCLES=""
ITER_TIMEOUT=420  # 7 minutes default

if [[ $1 == --* ]]; then
    # Named arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --plan)
                PLAN_FILE="$2"
                shift 2
                ;;
            --cycles)
                MAX_CYCLES="$2"
                shift 2
                ;;
            --timeout)
                ITER_TIMEOUT="$2"
                shift 2
                ;;
            *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
    done
else
    # Positional: only cycles (plan will be inferred from ticket)
    MAX_CYCLES="$1"
    ITER_TIMEOUT="${2:-420}"
fi

# === VALIDATION ===
if [[ -z "$MAX_CYCLES" ]]; then
    echo "Usage (named):"
    echo "  $0 --cycles <n> [--plan <file>] [--timeout <seconds>]"
    echo ""
    echo "Usage (positional):"
    echo "  $0 <max-cycles> [timeout-seconds]"
    echo ""
    echo "Plan file: Auto-inferred from branch ticket (DX-123_PLAN.md)"
    echo "           or pass --plan <file> to override"
    echo ""
    echo "Defaults: timeout = 420s (7 minutes)"
    exit 1
fi

if ! [[ "$MAX_CYCLES" =~ ^[0-9]+$ ]] || [[ "$MAX_CYCLES" -lt 1 ]]; then
    echo "Error: cycles must be positive integer (got: $MAX_CYCLES)"
    exit 1
fi

if ! [[ "$ITER_TIMEOUT" =~ ^[0-9]+$ ]] || [[ "$ITER_TIMEOUT" -lt 10 ]]; then
    echo "Error: timeout must be >= 10 seconds (got: $ITER_TIMEOUT)"
    exit 1
fi

# === GIT VALIDATION ===
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: Not in a git repository"
    exit 1
fi

if [[ ! -z "$(git status --porcelain)" ]]; then
    echo "Error: Repository has uncommitted changes"
    echo "Please commit or stash changes before running this script"
    exit 1
fi

# === SETUP ===
# Try to get ticket from branch name first (format: wm/DX-123-description)
TICKET_FROM_BRANCH=$(git rev-parse --abbrev-ref HEAD | grep -oE '[A-Z]+-[0-9]+' | head -1 || echo "")

# Fall back to plan file if branch doesn't have ticket (only if plan file provided)
TICKET_FROM_PLAN=""
if [[ -f "$PLAN_FILE" ]]; then
    TICKET_FROM_PLAN=$(grep -i "Ticket:" "$PLAN_FILE" | awk '{print $2}' 2>/dev/null || echo "")
fi

# Use branch ticket first, then plan, then unknown
TICKET_NUMBER="${TICKET_FROM_BRANCH:-${TICKET_FROM_PLAN:-UNKNOWN}}"

# Infer plan file from ticket if not provided
if [[ -z "$PLAN_FILE" ]]; then
    if [[ -f "${TICKET_NUMBER}_PLAN.md" ]]; then
        PLAN_FILE="${TICKET_NUMBER}_PLAN.md"
    elif [[ -f "${TICKET_NUMBER}_plan.md" ]]; then
        PLAN_FILE="${TICKET_NUMBER}_plan.md"
    fi
fi

# Verify plan file exists
if [[ ! -f "$PLAN_FILE" ]]; then
    echo "Error: plan file not found: $PLAN_FILE"
    echo "Tried to auto-infer from ticket: $TICKET_NUMBER"
    echo "Pass --plan <file> to specify explicitly"
    exit 1
fi

PROGRESS_FILE="${TICKET_NUMBER}_PROGRESS.md"

echo "Starting loop: $TICKET_NUMBER"
echo "Max cycles: $MAX_CYCLES | Timeout per iteration: ${ITER_TIMEOUT}s"
echo ""

cat > "$PROGRESS_FILE" << EOF
# Progress for Ticket $TICKET_NUMBER

**Plan File:** $PLAN_FILE
**Max Cycles:** $MAX_CYCLES
**Timeout per Iteration:** ${ITER_TIMEOUT}s
**Started:** $(date)

---

EOF

# === GUARDS ===
THRASH_COUNT=0
STUCK_COUNT=0
PREV_ISSUES=""
TOTAL_ITERATIONS=0

# === SUMMARY REPORT FUNCTION ===
append_summary_report() {
    local status="$1"
    local final_iteration="$2"
    
    cat >> "$PROGRESS_FILE" << 'SUMMARY'

---

## Summary Report

SUMMARY
    
    echo "**Status:** $status" >> "$PROGRESS_FILE"
    echo "**Iterations:** $final_iteration of $MAX_CYCLES" >> "$PROGRESS_FILE"
    echo "**Timeout per Iteration:** ${ITER_TIMEOUT}s" >> "$PROGRESS_FILE"
    echo "**Ticket:** $TICKET_NUMBER" >> "$PROGRESS_FILE"
    echo "**Completed:** $(date)" >> "$PROGRESS_FILE"
    echo "" >> "$PROGRESS_FILE"
    
    # Guards triggered
    if [[ $THRASH_COUNT -gt 0 ]] || [[ $STUCK_COUNT -gt 0 ]]; then
        echo "### Guards Triggered" >> "$PROGRESS_FILE"
        [[ $THRASH_COUNT -gt 0 ]] && echo "- No changes detected: $THRASH_COUNT time(s)" >> "$PROGRESS_FILE"
        [[ $STUCK_COUNT -gt 0 ]] && echo "- Repeated issues: $STUCK_COUNT time(s)" >> "$PROGRESS_FILE"
        echo "" >> "$PROGRESS_FILE"
    fi
    
    # Commits made
    echo "### Commits Made" >> "$PROGRESS_FILE"
    git log --oneline -n "$final_iteration" >> "$PROGRESS_FILE" 2>/dev/null || echo "(No commits)" >> "$PROGRESS_FILE"
    echo "" >> "$PROGRESS_FILE"
    
    # Next steps
    echo "### Next Steps" >> "$PROGRESS_FILE"
    if [[ "$status" == *"Complete"* ]]; then
        echo "1. Review changes: \`git log --patch\`" >> "$PROGRESS_FILE"
        echo "2. Manually test if needed" >> "$PROGRESS_FILE"
        echo "3. Optionally squash commits: \`git rebase -i\`" >> "$PROGRESS_FILE"
        echo "4. Create PR and request human review" >> "$PROGRESS_FILE"
    else
        echo "1. Review progress file: \`cat $PROGRESS_FILE\`" >> "$PROGRESS_FILE"
        echo "2. Investigate guard triggers above" >> "$PROGRESS_FILE"
        echo "3. Fix issues and rerun script, or continue manually" >> "$PROGRESS_FILE"
    fi
}

# === MAIN LOOP ===
for (( i=1; i<=$MAX_CYCLES; i++ ))
do
    ITER_START=$(date +%s)
    TOTAL_ITERATIONS=$i
    
    echo "## RUN $i" >> "$PROGRESS_FILE"
    echo ""
    echo "=== Iteration $i of $MAX_CYCLES ===" 
    
    # --- PHASE 1: EXECUTION ---
    echo "### Execution" >> "$PROGRESS_FILE"
    echo "[$(date '+%H:%M:%S')] Execution phase..."
    
    if [[ $i -gt 1 ]]; then
        PREV_CONTEXT=$(sed -n "/## RUN $((i-1))/,/## RUN $i/p" "$PROGRESS_FILE" 2>/dev/null || echo "(Previous context unavailable)")
    else
        PREV_CONTEXT="(First run)"
    fi
    
    EXEC_OUTPUT=$(timeout $ITER_TIMEOUT agent -p "Follow ~/.cursor/skills/tdd-workflow/SKILL.md

Execute this plan:
\`\`\`
$(cat "$PLAN_FILE")
\`\`\`

Context from previous iteration:
$PREV_CONTEXT

Complete the plan using Test-Driven Development:
1. Write failing tests
2. Write implementation to pass tests
3. Refactor as needed
4. All tests must pass before you complete

Provide a concise summary of what you accomplished." --output-format text 2>&1)
    EXEC_CODE=$?
    
    # Handle execution timeout
    if [[ $EXEC_CODE -eq 124 ]]; then
        echo "⚠ **Guard Triggered**: Execution timeout after ${ITER_TIMEOUT}s" >> "$PROGRESS_FILE"
        echo "⚠ Execution agent timeout"
        
        # Check if changes were made
        if [[ ! -z "$(git diff)" ]]; then
            git add -A
            git commit -m "WIP: Iteration $i - timeout during execution" 2>&1 | tee -a "$PROGRESS_FILE"
            echo "Partial changes committed"
        fi
        exit 1
    elif [[ $EXEC_CODE -ne 0 ]]; then
        echo "⚠ Execution failed (code $EXEC_CODE)" >> "$PROGRESS_FILE"
        echo "$EXEC_OUTPUT" >> "$PROGRESS_FILE"
        exit 1
    fi
    
    echo "$EXEC_OUTPUT" >> "$PROGRESS_FILE"
    
    # Capture git state
    GIT_STATUS=$(git status --short)
    GIT_DIFF=$(git diff)
    
    echo "" >> "$PROGRESS_FILE"
    echo "**Git Status:**" >> "$PROGRESS_FILE"
    echo "\`\`\`" >> "$PROGRESS_FILE"
    echo "$GIT_STATUS" >> "$PROGRESS_FILE"
    echo "\`\`\`" >> "$PROGRESS_FILE"
    echo "" >> "$PROGRESS_FILE"
    echo "**Git Diff:**" >> "$PROGRESS_FILE"
    echo "\`\`\`diff" >> "$PROGRESS_FILE"
    echo "$GIT_DIFF" >> "$PROGRESS_FILE"
    echo "\`\`\`" >> "$PROGRESS_FILE"
    
    # === GUARD 1: Check for no changes ===
    if [[ -z "$(echo "$GIT_DIFF" | tr -d '[:space:]')" ]]; then
        THRASH_COUNT=$((THRASH_COUNT + 1))
        echo "⚠ **Guard Alert**: No changes detected (Count: $THRASH_COUNT/2)" >> "$PROGRESS_FILE"
        echo "⚠ No changes detected"
        if [[ $THRASH_COUNT -ge 2 ]]; then
            echo "" >> "$PROGRESS_FILE"
            echo "⚠ **Guard Triggered**: No changes in 2 consecutive iterations. Exiting to prevent token waste." >> "$PROGRESS_FILE"
            echo "Aborting: agent produced no changes for 2 cycles (likely stuck)"
            exit 1
        fi
    else
        THRASH_COUNT=0
    fi
    
    # --- PHASE 2: REVIEW ---
    echo "" >> "$PROGRESS_FILE"
    echo "### Review" >> "$PROGRESS_FILE"
    echo "[$(date '+%H:%M:%S')] Review phase..."
    
    CURRENT_EXEC=$(sed -n "/## RUN $i/,/### Review/p" "$PROGRESS_FILE" 2>/dev/null | sed '$ d' || echo "")
    
    REVIEW_OUTPUT=$(timeout $ITER_TIMEOUT agent -p "Using the rails-code-reviewer subagent (~/.cursor/agents/rails-code-reviewer.md)

Review these changes against the plan:

Plan:
\`\`\`
$(cat "$PLAN_FILE")
\`\`\`

Changes made:
$CURRENT_EXEC

Provide your assessment. If the implementation looks good and meets the plan, start your response with: 👷‍♂️ Job's done!

If there are issues to address, start with: 🔧 Needs work and list the issues." --output-format text 2>&1)
    REVIEW_CODE=$?
    
    # Handle review timeout
    if [[ $REVIEW_CODE -eq 124 ]]; then
        echo "⚠ **Guard Triggered**: Review timeout after ${ITER_TIMEOUT}s" >> "$PROGRESS_FILE"
        echo "⚠ Review agent timeout"
        
        # Check if new changes were made
        if [[ ! -z "$(git diff)" ]]; then
            git add -A
            git commit -m "WIP: Iteration $i - timeout during review" 2>&1 | tee -a "$PROGRESS_FILE"
            echo "Changes committed before timeout"
        fi
        exit 1
    elif [[ $REVIEW_CODE -ne 0 ]]; then
        echo "⚠ Review failed (code $REVIEW_CODE)" >> "$PROGRESS_FILE"
        echo "$REVIEW_OUTPUT" >> "$PROGRESS_FILE"
        exit 1
    fi
    
    echo "$REVIEW_OUTPUT" >> "$PROGRESS_FILE"
    echo "" >> "$PROGRESS_FILE"
    
    # === GUARD 2: Check for repeated issues ===
    CURRENT_ISSUES=$(echo "$REVIEW_OUTPUT" | grep -oiE "(TODO|FIXME|bug|issue|error|fail|undefined|nil)" | sort | uniq | tr '\n' ',' || echo "")
    
    if [[ ! -z "$CURRENT_ISSUES" ]] && [[ "$PREV_ISSUES" == "$CURRENT_ISSUES" ]]; then
        STUCK_COUNT=$((STUCK_COUNT + 1))
        echo "⚠ **Guard Alert**: Same issues detected (Count: $STUCK_COUNT/2)" >> "$PROGRESS_FILE"
        echo "⚠ Same issues appearing (possible thrashing)"
        if [[ $STUCK_COUNT -ge 2 ]]; then
            echo "" >> "$PROGRESS_FILE"
            echo "⚠ **Guard Triggered**: Same issues in 2 consecutive iterations: $CURRENT_ISSUES" >> "$PROGRESS_FILE"
            echo "Agent may be thrashing. Exiting." >> "$PROGRESS_FILE"
            echo "Aborting: agent repeating same issues (likely stuck)"
            exit 1
        fi
    else
        STUCK_COUNT=0
    fi
    PREV_ISSUES="$CURRENT_ISSUES"
    
    # === GUARD 3: Check iteration duration ===
    ITER_END=$(date +%s)
    ITER_DURATION=$((ITER_END - ITER_START))
    
    if [[ $ITER_DURATION -gt $ITER_TIMEOUT ]]; then
        echo "⚠ **Guard Alert**: Iteration took ${ITER_DURATION}s (timeout: ${ITER_TIMEOUT}s)" >> "$PROGRESS_FILE"
    fi
    
    # === COMMIT ITERATION ===
    if [[ ! -z "$(git diff)" ]]; then
        git add -A
        COMMIT_MSG="Update: Iteration $i - Execution and review cycle"
        git commit -m "$COMMIT_MSG" --quiet
        echo "[$(date '+%H:%M:%S')] Committed iteration $i"
    fi
    
    # === CHECK FOR SUCCESS ===
    if echo "$REVIEW_OUTPUT" | grep -iq "job.*s done"; then
        SUCCESS_MSG="👷‍♂️ Job's done - ahead of schedule!
Completed in $i of $MAX_CYCLES iterations"
        echo "" >> "$PROGRESS_FILE"
        echo "$SUCCESS_MSG" >> "$PROGRESS_FILE"
        echo ""
        echo "$SUCCESS_MSG"
        
        # Final summary
        append_summary_report "✓ Complete - ahead of schedule" "$i"
        exit 0
    fi
    
    echo ""
done

# === MAX CYCLES REACHED ===
echo ""
echo "⚠ Max cycles ($MAX_CYCLES) reached without approval"
append_summary_report "⚠ Max cycles reached" "$TOTAL_ITERATIONS"
exit 1

