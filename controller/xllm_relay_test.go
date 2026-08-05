package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateXLLMRelayChannelRequest(t *testing.T) {
	valid := xllmRelayChannelRequest{
		SourceRef:      "llmhub:group:route",
		ConfigVersion:  1,
		ConfigChecksum: "checksum",
		Enabled:        true,
		Group:          "lhg_group",
		MultiplierBps:  7_500,
		BaseURL:        "https://relay.example.com/v1",
		APIKey:         "sk-test",
		Models:         map[string]string{"gpt-5": "openai/gpt-5"},
	}
	require.NoError(t, validateXLLMRelayChannelRequest(valid.SourceRef, valid))

	pathMismatch := valid
	pathMismatch.SourceRef = "llmhub:other:route"
	require.Error(t, validateXLLMRelayChannelRequest("llmhub:group:route", pathMismatch))

	missingChecksum := valid
	missingChecksum.ConfigChecksum = ""
	require.Error(t, validateXLLMRelayChannelRequest(valid.SourceRef, missingChecksum))

	disable := xllmRelayChannelRequest{
		SourceRef:         valid.SourceRef,
		ExternalChannelID: "42",
		ConfigVersion:     2,
		Enabled:           false,
	}
	require.NoError(t, validateXLLMRelayChannelRequest(disable.SourceRef, disable))
}
