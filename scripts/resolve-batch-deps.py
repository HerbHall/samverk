#!/usr/bin/env python3
"""
resolve-batch-deps.py — Replace string batch refs in depends_on with real GitHub issue numbers.

Usage:
    python scripts/resolve-batch-deps.py [--dry-run]

Pass 1: Load all three batch JSON files to build ref→title map.
Pass 2: Fetch live issues from GitHub (filtered by the three milestones).
Pass 3: For each issue whose frontmatter depends_on contains string refs,
        patch the issue body with resolved integer issue numbers.

Requires: gh CLI authenticated and on PATH, Python 3.8+
"""
import json
import re
import subprocess
import sys
import os

# Force UTF-8 stdout on Windows (prevents cp1252 errors with Unicode in print output)
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

BATCH_FILES = [
    "scripts/gitea-migration-batch.json",
    "scripts/adaptive-scaling-batch.json",
    "scripts/pc-agent-batch.json",
]

MILESTONE_TITLES = ["Gitea Migration", "Adaptive Worker Scaling", "PC Agent Worker"]

DRY_RUN = "--dry-run" in sys.argv

# ── Step 1: Build ref→title map from batch files ──────────────────────────────

ref_to_title: dict[str, str] = {}
for path in BATCH_FILES:
    with open(path, encoding="utf-8") as f:
        batch = json.load(f)
    for issue in batch["issues"]:
        ref = issue.get("_batch_ref")
        title = issue.get("title")
        if ref and title:
            ref_to_title[ref] = title

print(f"[pass 1] loaded {len(ref_to_title)} batch refs from {len(BATCH_FILES)} files")

# ── Step 2: Detect repo from git remote ───────────────────────────────────────

result = subprocess.run(
    ["git", "remote", "get-url", "origin"],
    capture_output=True, text=True, check=True
)
repo = result.stdout.strip()
repo = re.sub(r".*github\.com[:/]", "", repo).removesuffix(".git")
print(f"[pass 2] repo: {repo}")

# Resolve milestone titles → numbers (GitHub API requires the number for filtering)
raw_ms = subprocess.run(
    ["gh", "api", f"repos/{repo}/milestones", "--paginate",
     "--jq", ".[] | {title, number}"],
    capture_output=True, text=True, encoding="utf-8",
)
milestone_name_to_num: dict[str, int] = {}
for line in raw_ms.stdout.splitlines():
    line = line.strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
        milestone_name_to_num[obj["title"]] = obj["number"]
    except json.JSONDecodeError:
        pass

MILESTONES: dict[str, int] = {}
for title in MILESTONE_TITLES:
    if title in milestone_name_to_num:
        MILESTONES[title] = milestone_name_to_num[title]
        print(f"  milestone '{title}' → #{milestone_name_to_num[title]}")
    else:
        print(f"  WARNING: milestone '{title}' not found in repo")

# Fetch all open+closed issues from the three milestones (paginated)
title_to_number: dict[str, int] = {}
for milestone_title, milestone_num in MILESTONES.items():
    for state in ("open", "closed"):
        raw = subprocess.run(
            [
                "gh", "api",
                f"repos/{repo}/issues?milestone={milestone_num}&state={state}&per_page=100",
                "--paginate",
                "--jq", ".[] | {number, title}",
            ],
            capture_output=True, text=True, encoding="utf-8",
        )
        if raw.returncode != 0:
            print(f"  warning: gh api failed for milestone={milestone_title} state={state}: {raw.stderr.strip()}")
            continue
        for line in raw.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
                title_to_number[obj["title"]] = obj["number"]
            except json.JSONDecodeError:
                pass

print(f"[pass 2] fetched {len(title_to_number)} issues from GitHub")

# Build ref→number via ref→title→number
ref_to_number: dict[str, int] = {}
missing_refs: list[str] = []
for ref, title in ref_to_title.items():
    if title in title_to_number:
        ref_to_number[ref] = title_to_number[title]
    else:
        missing_refs.append(ref)

if missing_refs:
    print(f"[pass 2] WARNING: could not resolve {len(missing_refs)} refs: {missing_refs}")
else:
    print(f"[pass 2] all {len(ref_to_number)} refs resolved to issue numbers")

# ── Step 3: Patch issue bodies ────────────────────────────────────────────────

FRONTMATTER_RE = re.compile(r"^---\n(.*?)\n---", re.DOTALL)
DEPENDS_ON_RE = re.compile(r"^depends_on:\s*\[([^\]]*)\]", re.MULTILINE)

def resolve_depends_on(value: str) -> tuple[list[int], bool]:
    """Parse the depends_on value string, resolve refs, return (numbers, changed)."""
    items = [v.strip().strip('"').strip("'") for v in value.split(",") if v.strip()]
    resolved = []
    changed = False
    for item in items:
        if not item:
            continue
        # Already an integer
        if item.isdigit():
            resolved.append(int(item))
        elif item in ref_to_number:
            resolved.append(ref_to_number[item])
            changed = True
        else:
            print(f"    WARNING: unknown ref '{item}' — left as-is")
            resolved.append(item)  # type: ignore[arg-type]
    return resolved, changed

patched = 0
skipped = 0
failed = 0

for milestone_title, milestone_num in MILESTONES.items():
    raw = subprocess.run(
        [
            "gh", "api",
            f"repos/{repo}/issues?milestone={milestone_num}&state=open&per_page=100",
            "--paginate",
            "--jq", ".[] | {number, title, body}",
        ],
        capture_output=True, text=True, encoding="utf-8",
    )
    if raw.returncode != 0:
        print(f"  error fetching issues for milestone={milestone_title}: {raw.stderr.strip()}")
        continue

    for line in raw.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue

        number = obj["number"]
        title = obj["title"]
        body = obj.get("body") or ""
        # Normalize CRLF → LF (GitHub API returns \r\n on Windows)
        body = body.replace("\r\n", "\n")

        # Find frontmatter block
        fm_match = FRONTMATTER_RE.search(body)
        if not fm_match:
            continue

        fm_block = fm_match.group(1)
        dep_match = DEPENDS_ON_RE.search(fm_block)
        if not dep_match:
            continue

        dep_value = dep_match.group(1).strip()
        if not dep_value:
            continue  # empty list, nothing to resolve

        resolved, changed = resolve_depends_on(dep_value)
        if not changed:
            skipped += 1
            continue

        # Rebuild depends_on line
        new_dep_line = f"depends_on: [{', '.join(str(x) for x in resolved)}]"
        new_fm_block = DEPENDS_ON_RE.sub(new_dep_line, fm_block)
        new_body = body.replace(fm_block, new_fm_block, 1)

        print(f"  #{number} {title[:60]}")
        print(f"    depends_on: [{dep_value}] → [{', '.join(str(x) for x in resolved)}]")

        if DRY_RUN:
            patched += 1
            continue

        patch = subprocess.run(
            [
                "gh", "api",
                f"repos/{repo}/issues/{number}",
                "-X", "PATCH",
                "-f", f"body={new_body}",
            ],
            capture_output=True, text=True, encoding="utf-8",
        )
        if patch.returncode == 0:
            patched += 1
        else:
            print(f"    FAILED: {patch.stderr.strip()}")
            failed += 1

# ── Summary ───────────────────────────────────────────────────────────────────

print()
print(f"[pass 3] patched={patched} skipped={skipped} failed={failed}")
if DRY_RUN:
    print("[dry-run] no changes written to GitHub")
if failed:
    sys.exit(1)
