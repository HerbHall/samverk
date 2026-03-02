# Claude Desktop MCP Integration

How to configure Claude Desktop to use Samverk as an MCP server.

## Prerequisites

- Claude Desktop installed
- Go toolchain (for `go run` or a compiled `samverk` binary)
- GitHub personal access token with `repo` scope
- A GitHub repository to manage issues against

## 1. Start the Samverk Server

```bash
export GITHUB_TOKEN="ghp_your_token_here"
export SAMVERK_AUTH_TOKEN="your-secret-bearer-token"

go run ./cmd/samverk serve \
  --addr :8080 \
  --owner YourGitHubUser \
  --repo your-repo \
  --db .samverk/samverk.db \
  --budget 50.0
```

Verify it is running:

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

## 2. Configure Claude Desktop

Edit the Claude Desktop MCP configuration file:

- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

Add Samverk as a Streamable HTTP MCP server:

```json
{
  "mcpServers": {
    "samverk": {
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-secret-bearer-token"
      }
    }
  }
}
```

Restart Claude Desktop after saving.

## 3. Available Tools

After connecting, Claude Desktop should discover these tools:

| Tool | Description |
|------|-------------|
| `get_digest` | Check-in digest: pending decisions, completed work, project status |
| `get_cost_summary` | Token usage and cost summary for a time period |
| `add_label` | Add a label to a GitHub issue |
| `remove_label` | Remove a label from a GitHub issue |
| `add_comment` | Add a comment to a GitHub issue |
| `create_issue` | Create a new GitHub issue |

## 4. Verification Steps

### Tool Discovery

Ask Claude: *"What tools do you have available from Samverk?"*

Claude should list all 6 tools with their descriptions.

### Get Digest

Ask Claude: *"Show me my project digest for the last 48 hours."*

Claude calls `get_digest` with `{"since": "48h"}` and returns a formatted
summary of pending decisions, completed work, and active issues.

### Cost Summary

Ask Claude: *"How much have I spent on AI in the last 24 hours?"*

Claude calls `get_cost_summary` with `{"since": "24h"}` and returns
token usage and cost data from the SQLite store.

### Issue Operations

Ask Claude: *"Add the label 'priority:high' to issue #5."*

Claude calls `add_label` with `{"issue_number": 5, "label": "priority:high"}`.

### Create Issue

Ask Claude: *"Create an issue titled 'Fix login timeout' with body describing the bug."*

Claude calls `create_issue` and returns the new issue number.

## 5. Troubleshooting

### Tools not appearing

- Verify the server is running: `curl http://localhost:8080/healthz`
- Check the URL in config matches the server address
- Ensure the Authorization header matches `SAMVERK_AUTH_TOKEN`
- Restart Claude Desktop after config changes

### Authentication errors (401)

- `SAMVERK_AUTH_TOKEN` must be set when starting the server
- The Bearer token in Claude Desktop config must match exactly
- Health endpoint (`/healthz`) does not require authentication

### MCP handler disabled

If the server logs `MCP handler disabled`, ensure all three are set:

- `GITHUB_TOKEN` environment variable
- `--owner` flag (or `SAMVERK_GITHUB_OWNER`)
- `--repo` flag (or `SAMVERK_GITHUB_REPO`)

### Empty cost data

If `get_cost_summary` returns "no cost data available":

- Ensure `--db` flag points to a valid path (default: `.samverk/samverk.db`)
- The database is created automatically on first run
- Cost records accumulate as tools are called

## 6. Protocol Notes

- Transport: Streamable HTTP (stateless JSON response mode)
- Content-Type: `application/json`
- Authentication: Bearer token via `Authorization` header
- The server responds to JSON-RPC 2.0 requests at `/mcp`
- No SSE streaming -- all responses are synchronous JSON
