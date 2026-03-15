package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newUnstructured(kind string, fields map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": "test", "namespace": "default"},
	}}
	for k, v := range fields {
		obj.Object[k] = v
	}
	return obj
}

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name  string
		obj   *unstructured.Unstructured
		check func(t *testing.T, obj *unstructured.Unstructured)
	}{
		{
			name: "secret with data and stringData gets redacted",
			obj: newUnstructured("Secret", map[string]interface{}{
				"data":       map[string]interface{}{"token": "c2VjcmV0", "key": "dmFsdWU="},
				"stringData": map[string]interface{}{"password": "hunter2"},
			}),
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				for _, field := range []string{"data", "stringData"} {
					m, ok := obj.Object[field].(map[string]interface{})
					if !ok {
						t.Fatalf("expected %s to be a map", field)
					}
					for k, v := range m {
						if v != redactedValue {
							t.Errorf("%s[%s] = %v, want %s", field, k, v, redactedValue)
						}
					}
				}
			},
		},
		{
			name: "non-Secret is untouched",
			obj: newUnstructured("ConfigMap", map[string]interface{}{
				"data": map[string]interface{}{"key": "value"},
			}),
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				m := obj.Object["data"].(map[string]interface{})
				if m["key"] != "value" {
					t.Errorf("data[key] = %v, want value", m["key"])
				}
			},
		},
		{
			name: "secret without data or stringData fields",
			obj:  newUnstructured("Secret", nil),
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				if _, ok := obj.Object["data"]; ok {
					t.Error("expected no data field")
				}
				if _, ok := obj.Object["stringData"]; ok {
					t.Error("expected no stringData field")
				}
			},
		},
		{
			name: "secret with non-map data field",
			obj: newUnstructured("Secret", map[string]interface{}{
				"data": "not-a-map",
			}),
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				if obj.Object["data"] != "not-a-map" {
					t.Errorf("data = %v, want not-a-map", obj.Object["data"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RedactSecrets(tt.obj)
			tt.check(t, tt.obj)
		})
	}
}

func TestRedactSecretsList(t *testing.T) {
	secret := newUnstructured("Secret", map[string]interface{}{
		"data": map[string]interface{}{"token": "c2VjcmV0"},
	})
	configMap := newUnstructured("ConfigMap", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	secret2 := newUnstructured("Secret", map[string]interface{}{
		"stringData": map[string]interface{}{"pass": "s3cret"},
	})

	list := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{*secret, *configMap, *secret2},
	}

	RedactSecretsList(list)

	// First item: Secret data redacted
	m := list.Items[0].Object["data"].(map[string]interface{})
	if m["token"] != redactedValue {
		t.Errorf("Items[0] data[token] = %v, want %s", m["token"], redactedValue)
	}

	// Second item: ConfigMap untouched
	m = list.Items[1].Object["data"].(map[string]interface{})
	if m["key"] != "value" {
		t.Errorf("Items[1] data[key] = %v, want value", m["key"])
	}

	// Third item: Secret stringData redacted
	m = list.Items[2].Object["stringData"].(map[string]interface{})
	if m["pass"] != redactedValue {
		t.Errorf("Items[2] stringData[pass] = %v, want %s", m["pass"], redactedValue)
	}
}

func TestRedactMapValues(t *testing.T) {
	tests := []struct {
		name  string
		obj   *unstructured.Unstructured
		field string
		check func(t *testing.T, obj *unstructured.Unstructured)
	}{
		{
			name: "map present is redacted",
			obj: newUnstructured("Secret", map[string]interface{}{
				"data": map[string]interface{}{"a": "1", "b": "2"},
			}),
			field: "data",
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				m := obj.Object["data"].(map[string]interface{})
				for k, v := range m {
					if v != redactedValue {
						t.Errorf("data[%s] = %v, want %s", k, v, redactedValue)
					}
				}
			},
		},
		{
			name:  "field absent is no-op",
			obj:   newUnstructured("Secret", nil),
			field: "data",
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				if _, ok := obj.Object["data"]; ok {
					t.Error("expected no data field")
				}
			},
		},
		{
			name: "non-map value is not modified",
			obj: newUnstructured("Secret", map[string]interface{}{
				"data": "a-string",
			}),
			field: "data",
			check: func(t *testing.T, obj *unstructured.Unstructured) {
				t.Helper()
				if obj.Object["data"] != "a-string" {
					t.Errorf("data = %v, want a-string", obj.Object["data"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redactMapValues(tt.obj, tt.field)
			tt.check(t, tt.obj)
		})
	}
}
