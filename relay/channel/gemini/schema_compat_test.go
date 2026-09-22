package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeGeminiRawSchema(t *testing.T) {
	raw := json.RawMessage(`[{"functionDeclarations":[{"parameters":{"type":"object","properties":{"speakerKey":{"type":"string","enum":["main",""]}}}}]}]`)

	got := normalizeGeminiRawSchema(raw)
	assert.JSONEq(t, `[{"functionDeclarations":[{"parameters":{"type":"object","properties":{"speakerKey":{"type":"string","enum":["main"]}}}}]}]`, string(got))
}

func TestNormalizeGeminiRawSchemaLeavesInvalidJSONUnchanged(t *testing.T) {
	raw := json.RawMessage(`{"enum":["main",}`)

	assert.Equal(t, raw, normalizeGeminiRawSchema(raw))
}

func TestRemoveEmptyEnumValues(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"speakerKey": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{"main", "npc_1", ""},
			},
		},
	}

	got := removeEmptyEnumValues(schema)

	assert.Equal(t, []interface{}{"main", "npc_1"}, got.(map[string]interface{})["properties"].(map[string]interface{})["speakerKey"].(map[string]interface{})["enum"])
}

func TestRemoveEmptyEnumValuesLeavesOnlyEmptyEnumUnchanged(t *testing.T) {
	schema := map[string]interface{}{
		"enum": []interface{}{""},
	}

	got := removeEmptyEnumValues(schema)

	assert.Equal(t, []interface{}{""}, got.(map[string]interface{})["enum"])
}

func TestRemoveEmptyEnumValuesDoesNotChangeOtherValues(t *testing.T) {
	schema := map[string]interface{}{
		"enum":   []interface{}{"main", "npc_1"},
		"nested": []interface{}{map[string]interface{}{"type": "string"}},
	}

	got := removeEmptyEnumValues(schema)

	assert.Equal(t, schema, got)
}

func TestRemoveEmptyEnumValuesWithoutEnumIsSafe(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{"type": "string"},
		},
		"nullable": nil,
	}

	assert.Equal(t, schema, removeEmptyEnumValues(schema))
}

func TestRemoveEmptyEnumValuesAcceptsNilSchema(t *testing.T) {
	assert.Nil(t, removeEmptyEnumValues(nil))
}
