#!/usr/bin/env bash
# Create issues from a JSON batch file on GitHub, Gitea, or both.
# Usage: bash scripts/create-issues.sh [--dry-run] [--forge github|gitea|both] path/to/batch.json
#
# GitHub (default): uses gh CLI; reads GITHUB_TOKEN automatically.
# Gitea: uses curl against the Gitea REST API.
#   Required env vars for Gitea: GITEA_URL, GITEA_TOKEN, GITEA_OWNER, GITEA_REPO
set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────

DRY_RUN=false
BATCH_FILE=""
FORGE="github"  # github | gitea | both

for arg in "$@"; do
    case "$arg" in
        --dry-run)        DRY_RUN=true ;;
        --forge=*)        FORGE="${arg#--forge=}" ;;
        --forge)          ;;  # next arg handled below
        *)                BATCH_FILE="$arg" ;;
    esac
done

# Handle "--forge <value>" (two separate args)
prev=""
for arg in "$@"; do
    if [[ "$prev" == "--forge" ]]; then
        FORGE="$arg"
    fi
    prev="$arg"
done

if [[ "$FORGE" != "github" && "$FORGE" != "gitea" && "$FORGE" != "both" ]]; then
    echo "Error: --forge must be github, gitea, or both (got: $FORGE)"
    exit 1
fi

if [[ -z "$BATCH_FILE" ]]; then
    echo "Usage: bash scripts/create-issues.sh [--dry-run] [--forge github|gitea|both] <batch.json>"
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

# Force UTF-8 I/O on Windows (prevents cp1252 errors with Unicode in issue bodies)
export PYTHONIOENCODING=utf-8

# ── Detect GitHub repo owner/name from git remote ────────────────

GH_REPO=$(git remote get-url origin 2>/dev/null \
    | sed -E 's#.*github\.com[:/]##; s#\.git$##' || true)

# ── Gitea connection config from environment ─────────────────────

GITEA_URL="${GITEA_URL:-}"
GITEA_TOKEN="${GITEA_TOKEN:-}"
GITEA_OWNER="${GITEA_OWNER:-}"
GITEA_REPO="${GITEA_REPO:-}"

# ── Validate forge prerequisites ─────────────────────────────────

if [[ "$FORGE" == "github" || "$FORGE" == "both" ]]; then
    if [[ -z "$GH_REPO" ]]; then
        echo "Error: could not detect GitHub repo from git remote 'origin'."
        exit 1
    fi
fi

if [[ "$FORGE" == "gitea" || "$FORGE" == "both" ]]; then
    if [[ -z "$GITEA_URL" || -z "$GITEA_TOKEN" || -z "$GITEA_OWNER" || -z "$GITEA_REPO" ]]; then
        echo "Error: GITEA_URL, GITEA_TOKEN, GITEA_OWNER, and GITEA_REPO must be set for Gitea."
        exit 1
    fi
fi

echo "==> create-issues: forge=$FORGE dry_run=$DRY_RUN"
if [[ "$FORGE" == "github" || "$FORGE" == "both" ]]; then
    echo "    github: $GH_REPO"
fi
if [[ "$FORGE" == "gitea" || "$FORGE" == "both" ]]; then
    echo "    gitea:  $GITEA_URL/$GITEA_OWNER/$GITEA_REPO"
fi

# ── GitHub helpers ────────────────────────────────────────────────

if [[ "$FORGE" == "github" || "$FORGE" == "both" ]]; then
    echo "--- resolving GitHub milestones"
    GH_MILESTONE_MAP=$(gh api "repos/$GH_REPO/milestones" --paginate \
        --jq '.[] | "\(.title)\t\(.number)"' 2>/dev/null || true)

    gh_resolve_milestone() {
        local title="$1"
        if [[ -z "$title" || "$title" == "null" ]]; then echo ""; return; fi
        local num
        num=$(echo "$GH_MILESTONE_MAP" | while IFS=$'\t' read -r mt mn; do
            if [[ "$mt" == "$title" ]]; then echo "$mn"; break; fi
        done)
        if [[ -z "$num" ]]; then
            echo "Warning: GitHub milestone '$title' not found -- skipping" >&2
        fi
        echo "$num"
    }

    echo "--- validating GitHub labels"
    GH_REPO_LABELS=$(gh label list --repo "$GH_REPO" --json name --jq '.[].name' --limit 200)

    gh_validate_labels() {
        local missing=()
        for label in "$@"; do
            if ! echo "$GH_REPO_LABELS" | grep -qxF "$label"; then
                missing+=("$label")
            fi
        done
        if [[ ${#missing[@]} -gt 0 ]]; then
            echo "Warning: GitHub labels not in repo: ${missing[*]}" >&2
            return 1
        fi
        return 0
    }
fi

# ── Gitea helpers ─────────────────────────────────────────────────

if [[ "$FORGE" == "gitea" || "$FORGE" == "both" ]]; then
    GITEA_API="$GITEA_URL/api/v1"

    echo "--- resolving Gitea labels"
    # Build name→id map (tab-separated)
    GITEA_LABEL_MAP=$("$PYTHON" -c "
import json, sys, urllib.request

url = sys.argv[1] + '/repos/' + sys.argv[2] + '/' + sys.argv[3] + '/labels?limit=200'
req = urllib.request.Request(url, headers={'Authorization': 'Bearer ' + sys.argv[4], 'User-Agent': 'samverk-scripts'})
try:
    with urllib.request.urlopen(req) as r:
        labels = json.loads(r.read())
        for l in labels:
            print(l['name'] + '\t' + str(l['id']))
except Exception as e:
    print('Error fetching Gitea labels: ' + str(e), file=sys.stderr)
    sys.exit(1)
" "$GITEA_API" "$GITEA_OWNER" "$GITEA_REPO" "$GITEA_TOKEN")

    gitea_resolve_label_id() {
        local name="$1"
        local id
        id=$(echo "$GITEA_LABEL_MAP" | while IFS=$'\t' read -r ln li; do
            if [[ "$ln" == "$name" ]]; then echo "$li"; break; fi
        done)
        echo "$id"
    }

    gitea_resolve_label_ids() {
        # Accepts comma-separated label names, returns JSON array of IDs
        local csv="$1"
        local ids="["
        local first=true
        IFS=',' read -ra LABELS <<< "$csv"
        for label in "${LABELS[@]}"; do
            local id
            id=$(gitea_resolve_label_id "$label")
            if [[ -z "$id" ]]; then
                echo "Warning: Gitea label '$label' not found -- skipping" >&2
                continue
            fi
            if [[ "$first" == "true" ]]; then
                ids+="$id"
                first=false
            else
                ids+=",$id"
            fi
        done
        ids+="]"
        echo "$ids"
    }

    echo "--- resolving Gitea milestones"
    GITEA_MILESTONE_MAP=$("$PYTHON" -c "
import json, sys, urllib.request

url = sys.argv[1] + '/repos/' + sys.argv[2] + '/' + sys.argv[3] + '/milestones?limit=200'
req = urllib.request.Request(url, headers={'Authorization': 'Bearer ' + sys.argv[4], 'User-Agent': 'samverk-scripts'})
try:
    with urllib.request.urlopen(req) as r:
        ms = json.loads(r.read())
        for m in ms:
            print(m['title'] + '\t' + str(m['id']))
except Exception as e:
    print('Error fetching Gitea milestones: ' + str(e), file=sys.stderr)
    sys.exit(1)
" "$GITEA_API" "$GITEA_OWNER" "$GITEA_REPO" "$GITEA_TOKEN")

    gitea_resolve_milestone() {
        local title="$1"
        if [[ -z "$title" || "$title" == "null" ]]; then echo ""; return; fi
        local id
        id=$(echo "$GITEA_MILESTONE_MAP" | while IFS=$'\t' read -r mt mi; do
            if [[ "$mt" == "$title" ]]; then echo "$mi"; break; fi
        done)
        if [[ -z "$id" ]]; then
            echo "Warning: Gitea milestone '$title' not found -- skipping" >&2
        fi
        echo "$id"
    }

    gitea_create_issue() {
        local title="$1"
        local body="$2"
        local labels_json="$3"
        local milestone_id="$4"

        "$PYTHON" -c "
import json, sys, urllib.request

api = sys.argv[1]
owner = sys.argv[2]
repo = sys.argv[3]
token = sys.argv[4]

payload = {
    'title': sys.argv[5],
    'body': sys.argv[6],
    'labels': json.loads(sys.argv[7]),
}
ms_id = sys.argv[8]
if ms_id and ms_id != '':
    payload['milestone'] = int(ms_id)

data = json.dumps(payload).encode('utf-8')
url = f'{api}/repos/{owner}/{repo}/issues'
req = urllib.request.Request(url, data=data, method='POST', headers={
    'Authorization': 'Bearer ' + token,
    'Content-Type': 'application/json',
    'User-Agent': 'samverk-scripts',
})
try:
    with urllib.request.urlopen(req) as r:
        resp = json.loads(r.read())
        print(f\"created #{resp['number']}: {resp['html_url']}\")
except urllib.error.HTTPError as e:
    body_err = e.read().decode('utf-8', errors='replace')
    print(f'HTTP {e.code}: {body_err}', file=sys.stderr)
    sys.exit(1)
" "$GITEA_API" "$GITEA_OWNER" "$GITEA_REPO" "$GITEA_TOKEN" \
  "$title" "$body" "$labels_json" "$milestone_id"
    }
fi

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

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] would create:"
        echo "    title:     $TITLE"
        echo "    labels:    $LABELS_CSV"
        if [[ -n "$MILESTONE_TITLE" && "$MILESTONE_TITLE" != "null" ]]; then
            echo "    milestone: $MILESTONE_TITLE"
        fi
        echo "    body:"
        echo "$BODY" | sed 's/^/      /'
        CREATED=$(( CREATED + 1 ))
        continue
    fi

    # ── GitHub path ───────────────────────────────────────────────

    if [[ "$FORGE" == "github" || "$FORGE" == "both" ]]; then
        IFS=',' read -ra LABEL_ARRAY <<< "$LABELS_CSV"
        if ! gh_validate_labels "${LABEL_ARRAY[@]}"; then
            echo "  (continuing despite missing GitHub labels)"
        fi

        MILESTONE_NUM=""
        if [[ -n "$MILESTONE_TITLE" && "$MILESTONE_TITLE" != "null" ]]; then
            MILESTONE_NUM=$(gh_resolve_milestone "$MILESTONE_TITLE")
        fi

        GH_ARGS=(issue create --repo "$GH_REPO" --title "$TITLE" --body "$BODY")
        if [[ -n "$LABELS_CSV" ]]; then
            GH_ARGS+=(--label "$LABELS_CSV")
        fi
        if [[ -n "$MILESTONE_TITLE" && "$MILESTONE_TITLE" != "null" ]]; then
            GH_ARGS+=(--milestone "$MILESTONE_TITLE")
        fi

        if RESULT=$(gh "${GH_ARGS[@]}" 2>&1); then
            echo "  [github] created: $RESULT"
            CREATED=$(( CREATED + 1 ))
        else
            echo "  [github] FAILED: $RESULT"
            FAILED=$(( FAILED + 1 ))
        fi
    fi

    # ── Gitea path ────────────────────────────────────────────────

    if [[ "$FORGE" == "gitea" || "$FORGE" == "both" ]]; then
        LABEL_IDS_JSON="[]"
        if [[ -n "$LABELS_CSV" ]]; then
            LABEL_IDS_JSON=$(gitea_resolve_label_ids "$LABELS_CSV")
        fi

        GITEA_MS_ID=""
        if [[ -n "$MILESTONE_TITLE" && "$MILESTONE_TITLE" != "null" ]]; then
            GITEA_MS_ID=$(gitea_resolve_milestone "$MILESTONE_TITLE")
        fi

        if RESULT=$(gitea_create_issue "$TITLE" "$BODY" "$LABEL_IDS_JSON" "$GITEA_MS_ID" 2>&1); then
            echo "  [gitea] $RESULT"
            CREATED=$(( CREATED + 1 ))
        else
            echo "  [gitea] FAILED: $RESULT"
            FAILED=$(( FAILED + 1 ))
        fi
    fi
done

# ── Summary ──────────────────────────────────────────────────────

echo ""
echo "==> create-issues: $CREATED created, $FAILED failed"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
