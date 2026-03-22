package dispatcher

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/herbhall/samverk/internal/store"
)

// recordPipelineEvent persists a stage transition to the pipeline_events table.
// It is a no-op when the store is not configured.
func (d *Dispatcher) recordPipelineEvent(ctx context.Context, owner, repo string, issueNumber int, fromStage, toStage, triggeredBy string) {
	if d.store == nil {
		return
	}
	project := owner + "/" + repo
	ev := store.PipelineEvent{
		IssueNumber: issueNumber,
		Project:     project,
		FromStage:   fromStage,
		ToStage:     toStage,
		TriggeredBy: triggeredBy,
		OccurredAt:  time.Now().UTC(),
	}
	if err := d.store.RecordPipelineEvent(ctx, ev); err != nil {
		d.logger.Error("record pipeline event",
			zap.Int("issue", issueNumber),
			zap.String("from", fromStage),
			zap.String("to", toStage),
			zap.Error(err),
		)
	}
}
