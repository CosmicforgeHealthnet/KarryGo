//go:build ignore

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	_ "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
)

type M = map[string]any

func main() {
	messages := M{}
	refs := []any{}
	for _, def := range events.Registered() {
		name := messageName(def.Type)
		messages[name] = M{"name": name, "title": def.Type, "summary": def.Description, "contentType": "application/json", "payload": M{"$ref": "#/components/schemas/Envelope"}, "x-event-type": def.Type, "x-aggregate-type": def.AggregateType, "x-schema-version": def.SchemaVersion, "x-direction": def.Direction, "x-producers": def.Producers, "x-max-payload-bytes": def.MaxPayloadBytes, "x-tenant-required": def.TenantRequired, "x-forbidden-fields": def.ForbiddenFields}
		refs = append(refs, M{"$ref": "#/components/messages/" + name})
	}
	spec := M{"asyncapi": "2.6.0", "info": M{"title": "KarryGo Fleet Partner Section 26 Event Catalogue", "version": "26.1.0", "description": "Machine-readable produced and consumed event contracts with exact producer authorization, typed minimized payloads, and Redis consumer-group delivery."}, "channels": M{events.DefaultStreamKey: M{"x-stream-key": events.DefaultStreamKey, "x-consumer-group": events.DefaultConsumerGroup, "subscribe": M{"message": M{"oneOf": refs}}}}, "components": M{"messages": messages, "schemas": M{"Envelope": envelopeSchema()}}}
	raw, _ := json.MarshalIndent(spec, "", "  ")
	raw = append(raw, '\n')
	_, file, _, _ := runtime.Caller(0)
	if err := os.WriteFile(filepath.Join(filepath.Dir(file), "section26-events.asyncapi.json"), raw, 0o644); err != nil {
		panic(err)
	}
}
func messageName(eventType string) string {
	parts := strings.FieldsFunc(eventType, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}
func envelopeSchema() M {
	return M{"type": "object", "additionalProperties": false, "properties": M{"eventId": M{"type": "string"}, "eventType": M{"type": "string"}, "schemaVersion": M{"type": "integer"}, "aggregateType": M{"type": "string"}, "aggregateId": M{"type": "string"}, "aggregateVersion": M{"type": "integer"}, "organizationId": M{"type": "string"}, "correlationId": M{"type": "string"}, "occurredAt": M{"type": "string", "format": "date-time"}, "producer": M{"type": "string"}, "payload": M{"type": "object"}}, "required": []string{"eventId", "eventType", "schemaVersion", "aggregateType", "aggregateId", "aggregateVersion", "correlationId", "occurredAt", "producer", "payload"}}
}
