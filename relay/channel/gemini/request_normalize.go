package gemini

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

func normalizeGeminiRequest(c *gin.Context, request *dto.GeminiChatRequest) {
	if request == nil {
		return
	}

	rewriteGeminiSystemInstructions(c, request)
	request.GenerationConfig.ResponseSchema = removeEmptyEnumValues(request.GenerationConfig.ResponseSchema)
	request.GenerationConfig.ResponseJsonSchema = normalizeGeminiRawSchema(request.GenerationConfig.ResponseJsonSchema)
	request.Tools = normalizeGeminiRawSchema(request.Tools)
}

func normalizeGeminiRawSchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}

	value = removeEmptyEnumValues(value)
	normalized, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return normalized
}
