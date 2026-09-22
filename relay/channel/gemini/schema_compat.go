package gemini

// removeEmptyEnumValues removes an empty string from enum values when at least
// one non-empty value remains. Gemini rejects empty strings in enum definitions,
// while some OpenAI-compatible clients use one as a placeholder for an optional
// field. An enum containing only an empty string is left unchanged so the
// request is not silently broadened.
func removeEmptyEnumValues(schema interface{}) interface{} {
	switch value := schema.(type) {
	case map[string]interface{}:
		for key, nested := range value {
			if key == "enum" {
				if enum, ok := nested.([]interface{}); ok {
					filtered := make([]interface{}, 0, len(enum))
					for _, item := range enum {
						if text, ok := item.(string); ok && text == "" {
							continue
						}
						filtered = append(filtered, item)
					}
					if len(filtered) > 0 && len(filtered) != len(enum) {
						value[key] = filtered
					}
					continue
				}
			}
			value[key] = removeEmptyEnumValues(nested)
		}
		return value
	case []interface{}:
		for i, nested := range value {
			value[i] = removeEmptyEnumValues(nested)
		}
		return value
	default:
		return schema
	}
}
