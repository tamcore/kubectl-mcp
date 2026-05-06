package tools

import "encoding/json"

// rawK8sObjectSchema is a permissive JSON schema for a Kubernetes object.
// Used by tools that return dynamic objects whose exact shape varies by kind.
var rawK8sObjectSchema = json.RawMessage(`{"type":"object","properties":{"apiVersion":{"type":"string"},"kind":{"type":"string"},"metadata":{"type":"object"},"spec":{"type":"object"},"status":{"type":"object"}}}`)

// listEnvelopeSchema describes the structured envelope returned by list_resources.
type listEnvelopeSchema struct {
	Items    []any  `json:"items"`
	Count    int    `json:"count"`
	Continue string `json:"continue,omitempty"`
}
