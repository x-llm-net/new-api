package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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

func TestSystemInstructionRewriteReload(t *testing.T) {
	previous := activeSystemInstructionRewriteRules.Load()
	activeSystemInstructionRewriteRules.Store(nil)
	t.Cleanup(func() { activeSystemInstructionRewriteRules.Store(previous) })
	loader := systemInstructionRewriteLoader{path: filepath.Join(t.TempDir(), "rules.json")}
	require.NoError(t, loader.reload())

	const original = "alpha phrase deep literary foundation"
	for _, step := range []struct {
		name    string
		content string
		want    string
		remove  bool
		wantErr bool
	}{
		{name: "built-in defaults", remove: true, want: "alpha phrase profound literary background"},
		{name: "replace full table", content: `{"enabled":true,"rules":[{"from":"alpha phrase","to":"beta phrase"}]}`, want: "beta phrase deep literary foundation"},
		{name: "unchanged file", content: `{"enabled":true,"rules":[{"from":"alpha phrase","to":"beta phrase"}]}`, want: "beta phrase deep literary foundation"},
		{name: "invalid JSON retains old rules", content: `{"enabled":`, wantErr: true, want: "beta phrase deep literary foundation"},
		{name: "cached invalid JSON remains an error", content: `{"enabled":`, wantErr: true, want: "beta phrase deep literary foundation"},
		{name: "invalid rule retains old rules", content: `{"enabled":true,"rules":[{"from":"","to":"beta phrase"}]}`, wantErr: true, want: "beta phrase deep literary foundation"},
		{name: "missing file retains old rules", remove: true, wantErr: true, want: "beta phrase deep literary foundation"},
		{name: "oversized file retains old rules", content: strings.Repeat(" ", maxSystemRewriteConfigBytes+1), wantErr: true, want: "beta phrase deep literary foundation"},
		{name: "repair and update", content: `{"enabled":true,"rules":[{"from":"alpha phrase","to":"gamma phrase"}]}`, want: "gamma phrase deep literary foundation"},
		{name: "disable", content: `{"enabled":false}`, want: original},
		{name: "removal does not re-enable defaults", remove: true, wantErr: true, want: original},
		{name: "empty table", content: `{"enabled":true,"rules":[]}`, want: original},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.remove {
				err := os.Remove(loader.path)
				if !os.IsNotExist(err) {
					require.NoError(t, err)
				}
			} else {
				require.NoError(t, os.WriteFile(loader.path+".tmp", []byte(step.content), 0o600))
				require.NoError(t, os.Rename(loader.path+".tmp", loader.path))
			}
			err := loader.reload()
			if step.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			request := &dto.GeminiChatRequest{
				SystemInstructions: &dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: original}}},
				Contents: []dto.GeminiChatContent{
					{Role: "user", Parts: []dto.GeminiPart{{Text: original}}},
				},
			}
			rewriteGeminiSystemInstructions(nil, request)
			assert.Equal(t, step.want, request.SystemInstructions.Parts[0].Text)
			assert.Equal(t, original, request.Contents[0].Parts[0].Text)
		})
	}
}

func TestSystemInstructionRewriteConfigValidation(t *testing.T) {
	tooMany, err := common.Marshal(struct {
		Enabled bool                           `json:"enabled"`
		Rules   []systemInstructionRewriteRule `json:"rules"`
	}{Enabled: true, Rules: make([]systemInstructionRewriteRule, maxSystemRewriteRules+1)})
	require.NoError(t, err)
	for name, data := range map[string]string{
		"missing enabled":   `{"rules":[]}`,
		"missing rules":     `{"enabled":true}`,
		"null rules":        `{"enabled":true,"rules":null}`,
		"empty source":      `{"enabled":true,"rules":[{"from":"","to":"replacement"}]}`,
		"empty replacement": `{"enabled":true,"rules":[{"from":"source","to":" "}]}`,
		"duplicate source":  `{"enabled":true,"rules":[{"from":"source","to":"first"},{"from":"source","to":"second"}]}`,
		"long phrase":       `{"enabled":true,"rules":[{"from":"` + strings.Repeat("a", maxSystemRewritePhraseBytes+1) + `","to":"replacement"}]}`,
		"too many rules":    string(tooMany),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSystemInstructionRewriteRules([]byte(data))
			assert.Error(t, err)
		})
	}
}

func TestSystemInstructionRewriteConcurrentReload(t *testing.T) {
	previous := activeSystemInstructionRewriteRules.Load()
	t.Cleanup(func() { activeSystemInstructionRewriteRules.Store(previous) })
	oldRules := []systemInstructionRewriteRule{{From: "source phrase", To: "first wording"}}
	activeSystemInstructionRewriteRules.Store(&oldRules)
	loader := systemInstructionRewriteLoader{path: filepath.Join(t.TempDir(), "rules.json")}
	require.NoError(t, os.WriteFile(loader.path, []byte(`{"enabled":true,"rules":[{"from":"source phrase","to":"second wording"}]}`), 0o600))
	start := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		<-start
		done <- loader.reload()
	}()
	request := &dto.GeminiChatRequest{
		SystemInstructions: &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: "source phrase"}, {Text: "source phrase"}},
		},
	}
	close(start)
	rewriteGeminiSystemInstructions(nil, request)
	require.NoError(t, <-done)
	first := request.SystemInstructions.Parts[0].Text
	assert.Contains(t, []string{"first wording", "second wording"}, first)
	assert.Equal(t, first, request.SystemInstructions.Parts[1].Text)
	assert.Equal(t, "first wording", oldRules[0].To)
	request.SystemInstructions.Parts[0].Text = "source phrase"
	rewriteGeminiSystemInstructions(nil, request)
	assert.Equal(t, "second wording", request.SystemInstructions.Parts[0].Text)
}

func BenchmarkRewriteGeminiSystemInstructions(b *testing.B) {
	previous := activeSystemInstructionRewriteRules.Load()
	rules := geminiSystemInstructionRewriteRules
	activeSystemInstructionRewriteRules.Store(&rules)
	b.Cleanup(func() { activeSystemInstructionRewriteRules.Store(previous) })
	prefix := strings.Repeat("Follow the requested language and output format.\n", 5500)
	for _, scenario := range []struct {
		name string
		text string
	}{
		{name: "large_no_match", text: prefix},
		{name: "large_match", text: prefix + "deep literary foundation"},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			request := &dto.GeminiChatRequest{
				SystemInstructions: &dto.GeminiChatContent{Parts: []dto.GeminiPart{{}}},
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(scenario.text)))
			for b.Loop() {
				request.SystemInstructions.Parts[0].Text = scenario.text
				rewriteGeminiSystemInstructions(nil, request)
			}
		})
	}
}
