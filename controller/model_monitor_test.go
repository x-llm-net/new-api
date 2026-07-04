package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBuildModelMonitorSummaryUsesScheduledChannelProbeData(t *testing.T) {
	now := int64(2_000)
	channels := []*model.Channel{
		{
			Id:           1,
			Status:       common.ChannelStatusEnabled,
			Models:       "gpt-4o,claude-3-5-sonnet",
			TestTime:     now - 30,
			ResponseTime: 2400,
		},
		{
			Id:           2,
			Status:       common.ChannelStatusAutoDisabled,
			Models:       "gpt-4o",
			TestTime:     now - 40,
			ResponseTime: 0,
		},
		{
			Id:           3,
			Status:       common.ChannelStatusManuallyDisabled,
			Models:       "gpt-4o",
			TestTime:     now - 10,
			ResponseTime: 50,
		},
		{
			Id:           4,
			Status:       common.ChannelStatusEnabled,
			Models:       "deepseek-chat",
			TestTime:     now - 1_000,
			ResponseTime: 1800,
		},
	}

	summary := buildModelMonitorSummary(now, 300, channels, nil, nil)

	require.Equal(t, 3, summary.TotalModels)
	require.Equal(t, 1, summary.OperationalModels)
	require.Equal(t, 1, summary.DegradedModels)
	require.Equal(t, 1, summary.UnknownModels)
	require.Equal(t, 0, summary.UnavailableModels)
	require.Equal(t, 3, summary.TotalRoutes)
	require.Equal(t, 1, summary.AvailableRoutes)

	modelsByName := map[string]publicModelMonitorModel{}
	for _, item := range summary.Models {
		modelsByName[item.ModelName] = item
	}

	gpt := modelsByName["gpt-4o"]
	require.Equal(t, modelMonitorStatusDegraded, gpt.Status)
	require.Equal(t, 1, gpt.AvailableRoutes)
	require.Equal(t, 2, gpt.TotalRoutes)
	require.Equal(t, 1, gpt.UnavailableRoutes)
	require.Equal(t, 2400, gpt.MinLatencyMs)

	claude := modelsByName["claude-3-5-sonnet"]
	require.Equal(t, modelMonitorStatusOperational, claude.Status)
	require.Equal(t, 1, claude.AvailableRoutes)
	require.Equal(t, 1, claude.TotalRoutes)

	deepseek := modelsByName["deepseek-chat"]
	require.Equal(t, modelMonitorStatusUnknown, deepseek.Status)
	require.Equal(t, 0, deepseek.AvailableRoutes)
	require.Equal(t, 1, deepseek.UnknownRoutes)

	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.Equal(t, "default", group.GroupName)
	require.Equal(t, modelMonitorStatusDegraded, group.Status)
	require.Equal(t, 3, group.TotalRoutes)
	require.Equal(t, 1, group.AvailableRoutes)
	require.Equal(t, 1, group.UnavailableRoutes)
	require.Equal(t, 1, group.UnknownRoutes)
	require.Equal(t, 2400, group.MinLatencyMs)
	require.Len(t, group.Routes, 3)
}
