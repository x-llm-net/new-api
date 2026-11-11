package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
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

func TestRewriteGeminiSystemInstructionsThaiOutput(t *testing.T) {
	const replacement = "本提示词是中文，你的全部输出（narrator、char 和 npc 的 text 正文）必须是泰语。"
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "original sentence",
			text: "本提示词的说明文字是中文，但你的全部输出（narrator、char、npc 的 text 正文）必须是泰语。",
			want: replacement,
		},
		{
			name: "conjunction variant embedded in system text",
			text: "before\n本提示词的说明文字是中文，但你的全部输出（narrator、char 和 npc 的 text 正文）必须是泰语。\nafter",
			want: "before\n" + replacement + "\nafter",
		},
		{
			name: "different target language is preserved",
			text: "本提示词的说明文字是中文，但你的全部输出（narrator、char、npc 的 text 正文）必须是英语。",
			want: "本提示词的说明文字是中文，但你的全部输出（narrator、char、npc 的 text 正文）必须是英语。",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.GeminiChatRequest{
				SystemInstructions: &dto.GeminiChatContent{
					Parts: []dto.GeminiPart{{Text: tt.text}, {Text: replacement}},
				},
				Contents: []dto.GeminiChatContent{
					{Role: "user", Parts: []dto.GeminiPart{{Text: tt.text}}},
				},
			}

			rewriteGeminiSystemInstructions(nil, request)

			require.Len(t, request.SystemInstructions.Parts, 2)
			assert.Equal(t, tt.want, request.SystemInstructions.Parts[0].Text)
			assert.Equal(t, replacement, request.SystemInstructions.Parts[1].Text)
			assert.Equal(t, tt.text, request.Contents[0].Parts[0].Text)
		})
	}
}

func TestRewriteGeminiSystemInstructionsNoOpWithoutSystemInstruction(t *testing.T) {
	request := &dto.GeminiChatRequest{}

	rewriteGeminiSystemInstructions(nil, request)

	assert.Nil(t, request.SystemInstructions)
}
