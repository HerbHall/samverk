package mcp

import (
	"fmt"
	"regexp"
	"strings"

	"samverk.dev/samverk/pkg/models"
)

// conventionalCommitPrefixRe matches a valid conventional commit prefix in an issue title.
// Accepted forms: "type: ", "type(scope): ", "type!: ", "type(scope)!: "
var conventionalCommitPrefixRe = regexp.MustCompile(`^(feat|fix|chore|docs|test|ci|style|refactor)(\(.+\))?(!)?: `)

// agentLabelPrefixRe matches any label that begins with "agent:".
var agentLabelPrefixRe = regexp.MustCompile(`^agent:`)

// priorityLabelPrefixRe matches any label that begins with "priority:".
var priorityLabelPrefixRe = regexp.MustCompile(`^priority:`)

// agentTypeFromPrefix derives the default agent-type label from the conventional commit type.
// If the type is unknown it falls back to "agent:code-gen".
func agentTypeFromPrefix(title string) string {
	m := conventionalCommitPrefixRe.FindStringSubmatch(title)
	if len(m) == 0 {
		return models.LabelAgentCodeGen
	}
	switch m[1] {
	case "docs":
		return models.LabelAgentDocs
	default:
		// feat, fix, chore, test, ci, style, refactor -> code-gen
		return models.LabelAgentCodeGen
	}
}

// validateAndEnrichIssue applies dispatcher-readiness validations to a single
// issue title and label list. It returns the (possibly augmented) labels and
// an error if any hard requirement fails. The body is returned with optional
// YAML frontmatter prepended when frontmatter is non-nil.
//
// Validations applied:
//  1. Title must begin with a conventional commit prefix.
//  2. Labels must contain an "agent:*" entry; if absent one is auto-injected
//     based on the commit type heuristic.
//  3. Labels must contain a "priority:*" entry; if absent "priority:normal"
//     is injected.
//  4. Body is prepended with YAML frontmatter when frontmatter is non-nil.
func validateAndEnrichIssue(title, body string, labels []string, frontmatter map[string]string) (enrichedLabels []string, enrichedBody string, err error) {
	// 1. Title prefix validation.
	if !conventionalCommitPrefixRe.MatchString(title) {
		return nil, "", fmt.Errorf(
			"issue title must begin with a conventional commit prefix (e.g. feat: , fix: , chore: ); got %q",
			title,
		)
	}

	// Copy labels so callers are not surprised by in-place mutation.
	out := make([]string, len(labels))
	copy(out, labels)

	// 2. Agent label injection.
	hasAgent := false
	for _, l := range out {
		if agentLabelPrefixRe.MatchString(l) {
			hasAgent = true
			break
		}
	}
	if !hasAgent {
		out = append(out, agentTypeFromPrefix(title))
	}

	// 3. Priority default injection.
	hasPriority := false
	for _, l := range out {
		if priorityLabelPrefixRe.MatchString(l) {
			hasPriority = true
			break
		}
	}
	if !hasPriority {
		out = append(out, models.LabelPriorityNormal)
	}

	// 4. Frontmatter prepend.
	enrichedBody = body
	if len(frontmatter) > 0 {
		var sb strings.Builder
		sb.WriteString("---\n")
		for k, v := range frontmatter {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
		sb.WriteString(body)
		enrichedBody = sb.String()
	}

	return out, enrichedBody, nil
}
