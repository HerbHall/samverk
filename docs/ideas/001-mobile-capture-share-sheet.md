# Idea Brief: Mobile Capture via OS Share Sheet

---

schema_version: "1.0.0"
type: idea
phase: parked
priority: normal
origin: conversation, 2026-03-03

---

## Raw Input (preserved)

> Right now the phone is a real pain point for me. I always have it with me when I have free time during the day and it is perfect for researching ideas. The problem is, copy and paste doesn't work on a phone. None of the AI agents can save anything anywhere when I need them to, I can't use my work laptop for anything personal and I can only use my personal laptop at work occasionally. I really need a reliable easy way to save information from my phone to my projects. I really think there is an opportunity for a commercial product to solve these needs, but I don't have any idea how to solve it. Apparently it's really difficult or someone would have done it. My phone has a nice share feature that works well for sending some things to email, messenger, text messages. If I could have Samverk as an option in that it would solve some of the pain. Unfortunately it seems none of the AI chats support anything like this for some reason.

## Interpreted Problem

The gap between discovering something valuable on a phone and getting it into a project system is unsolved. The user's most available time (breaks at work, commuting, waiting rooms) happens on their phone, which is excellent for browsing and researching but terrible for structured capture. Copy-paste is painful on mobile. AI chat apps don't integrate with the OS share sheet. Work laptops are off-limits for personal projects. The result: valuable discoveries are lost because there's no frictionless path from "I found something interesting" to "it's saved in my project context."

This is not just a Samverk problem -- it's a universal gap in every AI-assisted development workflow. No tool currently bridges the phone's native share infrastructure with a project management or AI agent system.

## Interpreted Solution

A mobile-first capture mechanism that appears as a share target in the phone's native share sheet (Android Intent system / iOS Share Extension). When the user finds an article, GitHub repo, tool, or any web content, they tap Share -> Samverk and the content is routed to the appropriate project with minimal interaction.

The share target could:

- Accept URLs, text selections, images, screenshots, and files
- Prompt for optional context ("why is this interesting?") via a lightweight overlay
- Route to a specific project or let the system infer from content
- Queue as an Idea Brief for the ideation agent to structure
- Work entirely over the network (Tailscale to home server) with no local state

This could be a standalone product (commercial opportunity) or a Samverk feature (core capability for the async workflow).

## Initial Signals

- **Similar to**: Pocket (save-for-later), Raindrop.io (bookmarks), Apple Notes share extension, Notion Web Clipper. None of these route to an AI agent system or project context.
- **Target user**: Anyone who discovers valuable information on their phone but needs it in a project system. Primary: solo developers, researchers, knowledge workers. The Samverk target user is a perfect fit -- hobbyist dev with limited time, phone always available.
- **Novelty assessment**: The share-sheet-to-AI-agent pipeline appears to be genuinely novel. Existing share targets save to passive storage (bookmarks, notes). None feed into an active agent system that structures, researches, and acts on the captured content.
- **Commercial signal**: The user independently identified this as a potential commercial product, noting "apparently it's really difficult or someone would have done it."

## Why AI Chats Don't Do This (Hypotheses)

Research should investigate why no major AI chat (Claude, ChatGPT, Gemini, Copilot) offers share sheet integration:

1. **Platform complexity**: iOS Share Extensions and Android share targets require native code. AI chats are web-wrapped or native apps optimized for the chat UX, not system integration.
2. **Session model mismatch**: Share targets need to accept content and dismiss instantly. AI chats expect a conversation. The UX paradigm doesn't fit.
3. **No server-side persistence**: AI chat sessions are ephemeral. Shared content would need somewhere permanent to go. Most AI chats don't have a project/workspace concept.
4. **Business model**: AI chat companies monetize conversation turns, not information capture. A share target bypasses the conversation.
5. **Privacy/security**: Automatically sending web content to an AI API raises different privacy concerns than a user manually pasting into a chat.

Samverk is uniquely positioned because it HAS a server-side persistence layer (Gitea issues, Synapset semantic memory), a project concept, and an agent pipeline that can act on captured content.

## Suggested Research Questions

1. **Does this already exist?** Any share-sheet-to-project-management pipeline? Any share-sheet-to-AI integration? Check: Raycast, Obsidian mobile, Notion, Linear, Todoist, Taskwarrior mobile clients.
2. **Why haven't AI companies built this?** Is it technical difficulty, UX challenge, business model misalignment, or simply oversight?
3. **What does the Android share target API require?** Can a lightweight app (or even a PWA) register as a share target? What's the minimum viable native code?
4. **What does the iOS Share Extension require?** App Store requirements, review process, minimum viable implementation.
5. **Could this work without a native app?** Web Share Target API (Chrome on Android supports it for PWAs). Does iOS support anything similar?
6. **What's the minimum viable capture flow?** Share -> select project -> optional note -> send. How few taps?
7. **Is there a commercial market?** How many people have this exact pain point? (Likely large -- anyone who uses a phone for research and a computer for work.)
8. **What are the privacy/security implications?** Content routing through Tailscale to a home server vs. a cloud service.

## Two Product Framings

### As a Samverk Feature

- Share target routes content to Samverk's MCP endpoint as an `idea` or `research` issue type
- The ideation agent structures it into an Idea Brief
- Appears in the next check-in digest: "You shared 3 items since your last check-in. 1 matched an active research topic for Project X."
- Requires: a thin native app (Android/iOS) that acts as share target and forwards to the Samverk server via Tailscale

### As a Standalone Commercial Product

- Share target routes content to a cloud service
- AI structures and categorizes the capture
- Integrations with GitHub Issues, Linear, Notion, Obsidian, etc.
- Freemium model: free captures, paid AI structuring and routing
- Wider market than Samverk alone

These two framings are not mutually exclusive. Build it for Samverk first (dogfood), then extract and commercialize if the pattern proves valuable.

## Technical Sketch (Very Early)

```text
Phone                          Home Server
  |                                |
  | [Share Sheet]                  |
  |   -> Samverk app               |
  |     -> "Add note?" overlay     |
  |     -> Select project          |
  |     -> Send                    |
  |                                |
  | POST /api/v1/capture           |
  | (via Tailscale)                |
  | {                              |
  |   url: "...",                  |
  |   text: "...",                 |
  |   note: "user's context",     |
  |   project: "samverk",         |
  |   type: "research"            |
  | }                              |
  |                                |
  |                    Samverk Server
  |                    -> Creates issue (phase:intake)
  |                    -> Ideation agent structures it
  |                    -> Appears in next digest
```

## Existing Technology: Claude Code Remote Control

**Discovered**: 2026-03-03 (session research)

Anthropic released "Remote Control" for Claude Code (Feb 2026) which partially addresses the mobile capture pain point without requiring a custom share sheet integration.

### What It Is

Remote Control lets you start a Claude Code session on your desktop/server and continue that exact session from your phone, tablet, or any browser. Everything runs locally -- the phone is just a window into the local session.

### How It Works

1. On your machine: `claude remote-control` (or `/rc` from an existing session)
2. Displays a session URL and QR code
3. Scan QR or open URL on phone -- you're controlling the same session
4. Your filesystem, MCP servers, tools, and project config are all available remotely
5. Local session makes outbound HTTPS only -- no inbound ports. Anthropic API routes messages.
6. Auto-reconnects if laptop sleeps or network drops

### Requirements and Limitations

- **Subscription**: Max plan required (Pro coming soon). No API key support.
- **One remote session** per Claude Code instance
- **Terminal must stay running** -- if process stops, session ends
- **~10 min timeout** on extended network outages
- Can enable for ALL sessions via `/config` -> "Enable Remote Control for all sessions"

### How It Applies to the Mobile Capture Problem

**What it solves:**

- Phone access to your full project environment (filesystem, MCP servers, git) without Tailscale on the phone
- Can submit ideas, create issues, and interact with Samverk's MCP tools from mobile
- No native app needed -- works through Claude iOS/Android app or any browser at claude.ai/code
- Session state persists across devices -- start on desktop, continue on phone

**What it doesn't solve:**

- No OS-level share sheet integration (still can't Share -> Samverk from a browser tab)
- Must open Claude app, find the session, then type/paste -- more friction than a share target
- Locked to Claude (not model-agnostic like Samverk's provider model)
- Requires paid Max subscription
- Session must be actively running on your machine

### Connectivity Comparison

| Approach | Path | Requires |
|---|---|---|
| Direct MCP (Tailscale) | Phone -> Tailscale -> Samverk MCP server | Tailscale on phone |
| Remote Control | Phone -> Claude app -> Anthropic API -> local Claude Code -> Samverk MCP | Max subscription, running session |
| Share sheet (future) | Phone -> Share -> Samverk app -> Tailscale -> server | Native app + Tailscale |

### Strategic Assessment

Remote Control is a **viable bridge** until Samverk's own mobile capture exists. It eliminates the Tailscale-on-phone requirement and works today. However, it's Claude-specific, subscription-locked, and doesn't address the core UX insight (share sheet integration). The long-term share sheet solution remains the differentiated product opportunity.

### Sources

- [Claude Code Remote Control Docs](https://code.claude.com/docs/en/remote-control)
- [Anthropic: Remote MCP support](https://www.anthropic.com/news/claude-code-remote-mcp)
- [VentureBeat: Mobile version of Claude Code](https://venturebeat.com/orchestration/anthropic-just-released-a-mobile-version-of-claude-code-called-remote)
- [DevOps.com: Remote Control overview](https://devops.com/claude-code-remote-control-keeps-your-agent-local-and-puts-it-in-your-pocket/)

## Status

Parked. Requires research phase before any implementation decisions. The research questions above should be answered before scoping MVP. Claude Code Remote Control should be tested as a short-term bridge (see issue #157).

## Related Samverk Docs

- [Project Lifecycle](../project-lifecycle.md) -- Phase 1 Intake is the process this feature feeds into
- [Mobile Experience](../mobile-experience.md) -- current mobile strategy (responsive web, no native app)
- [Multi-Device Sync](../multi-device-sync.md) -- server-canonical state model this would use
- [User Interface](../user-interface.md) -- device flexibility requirement (ADR-009)
- [Shared Memory](../shared-memory.md) -- captured research could feed into Synapset (see Synapset project at `D:\DevSpace\Synapset\`)
- [MCP Server](../mcp-server.md) -- the /capture endpoint would be a new API surface
