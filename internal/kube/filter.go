package kube

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const redactedValue = "<redacted>"

// RedactSecrets replaces .data and .stringData values in Secret objects
// with a placeholder. It modifies the object in place.
func RedactSecrets(obj *unstructured.Unstructured) {
	if obj.GetKind() != "Secret" {
		return
	}
	redactMapValues(obj, "data")
	redactMapValues(obj, "stringData")
}

// RedactSecretsList redacts all Secret objects in a list.
func RedactSecretsList(list *unstructured.UnstructuredList) {
	for i := range list.Items {
		RedactSecrets(&list.Items[i])
	}
}

func redactMapValues(obj *unstructured.Unstructured, field string) {
	raw, ok := obj.Object[field]
	if !ok {
		return
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	for k := range m {
		m[k] = redactedValue
	}
	obj.Object[field] = m
}
