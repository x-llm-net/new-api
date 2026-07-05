package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBuildXLLMGroupTaskPointsMarksGroupAvailableWhenAnyChannelSucceeds(t *testing.T) {
	samples := []model.XLLMGroupStabilitySample{
		{ID: 1, TaskID: "task-1", GroupNames: "codex-pro", Success: false, TestedAt: 100},
		{ID: 2, TaskID: "task-1", GroupNames: "codex-pro", Success: true, TestedAt: 101},
		{ID: 3, TaskID: "task-2", GroupNames: "codex-pro", Success: false, TestedAt: 200},
	}

	groupTasks, orderedTasks := buildXLLMGroupTaskPoints(samples, []XLLMGroupStabilityGroupConfig{
		{Group: "codex-pro", DisplayName: "Codex Pro"},
	})
	buckets := buildXLLMRecentBuckets(groupTasks["codex-pro"], orderedTasks)

	require.Len(t, buckets, xllmGroupStabilityRecentRuns)
	require.Equal(t, "available", buckets[len(buckets)-2].Status)
	require.Equal(t, "unavailable", buckets[len(buckets)-1].Status)
	require.Equal(t, float64(50), xllmBucketSuccessRate(buckets))
}

func TestBuildXLLMRecentBucketsUsesUnknownWhenGroupHasNoTaskSamples(t *testing.T) {
	samples := []model.XLLMGroupStabilitySample{
		{ID: 1, TaskID: "task-1", GroupNames: "plus", Success: true, TestedAt: 100},
	}

	groupTasks, orderedTasks := buildXLLMGroupTaskPoints(samples, []XLLMGroupStabilityGroupConfig{
		{Group: "codex-pro", DisplayName: "Codex Pro"},
	})
	buckets := buildXLLMRecentBuckets(groupTasks["codex-pro"], orderedTasks)

	require.Len(t, buckets, xllmGroupStabilityRecentRuns)
	require.Equal(t, "unknown", buckets[len(buckets)-1].Status)
	require.Zero(t, xllmBucketSuccessRate(buckets))
}

func TestBuildXLLMSevenDayBucketsAggregatesPerTaskAvailability(t *testing.T) {
	now := time.Unix(7200+1800, 0)
	points := map[string]xllmGroupTaskPoint{
		"task-1": {taskID: "task-1", testedAt: 3600 + 100, hasSample: true, available: true},
		"task-2": {taskID: "task-2", testedAt: 3600 + 200, hasSample: true, available: false},
		"task-3": {taskID: "task-3", testedAt: 7200 + 100, hasSample: true, available: true},
	}

	endHour := now.Unix() - (now.Unix() % 3600)
	buckets := buildXLLMSevenDayBuckets(points, endHour)

	require.Len(t, buckets, xllmGroupStabilitySevenDayHours)
	require.Equal(t, "degraded", buckets[len(buckets)-2].Status)
	require.Equal(t, float64(50), buckets[len(buckets)-2].SuccessRate)
	require.Equal(t, 1, buckets[len(buckets)-2].AvailableRuns)
	require.Equal(t, 2, buckets[len(buckets)-2].TotalRuns)
	require.Equal(t, "available", buckets[len(buckets)-1].Status)
	require.Equal(t, float64(100), buckets[len(buckets)-1].SuccessRate)
}

func TestXLLMSevenDayEndHourUsesLatestCompletedSample(t *testing.T) {
	now := time.Unix(7200+1800, 0)
	points := map[string]xllmGroupTaskPoint{
		"task-1": {taskID: "task-1", testedAt: 3600 + 100, hasSample: true},
	}

	require.Equal(t, int64(3600), xllmSevenDayEndHour(points, now))
}

func TestXLLMSevenDayEndHourUsesCurrentHourWhenGroupHasCurrentSample(t *testing.T) {
	now := time.Unix(7200+1800, 0)
	points := map[string]xllmGroupTaskPoint{
		"task-1": {taskID: "task-1", testedAt: 7200 + 100, hasSample: true},
	}

	require.Equal(t, int64(7200), xllmSevenDayEndHour(points, now))
}

func TestXLLMHourlyBucketStatusOnlyTurnsRedBelowHalf(t *testing.T) {
	require.Equal(t, "available", xllmHourlyBucketStatus(100))
	require.Equal(t, "degraded", xllmHourlyBucketStatus(99.99))
	require.Equal(t, "degraded", xllmHourlyBucketStatus(50))
	require.Equal(t, "unavailable", xllmHourlyBucketStatus(49.99))
}

func TestLoadXLLMGroupStabilityConfigAcceptsUTF8BOM(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "xllm-group-stability.json")
	content := []byte("\xef\xbb\xbf" + `{"enabled":true,"groups":[{"group":"codex-pro","display_name":"Codex Pro","sort_order":10}]}`)
	require.NoError(t, os.WriteFile(configPath, content, 0o600))

	t.Setenv("XLLM_GROUP_STABILITY_CONFIG", configPath)
	xllmGroupStabilityConfigCache.Lock()
	xllmGroupStabilityConfigCache.loaded = false
	xllmGroupStabilityConfigCache.Unlock()

	config, err := LoadXLLMGroupStabilityConfig()

	require.NoError(t, err)
	require.True(t, xllmGroupStabilityConfigEnabled(config))
	require.Len(t, config.Groups, 1)
	require.Equal(t, "codex-pro", config.Groups[0].Group)
	require.Equal(t, "Codex Pro", config.Groups[0].DisplayName)
}
