package dispatcher

import "encoding/json"

// EventBroadcaster publishes serialized JSON events to connected WebSocket clients.
// server.Hub satisfies this interface.
type EventBroadcaster interface {
	Broadcast([]byte)
}

// broadcastEvent marshals eventType+data as JSON and sends to b.
// No-op if b is nil or marshaling fails.
func broadcastEvent(b EventBroadcaster, eventType string, data any) {
	if b == nil {
		return
	}
	msg := struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}{
		Type: eventType,
		Data: data,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	b.Broadcast(payload)
}
