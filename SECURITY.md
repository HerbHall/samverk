# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Samverk, please report it responsibly.

**Do not open a public issue.** Instead, use one of these methods:

1. **Gitea Private Reporting**: Use the security advisory feature on the [Samverk repository](https://gitea.herbhall.net/samverk/samverk)
2. **Email**: Contact the maintainer directly

## What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Affected component (dispatcher, API, dashboard, agent execution)
- Suggested fix (if you have one)

### Samverk-Specific Concerns

Samverk orchestrates autonomous agent execution. Pay special attention to:

- **API key exposure**: Claude API keys, Gitea PATs, or GitHub tokens used by the dispatcher
- **Agent autonomy escalation**: Vulnerabilities that could allow Tier 1 actions to bypass Tier 2/3 confirmation gates
- **Cost manipulation**: Attacks that could trigger excessive API spending through the agent pipeline
- **Injection via issue content**: Malicious payloads in Gitea issue titles or bodies that agents execute
- **Dashboard XSS**: Server-injected values in the embedded SPA (auth tokens, config) must be escaped
- **MCP endpoint security**: Unauthorized access to Samverk's MCP tool surface

## Response Timeline

- **Acknowledgement**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix or mitigation**: Depends on severity, targeting 30 days for critical issues
- API keys and tokens are rotated immediately upon confirmed exposure

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |
| Older   | No        |
