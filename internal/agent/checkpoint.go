package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/herbhall/samverk/internal/forge"
)

// checkpointPrefix is the marker that identifies a checkpoint comment.
const checkpointPrefix = "CHECKPOINT ["

// FormatCheckpoint wraps partial output in the checkpoint comment format.
// The session ID is embedded so multi-retry scenarios can identify which
// attempt produced the checkpoint.
func FormatCheckpoint(sessionID, partialOutput string) string {
	return fmt.Sprintf("CHECKPOINT [%s]\n\n```\n%s\n```", sessionID, partialOutput)
}

// FindLatestCheckpoint scans issue comments (newest first) for the most
// recent CHECKPOINT block and returns its content. Returns empty string
// if no checkpoint exists.
func FindLatestCheckpoint(comments []*forge.Comment) string {
	// Comments are typically returned oldest-first; scan in reverse.
	for i := len(comments) - 1; i >= 0; i-- {
		body := comments[i].Body
		if content := extractCheckpointContent(body); content != "" {
			return content
		}
	}
	return ""
}

// extractCheckpointContent pulls the content from inside a CHECKPOINT comment.
// Expected format:
//
//	CHECKPOINT [session_id]
//
//	```
//	<content>
//	```
func extractCheckpointContent(body string) string {
	if !strings.HasPrefix(body, checkpointPrefix) {
		return ""
	}

	// Find the opening code fence.
	start := strings.Index(body, "```\n")
	if start < 0 {
		return ""
	}
	start += len("```\n")

	// Find the closing code fence.
	end := strings.LastIndex(body, "\n```")
	if end < 0 || end <= start {
		return ""
	}

	return body[start:end]
}

// HashCheckpoint returns a hex-encoded SHA-256 of the checkpoint content.
// Used to dedup: if a session already posted this exact checkpoint, skip.
func HashCheckpoint(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// DeduplicateEdits filters out EDIT blocks from newEdits that already exist
// in the checkpoint content. Two edits match when they have the same path
// and identical content hash. This prevents writing files that were already
// committed to the branch in a prior attempt.
func DeduplicateEdits(newEdits []FileEdit, checkpointContent string) []FileEdit {
	if checkpointContent == "" {
		return newEdits
	}

	// Parse EDIT blocks from the checkpoint.
	prior := ParseEditBlocks(checkpointContent)
	if prior == nil || len(prior.Edits) == 0 {
		return newEdits
	}

	// Build a set of path+hash for prior edits.
	existing := make(map[string]string, len(prior.Edits))
	for _, e := range prior.Edits {
		existing[e.Path] = hashContent(e.Content)
	}

	result := make([]FileEdit, 0, len(newEdits))
	for _, e := range newEdits {
		priorHash, found := existing[e.Path]
		if found && priorHash == hashContent(e.Content) {
			// Identical file — already on the branch from a prior attempt.
			continue
		}
		result = append(result, e)
	}
	return result
}

// hashContent returns a hex-encoded SHA-256 of file content.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// BuildResumePrompt creates the system prompt injection that instructs the
// model to continue from a prior checkpoint rather than starting over.
func BuildResumePrompt(checkpointContent string) string {
	return fmt.Sprintf(`You previously produced the following partial work on this issue. Continue from where you left off. Do not regenerate completed sections. If the partial work contains EDIT blocks, those files are already written — focus on remaining files and the PR creation step.

<prior-work>
%s
</prior-work>`, checkpointContent)
}
