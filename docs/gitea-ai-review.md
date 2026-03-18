# Gitea AI Code Review

Automated AI-powered code review for Samverk-managed Gitea projects using
[ai-review](https://github.com/Nikita-Filonov/ai-review).

## How It Works

When a pull request is opened or updated on a Gitea project, the AI review
workflow runs automatically via Gitea Actions. It:

1. Fetches the PR diff from the Gitea API
2. Sends the diff to an AI provider (Claude or Ollama) for review
3. Posts inline comments on specific lines and a summary review comment

## Multi-Model Review Pipeline

The workflow uses a two-tier provider strategy:

| Priority | Provider | Model | When |
|----------|----------|-------|------|
| Primary | Claude (Anthropic) | claude-sonnet-4-20250514 | When `ANTHROPIC_API_KEY` secret is set |
| Fallback | Ollama | qwen2.5-coder:7b | When Claude key is unavailable |

Claude provides higher-quality reviews but requires an API key and incurs
costs. Ollama runs on the local infrastructure (VM 300, 192.168.1.207:11434)
at zero marginal cost but with lower review quality.

## Adding AI Review to a New Gitea Project

### Prerequisites

- Gitea Actions runner (`act_runner`) with Docker support on the Gitea host
- Repository-level secrets configured (see below)

### Step 1: Copy the workflow template

Copy `overlay/templates/gitea-ai-review.yml` to the target repo:

```bash
mkdir -p .gitea/workflows
cp overlay/templates/gitea-ai-review.yml .gitea/workflows/ai-review.yml
```

Or push via the Gitea API:

```bash
GITEA_URL="https://gitea.herbhall.net/api/v1"
OWNER="samverk"
REPO="your-project"

curl -X POST "$GITEA_URL/repos/$OWNER/$REPO/contents/.gitea/workflows/ai-review.yml" \
  -H "Authorization: token $GITEA_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"message\": \"ci: add AI code review workflow\",
    \"content\": \"$(base64 -w0 overlay/templates/gitea-ai-review.yml)\"
  }"
```

### Step 2: Configure secrets

In the Gitea repository settings (Settings > Actions > Secrets), add:

| Secret | Required | Description |
|--------|----------|-------------|
| `GITEA_TOKEN` | Yes | Personal access token with `repo` scope |
| `ANTHROPIC_API_KEY` | No | Anthropic API key for Claude reviews |

If `ANTHROPIC_API_KEY` is not set, the workflow falls back to Ollama
automatically.

### Step 3: (Optional) Configure Ollama host

If your Ollama instance is at a non-default address, set a repository
variable (Settings > Actions > Variables):

| Variable | Default | Description |
|----------|---------|-------------|
| `OLLAMA_HOST` | `http://192.168.1.207:11434` | Ollama API endpoint |

## Review Modes

The workflow uses `REVIEW__MODE: "added"` by default, reviewing only added
lines in the diff. Available modes:

| Mode | Description |
|------|-------------|
| `added` | Review only added lines (default, recommended) |
| `full` | Review the entire diff including removed lines |
| `removed` | Review only removed lines |
| `with-context` | Include surrounding context in the review |

## Customizing the Review

### Change the AI model

Edit the `LLM__META__MODEL` env var in the workflow. Supported Claude models:
`claude-sonnet-4-20250514`, `claude-3-haiku-20240307`. Supported Ollama models: any model
pulled on the Ollama host (`qwen2.5-coder:7b`, `deepseek-r1:7b`, etc.).

### Limit inline comments

Set `REVIEW__MAX_INLINE_COMMENTS` to control the maximum number of inline
comments per review (default: 20 for Claude, 15 for Ollama).

### File filters

Add `REVIEW__INCLUDE_FILES` or `REVIEW__EXCLUDE_FILES` with glob patterns
to scope the review to specific files:

```yaml
REVIEW__INCLUDE_FILES: "*.go,*.ts"
REVIEW__EXCLUDE_FILES: "vendor/**,*.generated.go"
```

## Infrastructure

| Component | Host | Notes |
|-----------|------|-------|
| Gitea | CT 200 (192.168.1.160:3000) | `gitea.herbhall.net` |
| Ollama | VM 300 (192.168.1.207:11434) | RTX 3090 Ti, qwen2.5-coder:7b |
| Samverk | CT 202 (192.168.1.162:8080) | Dispatcher + PR watcher |

## Comparison with GitHub Copilot Review

| Feature | GitHub Copilot | Gitea AI Review |
|---------|---------------|-----------------|
| Inline comments | Yes | Yes |
| Summary review | Yes | Yes |
| Can approve PRs | No (COMMENT only) | No (COMMENT only) |
| Provider | GitHub-managed | Claude or Ollama (configurable) |
| Cost | Included in Copilot plan | Anthropic API or free (Ollama) |
| Self-hosted | No | Yes |

## Template Location

The canonical workflow template is at `overlay/templates/gitea-ai-review.yml`.
New Samverk-managed Gitea projects should include this workflow during
scaffolding.

## Related Documents

- [PR Merge Policy](pr-merge-policy.md) — tier-based merge rules
- [Overlay README](../overlay/README.md) — overlay architecture
- [ADR-031: Dual-Forge Operational Model](decisions/ADR-031-dual-forge-operational-model.md)
