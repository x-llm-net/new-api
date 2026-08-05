package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertXLLMRelayChannelIsVersionedAndReversible(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&XLLMChannelBinding{}, &Option{}))
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	group := "lhg_model_test"
	sourceRef := "llmhub:model-test:route-a"
	t.Cleanup(func() {
		DB.Where("source_ref = ?", sourceRef).Delete(&XLLMChannelBinding{})
	})

	input := XLLMRelayChannelInput{
		SourceRef:      sourceRef,
		ConfigVersion:  1,
		ConfigChecksum: "checksum-1",
		Enabled:        true,
		Group:          group,
		MultiplierBps:  7_500,
		BaseURL:        "https://relay.example.com/v1",
		APIKey:         "sk-test",
		Models:         map[string]string{"gpt-5": "openai/gpt-5"},
	}
	created, err := UpsertXLLMRelayChannel(input)
	require.NoError(t, err)
	assert.True(t, created.Applied)
	assert.Positive(t, created.ChannelID)
	assert.Equal(t, 0.75, ratio_setting.GetGroupRatio(group))
	active, err := IsActiveXLLMRelayChannel(created.ChannelID)
	require.NoError(t, err)
	assert.True(t, active)

	repeated, err := UpsertXLLMRelayChannel(input)
	require.NoError(t, err)
	assert.False(t, repeated.Applied)
	assert.Equal(t, created.ChannelID, repeated.ChannelID)

	conflict := input
	conflict.ConfigChecksum = "different-checksum"
	_, err = UpsertXLLMRelayChannel(conflict)
	require.ErrorIs(t, err, ErrXLLMConfigVersionConflict)

	updated := input
	updated.ConfigVersion = 2
	updated.ConfigChecksum = "checksum-2"
	updated.BaseURL = "https://relay-2.example.com/v1"
	updated.Models = map[string]string{
		"gpt-5":      "openai/gpt-5",
		"gpt-5-mini": "openai/gpt-5-mini",
	}
	result, err := UpsertXLLMRelayChannel(updated)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, created.ChannelID, result.ChannelID)

	stale := input
	stale.ConfigChecksum = "checksum-stale"
	stale.ConfigVersion = 1
	result, err = UpsertXLLMRelayChannel(stale)
	require.NoError(t, err)
	assert.False(t, result.Applied)
	assert.Equal(t, 2, result.ConfigVersion)

	disabled, err := UpsertXLLMRelayChannel(XLLMRelayChannelInput{
		SourceRef:         sourceRef,
		ExternalChannelID: strconv.Itoa(created.ChannelID),
		ConfigVersion:     3,
		Enabled:           false,
	})
	require.NoError(t, err)
	assert.True(t, disabled.Applied)

	channel, err := GetChannelById(created.ChannelID, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	assert.Equal(t, "gpt-5,gpt-5-mini", channel.Models)
	assert.Equal(t, "https://relay-2.example.com", channel.GetBaseURL())

	var binding XLLMChannelBinding
	require.NoError(t, DB.Where("source_ref = ?", sourceRef).First(&binding).Error)
	assert.False(t, binding.Active)
	assert.Equal(t, 3, binding.ConfigVersion)
	active, err = IsActiveXLLMRelayChannel(created.ChannelID)
	require.NoError(t, err)
	assert.False(t, active)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", created.ChannelID).Find(&abilities).Error)
	require.Len(t, abilities, 2)
	for _, ability := range abilities {
		assert.False(t, ability.Enabled)
	}

	var channelCount int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", created.ChannelID).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}
