package dispatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/herbhall/samverk/internal/forge"
	"github.com/herbhall/samverk/pkg/models"
)

// knownAgentTypes is the set of valid agent types for routing validation.
var knownAgentTypes = map[models.AgentType]bool{
	models.AgentTypeOrchestrator: true,
	models.AgentTypeDispatcher:   true,
	models.AgentTypeCodeGen:      true,
	models.AgentTypeTest:         true,
	models.AgentTypeDocs:         true,
	models.AgentTypeResearch:     true,
	models.AgentTypeQC:           true,
	models.AgentTypeHuman:        true,
}

// classify parses frontmatter from the issue body and validates the agent_type.
// Returns an error if frontmatter is missing or agent_type is invalid.
func (d *Dispatcher) classify(_ context.Context, issue *forge.Issue) (models.AgentType, error) {
	fm, err := d.parseFrontmatter(issue)
	if err != nil {
		return "", fmt.Errorf("classify issue #%d: %w", issue.Number, err)
	}
	if fm == nil {
		return "", fmt.Errorf("classify issue #%d: no frontmatter found", issue.Number)
	}
	if fm.AgentType == "" {
		return "", fmt.Errorf("classify issue #%d: agent_type is empty", issue.Number)
	}
	if !knownAgentTypes[fm.AgentType] {
		return "", fmt.Errorf("classify issue #%d: unknown agent_type %q", issue.Number, fm.AgentType)
	}
	return fm.AgentType, nil
}

// route assigns the issue to the agent pool matching agentType.
// It transitions the issue from queued to claimed and records it in memory.
func (d *Dispatcher) route(ctx context.Context, issue *forge.Issue, agentType models.AgentType) error {
	if err := d.tracker.RemoveLabel(ctx, issue.Number, "status:queued"); err != nil {
		d.logger.Printf("remove queued label from #%d: %v", issue.Number, err)
	}
	if err := d.tracker.AddLabel(ctx, issue.Number, "status:claimed"); err != nil {
		return fmt.Errorf("add claimed label to #%d: %w", issue.Number, err)
	}
	if err := d.tracker.Assign(ctx, issue.Number, string(agentType)); err != nil {
		return fmt.Errorf("assign #%d to %s: %w", issue.Number, agentType, err)
	}

	now := time.Now()
	d.mu.Lock()
	d.claimed[issue.Number] = &claimedIssue{
		AgentID:       string(agentType),
		ClaimedAt:     now,
		LastHeartbeat: now,
	}
	d.mu.Unlock()

	d.logger.Printf("routed issue #%d to %s", issue.Number, agentType)
	return nil
}

// parseFrontmatter extracts the IssueFrontmatter from an issue's body.
// Returns (nil, nil) if no frontmatter block exists.
func (d *Dispatcher) parseFrontmatter(issue *forge.Issue) (*models.IssueFrontmatter, error) {
	result, err := models.ParseFrontmatter(issue.Body)
	if err != nil {
		return nil, err
	}
	return result.Frontmatter, nil
}
