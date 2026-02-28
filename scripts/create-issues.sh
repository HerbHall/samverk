#!/usr/bin/env bash
# Create GitHub issues from a JSON batch file.
# Usage: bash scripts/create-issues.sh [--dry-run] path/to/batch.json
set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────

DRY_RUN=false
BATCH_FILE=""

for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
        *)         BATCH_FILE="$arg" ;;
    esac
done

if [[ -z "$BATCH_FILE" ]]; then
    echo "Usage: bash scripts/create-issues.sh [--dry-run] <batch.json>"
    exit 1
fi

if [[ ! -f "$BATCH_FILE" ]]; then
    echo "Error: file not found: $BATCH_FILE"
    exit 1
fi

# ── Python detection (Windows Store aliases hang -- test with --version) ──

PYTHON=""
for p in python3 python "/c/Program Files/Python312/python" "/c/Program Files/Python39/python"; do
    if "$p" --version &>/dev/null 2>&1; then PYTHON="$p"; break; fi
done

if [[ -z "$PYTHON" ]]; then
    echo "Error: python not found. Install Python 3 and ensure it is on PATH."
    exit 1
fi

# ── Detect repo owner/name from git remote ───────────────────────

REPO=$(git remote get-url origin 2>/dev/null \
    | sed -E 's#.*github\.com[:/]##; s#\.git$##')

if [[ -z "$REPO" ]]; then
    echo "Error: could not detect GitHub repo from git remote 'origin'."
    exit 1
fi

echo "==> create-issues: repo=$REPO dry_run=$DRY_RUN"

# ── Resolve milestone titles to numbers ──────────────────────────

echo "--- resolving milestones"
MILESTONE_MAP=$(gh api "repos/$REPO/milestones" --paginate \
    --jq '.[] | "\(.title)\t\(.number)"' 2>/dev/null || true)

resolve_milestone() {
    local title="$1"
    if [[ -z "$title" || "$title" == "null" ]]; then
        echo ""
        return
    fi
    local num
    num=$(echo "$MILESTONE_MAP" | while IFS=$'\t' read -r mt mn; do
        if [[ "$mt" == "$title" ]]; then echo "$mn"; break; fi
    done)
    if [[ -z "$num" ]]; then
        echo "Warning: milestone '$title' not found in repo -- skipping milestone assignment" >&2
        echo ""
    else
        echo "$num"
    fi
}

# ── Validate labels exist in repo ────────────────────────────────

echo "--- validating labels"
REPO_LABELS=$(gh label list --repo "$REPO" --json name --jq '.[].name' --limit 200)

validate_labels() {
    local missing=()
    for label in "$@"; do
        if ! echo "$REPO_LABELS" | grep -qxF "$label"; then
            missing+=("$label")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Warning: labels not in repo: ${missing[*]}" >&2
        return 1
    fi
    return 0
}

# ── Count issues in batch ────────────────────────────────────────

ISSUE_COUNT=$("$PYTHON" -c "
import json, sys
batch = json.load(open(sys.argv[1], encoding='utf-8'))
print(len(batch.get('issues', [])))
" "$BATCH_FILE")

echo "--- found $ISSUE_COUNT issues in batch"

# ── Process each issue ───────────────────────────────────────────

CREATED=0
FAILED=0

for i in $(seq 0 $(( ISSUE_COUNT - 1 ))); do
    # Extract issue fields via Python
    ISSUE_JSON=$("$PYTHON" -c "
import json, sys

batch = json.load(open(sys.argv[1], encoding='utf-8'))
issue = batch['issues'][int(sys.argv[2])]

# Build YAML frontmatter
fm = issue.get('frontmatter', {})
lines = ['---']
for key in ['schema_version', 'type', 'agent_type', 'priority',
            'parent_issue', 'depends_on', 'estimated_tokens']:
    val = fm.get(key)
    if val is None:
        continue
    if isinstance(val, str):
        lines.append(f'{key}: \"{val}\"')
    elif isinstance(val, list):
        items = ', '.join(str(v) for v in val)
        lines.append(f'{key}: [{items}]')
    else:
        lines.append(f'{key}: {val}')
lines.append('---')

# Build body sections
sections = issue.get('sections', {})
if sections.get('summary'):
    lines.append('')
    lines.append('## Summary')
    lines.append('')
    lines.append(sections['summary'])

if sections.get('context'):
    lines.append('')
    lines.append('## Context')
    lines.append('')
    lines.append(sections['context'])

if sections.get('acceptance_criteria'):
    lines.append('')
    lines.append('## Acceptance Criteria')
    lines.append('')
    for crit in sections['acceptance_criteria']:
        lines.append(f'- [ ] {crit}')

# Always add empty Result and Notes sections
lines.append('')
lines.append('## Result')
lines.append('')
lines.append('## Notes')
lines.append('')

body = '\n'.join(lines)

# Output as JSON for bash to consume
result = {
    'title': issue['title'],
    'labels': issue.get('labels', []),
    'milestone': issue.get('milestone', ''),
    'body': body,
}
print(json.dumps(result))
" "$BATCH_FILE" "$i")

    TITLE=$("$PYTHON" -c "import json,sys; print(json.loads(sys.stdin.read())['title'])" <<< "$ISSUE_JSON")
    LABELS_CSV=$("$PYTHON" -c "
import json,sys
d = json.loads(sys.stdin.read())
print(','.join(d.get('labels', [])))
" <<< "$ISSUE_JSON")
    MILESTONE_TITLE=$("$PYTHON" -c "import json,sys; print(json.loads(sys.stdin.read()).get('milestone',''))" <<< "$ISSUE_JSON")
    BODY=$("$PYTHON" -c "import json,sys; print(json.loads(sys.stdin.read())['body'])" <<< "$ISSUE_JSON")

    echo ""
    echo "--- issue $(( i + 1 ))/$ISSUE_COUNT: $TITLE"

    # Validate labels
    IFS=',' read -ra LABEL_ARRAY <<< "$LABELS_CSV"
    if ! validate_labels "${LABEL_ARRAY[@]}"; then
        echo "  (continuing despite missing labels)"
    fi

    # Resolve milestone
    MILESTONE_NUM=""
    if [[ -n "$MILESTONE_TITLE" && "$MILESTONE_TITLE" != "null" ]]; then
        MILESTONE_NUM=$(resolve_milestone "$MILESTONE_TITLE")
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] would create:"
        echo "    title:     $TITLE"
        echo "    labels:    $LABELS_CSV"
        if [[ -n "$MILESTONE_NUM" ]]; then
            echo "    milestone: $MILESTONE_TITLE (#$MILESTONE_NUM)"
        fi
        echo "    body:"
        echo "$BODY" | sed 's/^/      /'
        CREATED=$(( CREATED + 1 ))
        continue
    fi

    # Build gh command
    GH_ARGS=(issue create --repo "$REPO" --title "$TITLE" --body "$BODY")
    if [[ -n "$LABELS_CSV" ]]; then
        GH_ARGS+=(--label "$LABELS_CSV")
    fi
    if [[ -n "$MILESTONE_NUM" ]]; then
        GH_ARGS+=(--milestone "$MILESTONE_NUM")
    fi

    if RESULT=$(gh "${GH_ARGS[@]}" 2>&1); then
        ISSUE_NUM=$(echo "$RESULT" | grep -oE '[0-9]+$' || echo "$RESULT")
        echo "  created: $RESULT"
        CREATED=$(( CREATED + 1 ))
    else
        echo "  FAILED: $RESULT"
        FAILED=$(( FAILED + 1 ))
    fi
done

# ── Summary ──────────────────────────────────────────────────────

echo ""
echo "==> create-issues: $CREATED created, $FAILED failed"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
