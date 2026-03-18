package tools

// noisyMetadataKeys lists metadata fields that are stripped to reduce token usage.
var noisyMetadataKeys = []string{
	"uid",
	"resourceVersion",
	"generation",
	"selfLink",
	"managedFields",
}

// StripNoisyMetadata returns a deep copy of obj with noisy metadata fields removed.
// It strips uid, resourceVersion, generation, selfLink, and managedFields from
// the metadata block. Empty string, nil, empty map, and empty slice values in
// metadata are also removed.
//
// The input map is never mutated.
func StripNoisyMetadata(obj map[string]interface{}) map[string]interface{} {
	result := shallowCopyMap(obj)

	rawMeta, ok := result["metadata"]
	if !ok {
		return result
	}
	metaMap, ok := rawMeta.(map[string]interface{})
	if !ok {
		return result
	}

	cleanedMeta := shallowCopyMap(metaMap)

	// Remove well-known noisy keys.
	for _, key := range noisyMetadataKeys {
		delete(cleanedMeta, key)
	}

	// Remove empty values (empty string, nil, empty map, empty slice).
	for k, v := range cleanedMeta {
		if isEmpty(v) {
			delete(cleanedMeta, k)
		}
	}

	result["metadata"] = cleanedMeta
	return result
}

// shallowCopyMap returns a new map with the same key-value pairs.
func shallowCopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// isEmpty returns true for nil, empty string, empty map, or empty slice values.
func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case map[string]interface{}:
		return len(val) == 0
	case []interface{}:
		return len(val) == 0
	default:
		return false
	}
}
