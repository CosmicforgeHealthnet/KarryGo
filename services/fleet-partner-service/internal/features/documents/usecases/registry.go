package usecases

import "cosmicforge/logistics/services/fleet-partner-service/internal/events"

// Section 26 gap-close: register this domain's produced event types in the
// central events.Registry, so "what does the documents domain publish, and
// what does each event mean" has one machine-checkable answer instead of
// only the comment beside each `const Event... =` declaration above.
func init() {
	register := func(eventType, description string) {
		events.Register(events.EventDefinition{
			Type: eventType, SchemaVersion: 1, AggregateType: "document", Description: description,
		})
	}
	register(EventDocumentRegistered, "A new document reference was created and an upload was authorized.")
	register(EventDocumentStateChanged, "Media's authoritative projection of a document moved to a new lifecycle state.")
	register(EventDocumentVerified, "An organization user recorded a positive verification decision.")
	register(EventDocumentRejected, "An organization user recorded a negative verification decision with a controlled reason.")
	register(EventDocumentReplaced, "A document was superseded by a newer replacement document.")
	register(EventDocumentRevoked, "A document was revoked and can no longer satisfy a requirement.")
	register(EventDocumentExpired, "A document's stated expiry date has passed.")
	register(EventDocumentDeleted, "Media confirmed the underlying object was deleted.")
	register(EventDocumentReconciling, "A version gap or projection disagreement opened a reconciliation case.")
	register(EventDocumentArchived, "A document was organizationally archived out of default listings.")
}
