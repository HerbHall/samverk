package dispatcher

import (
	"context"

	"go.uber.org/zap"

	"samverk.dev/samverk/internal/store"
)

// runEventRecorder subscribes to all events on the bus and writes them to
// the pipeline_events table. Runs until ctx is cancelled.
func (d *Dispatcher) runEventRecorder(ctx context.Context) {
	ch := d.eventBus.Subscribe() // all event types

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return // bus closed
			}
			pe := store.PipelineEvent{
				IssueNumber: ev.IssueNumber,
				Project:     ev.Project,
				TriggeredBy: ev.Source,
				OccurredAt:  ev.Timestamp,
				EventType:   string(ev.Type),
				Key:         ev.Key,
				Value:       ev.Value,
				OldValue:    ev.OldValue,
			}
			if err := d.store.RecordEvent(ctx, pe); err != nil {
				d.logger.Debug("event recorder: write failed",
					zap.Int("issue", ev.IssueNumber),
					zap.String("type", string(ev.Type)),
					zap.Error(err))
			}
		}
	}
}
