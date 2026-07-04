package controller

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXLLMHomeMonitorSummaryUsesProbeRollups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.XLLMProbeSample{}, &model.XLLMProbeRollup{}))

	t.Setenv("XLLM_HOME_MONITOR_CONFIG", writeXLLMHomeMonitorTestConfig(t))

	start := time.Now().Add(-1500 * time.Millisecond)
	info := &relaycommon.RelayInfo{
		StartTime:         start,
		FirstResponseTime: start.Add(1200 * time.Millisecond),
		IsStream:          true,
	}
	err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
		TaskID:            "task_test",
		SelectedChannelID: 1001,
		Entry: service.XLLMHomeMonitorEntry{
			GroupName:   "codex-pro",
			ModelName:   "gpt-5.4-mini",
			DisplayName: "Codex Pro",
			Enabled:     true,
		},
		Info:        info,
		StreamProbe: true,
		Success:     true,
		TotalMs:     1500,
		TestedAt:    time.Now().Unix(),
	})
	require.NoError(t, err)

	summary, err := service.GetXLLMHomeMonitorSummary()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(summary.Items), 1)

	item := summary.Items[0]
	assert.Equal(t, "codex-pro", item.GroupName)
	assert.Equal(t, "gpt-5.4-mini", item.ModelName)
	assert.Equal(t, "Codex Pro", item.DisplayName)
	assert.Equal(t, "operational", item.Status)
	assert.EqualValues(t, 1200, item.LatestTTFTMs)
	assert.EqualValues(t, 1200, item.AvgTTFTMs60m)
	assert.EqualValues(t, 1200, item.P50TTFTMs60m)
	assert.EqualValues(t, 5*60*60, summary.RecentWindowSecs)
	assert.Len(t, item.RecentBuckets, 60)
	assert.Len(t, item.SevenDayBuckets, 168)
	assert.True(t, item.HasReliableTTFT)

	filledRecentBuckets := 0
	for _, bucket := range item.RecentBuckets {
		if bucket.SampleCount > 0 {
			filledRecentBuckets++
			assert.EqualValues(t, 1200, bucket.AvgTTFTMs)
			assert.EqualValues(t, 1200, bucket.P50TTFTMs)
		}
	}
	assert.Equal(t, 1, filledRecentBuckets)
}

func TestXLLMHomeMonitorSummaryUsesMedianTTFTForHourlyBuckets(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.XLLMProbeSample{}, &model.XLLMProbeRollup{}))

	t.Setenv("XLLM_HOME_MONITOR_CONFIG", writeXLLMHomeMonitorTestConfig(t))

	entry := service.XLLMHomeMonitorEntry{
		GroupName:   "codex-pro",
		ModelName:   "gpt-5.4-mini",
		DisplayName: "Codex Pro",
		Enabled:     true,
	}
	bucketStart := time.Now().Truncate(time.Hour).Add(-time.Hour)
	ttftValues := []int64{5_430, 2_248, 64_854, 1_693, 4_813}
	for index, ttftMs := range ttftValues {
		start := bucketStart.Add(time.Duration(index*10) * time.Minute)
		info := &relaycommon.RelayInfo{
			StartTime:         start,
			FirstResponseTime: start.Add(time.Duration(ttftMs) * time.Millisecond),
			IsStream:          true,
		}
		err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
			TaskID:            "task_hourly_median",
			SelectedChannelID: 1001 + index,
			Entry:             entry,
			Info:              info,
			StreamProbe:       true,
			Success:           true,
			TotalMs:           ttftMs + 100,
			TestedAt:          start.Unix(),
		})
		require.NoError(t, err)
	}

	summary, err := service.GetXLLMHomeMonitorSummary()
	require.NoError(t, err)
	require.NotEmpty(t, summary.Items)

	item := summary.Items[0]
	var hourlyBucket *service.XLLMHomeMonitorBucket
	for index := range item.SevenDayBuckets {
		if item.SevenDayBuckets[index].Ts == bucketStart.Unix() {
			hourlyBucket = &item.SevenDayBuckets[index]
			break
		}
	}
	require.NotNil(t, hourlyBucket)
	assert.EqualValues(t, 5, hourlyBucket.SampleCount)
	assert.EqualValues(t, 15_807, hourlyBucket.AvgTTFTMs)
	assert.EqualValues(t, 4_813, hourlyBucket.P50TTFTMs)
}

func TestXLLMHomeMonitorSummaryKeysDataByGroupAndModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.XLLMProbeSample{}, &model.XLLMProbeRollup{}))

	t.Setenv("XLLM_HOME_MONITOR_CONFIG", writeXLLMHomeMonitorTestConfig(t))

	now := time.Now()
	for _, sample := range []struct {
		groupName string
		modelName string
		ttftMs    int64
	}{
		{groupName: "codex-pro", modelName: "gpt-5.4-mini", ttftMs: 1200},
		{groupName: "codex-pro", modelName: "other-model", ttftMs: 9000},
		{groupName: "other-group", modelName: "gpt-5.4-mini", ttftMs: 7000},
	} {
		start := now.Add(-time.Duration(sample.ttftMs) * time.Millisecond)
		info := &relaycommon.RelayInfo{
			StartTime:         start,
			FirstResponseTime: start.Add(time.Duration(sample.ttftMs) * time.Millisecond),
			IsStream:          true,
		}
		err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
			TaskID:            "task_group_model_key",
			SelectedChannelID: 1001,
			Entry: service.XLLMHomeMonitorEntry{
				GroupName: sample.groupName,
				ModelName: sample.modelName,
				Enabled:   true,
			},
			Info:        info,
			StreamProbe: true,
			Success:     true,
			TotalMs:     sample.ttftMs + 100,
			TestedAt:    now.Unix(),
		})
		require.NoError(t, err)
	}

	summary, err := service.GetXLLMHomeMonitorSummary()
	require.NoError(t, err)
	require.Len(t, summary.Items, 1)

	item := summary.Items[0]
	assert.Equal(t, "codex-pro", item.GroupName)
	assert.Equal(t, "gpt-5.4-mini", item.ModelName)
	assert.EqualValues(t, 1200, item.LatestTTFTMs)
	assert.EqualValues(t, 1200, item.AvgTTFTMs60m)
}

func TestXLLMHomeMonitorSummaryRecordsStreamProbeFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.XLLMProbeSample{}, &model.XLLMProbeRollup{}))

	t.Setenv("XLLM_HOME_MONITOR_CONFIG", writeXLLMHomeMonitorTestConfig(t))

	err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
		TaskID:            "task_failed",
		SelectedChannelID: 1001,
		Entry:             service.XLLMHomeMonitorEntry{GroupName: "codex-pro", ModelName: "gpt-5.4-mini", Enabled: true},
		StreamProbe:       true,
		Success:           false,
		ErrorCode:         "bad_response",
		TotalMs:           1500,
		TestedAt:          time.Now().Unix(),
	})
	require.NoError(t, err)

	summary, err := service.GetXLLMHomeMonitorSummary()
	require.NoError(t, err)
	require.Len(t, summary.Items, 1)

	item := summary.Items[0]
	assert.Equal(t, "unavailable", item.Status)
	assert.EqualValues(t, 0, item.SuccessRate60m)
	assert.EqualValues(t, 0, item.LatestTTFTMs)
	assert.False(t, item.HasReliableTTFT)
}

func TestXLLMHomeMonitorSummarySkipsNonStreamProbe(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.XLLMProbeSample{}, &model.XLLMProbeRollup{}))

	t.Setenv("XLLM_HOME_MONITOR_CONFIG", writeXLLMHomeMonitorTestConfig(t))

	err := service.RecordXLLMProbeSample(service.XLLMProbeRecordInput{
		TaskID:            "task_non_stream",
		SelectedChannelID: 1001,
		Entry:             service.XLLMHomeMonitorEntry{GroupName: "codex-pro", ModelName: "gpt-5.4-mini", Enabled: true},
		Info:              &relaycommon.RelayInfo{IsStream: false},
		StreamProbe:       true,
		Success:           false,
		ErrorCode:         "bad_response",
		TotalMs:           1500,
		TestedAt:          time.Now().Unix(),
	})
	require.NoError(t, err)

	summary, err := service.GetXLLMHomeMonitorSummary()
	require.NoError(t, err)
	require.Len(t, summary.Items, 1)

	item := summary.Items[0]
	assert.Equal(t, "unknown", item.Status)
	assert.EqualValues(t, 0, item.LatestTTFTMs)
	assert.EqualValues(t, 0, item.SuccessRate60m)
	assert.False(t, item.HasReliableTTFT)
}

func writeXLLMHomeMonitorTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xllm-home-monitor.json")
	content := []byte(`{
  "enabled": true,
  "interval_minutes": 5,
  "entries": [
    {
      "group": "codex-pro",
      "model": "gpt-5.4-mini",
      "display_name": "Codex Pro",
      "enabled": true,
      "sort_order": 10
    }
  ]
}`)
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}
