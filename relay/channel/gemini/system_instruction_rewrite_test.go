package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteGeminiSystemInstructions(t *testing.T) {
	request := &dto.GeminiChatRequest{
		SystemInstructions: &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{
				{Text: "deep literary foundation"},
				{Text: "keep deep literary foundation here too"},
				{Text: "DEEP LITERARY FOUNDATION"},
			},
		},
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: "deep literary foundation"}}},
		},
	}

	rewriteGeminiSystemInstructions(nil, request)

	require.Len(t, request.SystemInstructions.Parts, 3)
	assert.Equal(t, "profound literary background", request.SystemInstructions.Parts[0].Text)
	assert.Equal(t, "keep profound literary background here too", request.SystemInstructions.Parts[1].Text)
	assert.Equal(t, "DEEP LITERARY FOUNDATION", request.SystemInstructions.Parts[2].Text)
	assert.Equal(t, "deep literary foundation", request.Contents[0].Parts[0].Text)
}

func TestRewriteGeminiSystemInstructionsNoOpWithoutMatch(t *testing.T) {
	request := &dto.GeminiChatRequest{
		SystemInstructions: &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: "ordinary system instruction"}},
		},
	}

	rewriteGeminiSystemInstructions(nil, request)

	assert.Equal(t, "ordinary system instruction", request.SystemInstructions.Parts[0].Text)
}

func TestRewriteGeminiSystemInstructionsNoOpWithoutSystemInstruction(t *testing.T) {
	request := &dto.GeminiChatRequest{}

	rewriteGeminiSystemInstructions(nil, request)

	assert.Nil(t, request.SystemInstructions)
}
