# User Interface

## The Check-in Model

The primary interaction pattern is not "sit down and work with AI." It is:

1. User opens Samverk on whatever device they have available
2. Samverk surfaces a digest: what's been completed, what's blocked, what decisions are needed
3. User spends 5-15 minutes answering questions and providing direction
4. User closes the app and goes back to their life
5. Samverk continues working

This is the most important part of the product to get right. Bad check-in UX wastes the user's entire available window.

## Device Flexibility

Users must be able to check in from:

- Desktop (Windows primary, Mac/Linux secondary)
- Laptop
- Android phone
- iOS phone
- Tablet

This is non-negotiable ([ADR-009](decisions/ADR-009-device-flexibility.md)). The async model only works if the user can interact from wherever they are. A tool that requires sitting at a specific machine defeats the purpose.

### No File Transfer, No Copy-Paste Between Devices

The user interface must provide direct access to the project from any device:

- Web-based interface, OR
- Native apps with cloud sync, OR
- API-accessible interface that other tools (like Claude on mobile) can talk to

The experience of transferring files between devices or copy-pasting content to move between contexts is explicitly a UX failure state.

## Check-in Digest Requirements

When the user opens Samverk with 10 minutes to spare, they need to immediately see:

1. **What was completed** since last check-in
2. **What is currently in progress**
3. **What is blocked** waiting for user input (prioritized -- most critical first)
4. **Autonomous decisions** the system made (for review/override)
5. **Cost burn rate** and remaining budget

Every element must be actionable or informational. No filler, no status that doesn't help the user decide what to do with their 10 minutes.

## Autonomy Policy

Samverk agents must have a configurable policy for:

- **Proceed autonomously** -- agent decides, logs decision for review at next check-in
- **Block for user input** -- stop that work stream, continue others, surface at next check-in
- **Require confirmation** -- irreversible decisions always require user sign-off

### Getting This Wrong

Getting the autonomy balance wrong in either direction is costly:

- **Too much blocking** = async value is destroyed. User comes back to no progress.
- **Too little blocking** = agents build in the wrong direction for days.

The default should lean toward autonomy (proceed, log for review) with explicit user overrides for decisions they want to control. The system should learn from override patterns over time.

## Input Methods

The user needs to provide direction during check-ins. Options to explore:

- **Chat** -- natural language, familiar, works on all devices
- **Structured forms** -- faster for repeated decision types, less ambiguous
- **Voice** -- hands-free, useful for mobile, requires transcription
- **Quick actions** -- approve/reject/defer buttons for queued decisions

The right answer is likely a combination: quick actions for simple decisions, chat for nuanced direction, structured forms for configuration.
