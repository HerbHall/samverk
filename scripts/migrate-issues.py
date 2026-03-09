#!/usr/bin/env python3
"""Migrate open GitHub issues to Gitea, preserving title, body, labels, and comments.

Usage:
    python scripts/migrate-issues.py [--dry-run] [--state open|closed|all]

Required environment variables:
    GITHUB_TOKEN    GitHub personal access token (read:repo scope)
    GITHUB_OWNER    GitHub repo owner (e.g. HerbHall)
    GITHUB_REPO     GitHub repo name (e.g. samverk)
    GITEA_URL       Gitea instance base URL (e.g. https://gitea.herbhall.net)
    GITEA_TOKEN     Gitea personal access token
    GITEA_OWNER     Gitea repo owner (e.g. samverk)
    GITEA_REPO      Gitea repo name (e.g. samverk)
"""

import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

# ── Config from environment ──────────────────────────────────────

GITHUB_TOKEN = os.environ.get("GITHUB_TOKEN", "")
GITHUB_OWNER = os.environ.get("GITHUB_OWNER", "")
GITHUB_REPO = os.environ.get("GITHUB_REPO", "")
GITEA_URL = os.environ.get("GITEA_URL", "").rstrip("/")
GITEA_TOKEN = os.environ.get("GITEA_TOKEN", "")
GITEA_OWNER = os.environ.get("GITEA_OWNER", "")
GITEA_REPO = os.environ.get("GITEA_REPO", "")

DRY_RUN = "--dry-run" in sys.argv
STATE = "open"
for i, arg in enumerate(sys.argv[1:], 1):
    if arg == "--state" and i < len(sys.argv) - 1:
        STATE = sys.argv[i + 1]

# ── Validation ────────────────────────────────────────────────────

missing = [k for k in ("GITHUB_TOKEN", "GITHUB_OWNER", "GITHUB_REPO",
                        "GITEA_URL", "GITEA_TOKEN", "GITEA_OWNER", "GITEA_REPO")
           if not os.environ.get(k)]
if missing:
    print(f"Error: missing required env vars: {', '.join(missing)}", file=sys.stderr)
    sys.exit(1)

GH_API = "https://api.github.com"
GT_API = f"{GITEA_URL}/api/v1"

# ── HTTP helpers ──────────────────────────────────────────────────

def gh_get(path, params=None):
    """GET from GitHub API, returns parsed JSON."""
    url = f"{GH_API}{path}"
    if params:
        qs = "&".join(f"{k}={urllib.parse.quote(str(v))}" for k, v in params.items())
        url = f"{url}?{qs}"
    req = urllib.request.Request(url, headers={
        "Authorization": f"Bearer {GITHUB_TOKEN}",
        "Accept": "application/vnd.github+json",
        "User-Agent": "samverk-migrate-issues",
    })
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read()), dict(r.headers)


def gh_get_all(path, params=None):
    """GET all pages from GitHub API."""
    import urllib.parse
    results = []
    page = 1
    while True:
        p = dict(params or {})
        p["per_page"] = 100
        p["page"] = page
        data, headers = gh_get(path, p)
        results.extend(data)
        link = headers.get("Link", "")
        if 'rel="next"' not in link:
            break
        page += 1
    return results


def gt_get(path):
    """GET from Gitea API, returns parsed JSON."""
    url = f"{GT_API}{path}"
    req = urllib.request.Request(url, headers={
        "Authorization": f"Bearer {GITEA_TOKEN}",
        "User-Agent": "samverk-migrate-issues",
    })
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())


def gt_post(path, payload):
    """POST to Gitea API, returns parsed JSON response."""
    url = f"{GT_API}{path}"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST", headers={
        "Authorization": f"Bearer {GITEA_TOKEN}",
        "Content-Type": "application/json",
        "User-Agent": "samverk-migrate-issues",
    })
    try:
        with urllib.request.urlopen(req) as r:
            return json.loads(r.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {e.code}: {body}") from e


# ── Gitea label resolution ────────────────────────────────────────

def load_gitea_labels():
    """Returns dict: label_name -> label_id."""
    labels = gt_get(f"/repos/{GITEA_OWNER}/{GITEA_REPO}/labels?limit=200")
    return {l["name"]: l["id"] for l in labels}


def load_gitea_milestones():
    """Returns dict: milestone_title -> milestone_id."""
    ms = gt_get(f"/repos/{GITEA_OWNER}/{GITEA_REPO}/milestones?limit=200")
    return {m["title"]: m["id"] for m in ms}


# ── GitHub issue fetching ─────────────────────────────────────────

def fetch_github_issues():
    """Returns list of GitHub issue objects (excludes PRs)."""
    issues = gh_get_all(
        f"/repos/{GITHUB_OWNER}/{GITHUB_REPO}/issues",
        {"state": STATE, "sort": "created", "direction": "asc"},
    )
    # GitHub /issues returns PRs too; filter them out.
    return [i for i in issues if "pull_request" not in i]


def fetch_github_comments(issue_number):
    """Returns list of GitHub comment objects for an issue."""
    return gh_get_all(
        f"/repos/{GITHUB_OWNER}/{GITHUB_REPO}/issues/{issue_number}/comments"
    )


# ── Gitea issue creation ──────────────────────────────────────────

def create_gitea_issue(title, body, label_ids, milestone_id):
    """Creates an issue on Gitea; returns the created issue dict."""
    payload = {
        "title": title,
        "body": body,
        "labels": label_ids,
    }
    if milestone_id:
        payload["milestone"] = milestone_id
    return gt_post(f"/repos/{GITEA_OWNER}/{GITEA_REPO}/issues", payload)


def create_gitea_comment(issue_number, body):
    """Adds a comment to a Gitea issue."""
    return gt_post(
        f"/repos/{GITEA_OWNER}/{GITEA_REPO}/issues/{issue_number}/comments",
        {"body": body},
    )


# ── Migration header ──────────────────────────────────────────────

MIGRATION_HEADER_TEMPLATE = (
    "<!-- Migrated from GitHub issue "
    "https://github.com/{gh_owner}/{gh_repo}/issues/{gh_number} -->\n\n"
)

COMMENT_HEADER_TEMPLATE = (
    "<!-- Migrated from GitHub comment by @{author} on {date} -->\n\n"
)


# ── Main ──────────────────────────────────────────────────────────

def main():
    print(f"==> migrate-issues: {GITHUB_OWNER}/{GITHUB_REPO} -> "
          f"{GITEA_URL}/{GITEA_OWNER}/{GITEA_REPO}")
    print(f"    state={STATE}  dry_run={DRY_RUN}")

    if not DRY_RUN:
        print("--- loading Gitea labels")
        gitea_labels = load_gitea_labels()
        print(f"    {len(gitea_labels)} labels loaded")

        print("--- loading Gitea milestones")
        gitea_milestones = load_gitea_milestones()
        print(f"    {len(gitea_milestones)} milestones loaded")
    else:
        gitea_labels = {}
        gitea_milestones = {}

    print("--- fetching GitHub issues")
    issues = fetch_github_issues()
    print(f"    {len(issues)} issues to migrate")

    migrated = 0
    failed = 0

    for gh_issue in issues:
        gh_num = gh_issue["number"]
        title = gh_issue["title"]
        gh_body = gh_issue.get("body") or ""

        # Prepend migration header so reviewers know the source.
        body = MIGRATION_HEADER_TEMPLATE.format(
            gh_owner=GITHUB_OWNER,
            gh_repo=GITHUB_REPO,
            gh_number=gh_num,
        ) + gh_body

        # Resolve labels to Gitea IDs.
        label_names = [l["name"] for l in gh_issue.get("labels", [])]
        label_ids = []
        for name in label_names:
            lid = gitea_labels.get(name)
            if lid is None:
                print(f"  Warning: label '{name}' not in Gitea, skipping", file=sys.stderr)
            else:
                label_ids.append(lid)

        # Resolve milestone.
        milestone_id = None
        gh_milestone = gh_issue.get("milestone")
        if gh_milestone:
            ms_title = gh_milestone["title"]
            milestone_id = gitea_milestones.get(ms_title)
            if milestone_id is None:
                print(f"  Warning: milestone '{ms_title}' not in Gitea, skipping",
                      file=sys.stderr)

        print(f"\n--- #{gh_num}: {title}")
        print(f"    labels: {label_names}")
        if gh_milestone:
            print(f"    milestone: {gh_milestone['title']}")

        if DRY_RUN:
            print("  [dry-run] would migrate")
            migrated += 1
            continue

        try:
            gt_issue = create_gitea_issue(title, body, label_ids, milestone_id)
            gt_num = gt_issue["number"]
            print(f"  created Gitea #{gt_num}")

            # Migrate comments.
            comments = fetch_github_comments(gh_num)
            for comment in comments:
                author = comment.get("user", {}).get("login", "unknown")
                date = comment.get("created_at", "")
                comment_body = (
                    COMMENT_HEADER_TEMPLATE.format(author=author, date=date)
                    + (comment.get("body") or "")
                )
                create_gitea_comment(gt_num, comment_body)
                # Be polite to the API.
                time.sleep(0.2)

            if comments:
                print(f"  migrated {len(comments)} comment(s)")

            migrated += 1
            # Small delay between issues to avoid rate limiting.
            time.sleep(0.5)

        except Exception as exc:  # noqa: BLE001 — log and continue
            print(f"  FAILED: {exc}", file=sys.stderr)
            failed += 1

    print(f"\n==> migrate-issues: {migrated} migrated, {failed} failed")
    if failed:
        sys.exit(1)


if __name__ == "__main__":
    main()
