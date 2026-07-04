package service

import (
	"errors"
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"gorm.io/gorm"
)

const (
	XLLMProbeStatusSuccess = "success"
	XLLMProbeStatusFailed  = "failed"

	xllmProbeSchemaVersion       = 3
	xllmProbeMinuteBucketSeconds = int64(60)
	xllmProbeHourBucketSeconds   = int64(3600)
	xllmProbeRecentBuckets       = 60
	xllmProbeSevenDayHours       = 24 * 7
)

type XLLMProbeRecordInput struct {
	TaskID            string
	Entry             XLLMHomeMonitorEntry
	SelectedChannelID int
	Info              *relaycommon.RelayInfo
	StreamProbe       bool
	Success           bool
	ErrorCode         string
	TotalMs           int64
	TestedAt          int64
}

type XLLMHomeMonitorBucket struct {
	Ts           int64   `json:"ts"`
	Status       string  `json:"status"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTTFTMs    int64   `json:"avg_ttft_ms"`
	P50TTFTMs    int64   `json:"p50_ttft_ms"`
	SampleCount  int64   `json:"sample_count"`
	SuccessCount int64   `json:"success_count"`
	FailureCount int64   `json:"failure_count"`
	TTFTCount    int64   `json:"ttft_count"`
	LastTestedAt int64   `json:"last_tested_at"`
}

type XLLMHomeMonitorItem struct {
	GroupName       string                  `json:"group_name"`
	ModelName       string                  `json:"model_name"`
	DisplayName     string                  `json:"display_name"`
	Status          string                  `json:"status"`
	SuccessRate60m  float64                 `json:"success_rate_60m"`
	SuccessRate7d   float64                 `json:"success_rate_7d"`
	AvgTTFTMs60m    int64                   `json:"avg_ttft_ms_60m"`
	AvgTTFTMs7d     int64                   `json:"avg_ttft_ms_7d"`
	P50TTFTMs60m    int64                   `json:"p50_ttft_ms_60m"`
	P50TTFTMs7d     int64                   `json:"p50_ttft_ms_7d"`
	LatestTTFTMs    int64                   `json:"latest_ttft_ms"`
	LastTestedAt    int64                   `json:"last_tested_at"`
	RecentBuckets   []XLLMHomeMonitorBucket `json:"recent_buckets"`
	SevenDayBuckets []XLLMHomeMonitorBucket `json:"seven_day_buckets"`
	Configured      bool                    `json:"configured"`
	StreamOnly      bool                    `json:"stream_only"`
	HasReliableTTFT bool                    `json:"has_reliable_ttft"`
}

type XLLMHomeMonitorSummary struct {
	Items              []XLLMHomeMonitorItem `json:"items"`
	TotalItems         int                   `json:"total_items"`
	OperationalItems   int                   `json:"operational_items"`
	DegradedItems      int                   `json:"degraded_items"`
	UnavailableItems   int                   `json:"unavailable_items"`
	UnknownItems       int                   `json:"unknown_items"`
	OverallStatus      string                `json:"overall_status"`
	LastTestedAt       int64                 `json:"last_tested_at"`
	RecentWindowSecs   int64                 `json:"recent_window_secs"`
	SevenDayWindowSecs int64                 `json:"seven_day_window_secs"`
}

type xllmProbeAggregate struct {
	SampleCount  int64
	SuccessCount int64
	FailureCount int64
	TTFTSumMs    int64
	TTFTCount    int64
	P50Values    []int64
	P50TTFTMs    int64
	LastTestedAt int64
}

func RecordXLLMProbeSample(input XLLMProbeRecordInput) error {
	if input.Entry.GroupName == "" || input.Entry.ModelName == "" {
		return nil
	}
	if !input.StreamProbe {
		return nil
	}
	if input.Info != nil && !input.Info.IsStream {
		return nil
	}
	if input.TestedAt <= 0 {
		input.TestedAt = time.Now().Unix()
	}
	if input.TotalMs < 0 {
		input.TotalMs = 0
	}

	status := XLLMProbeStatusFailed
	if input.Success && input.Info != nil && input.Info.HasSendResponse() {
		status = XLLMProbeStatusSuccess
	}

	var ttftMs *int64
	if status == XLLMProbeStatusSuccess && input.Info != nil {
		value := input.Info.FirstResponseTime.Sub(input.Info.StartTime).Milliseconds()
		if value >= 0 {
			ttftMs = &value
		}
	}

	sample := &model.XLLMProbeSample{
		ProbeVersion:      xllmProbeSchemaVersion,
		TaskID:            input.TaskID,
		SelectedChannelID: input.SelectedChannelID,
		GroupName:         input.Entry.GroupName,
		ModelName:         input.Entry.ModelName,
		Status:            status,
		ErrorCode:         input.ErrorCode,
		TTFTMs:            ttftMs,
		TotalMs:           input.TotalMs,
		TestedAt:          input.TestedAt,
		CreatedAt:         common.GetTimestamp(),
	}
	if err := model.CreateXLLMProbeSample(sample); err != nil {
		return err
	}
	for _, bucketSeconds := range []int64{xllmProbeMinuteBucketSeconds, xllmProbeHourBucketSeconds} {
		if err := model.UpsertXLLMProbeRollup(buildXLLMProbeRollup(sample, bucketSeconds)); err != nil {
			return err
		}
	}
	return nil
}

func GetXLLMHomeMonitorSummary() (XLLMHomeMonitorSummary, error) {
	entries := GetXLLMHomeMonitorEntries()
	recentBucketSeconds := xllmProbeRecentBucketSeconds()
	summary := XLLMHomeMonitorSummary{
		OverallStatus:      "unknown",
		RecentWindowSecs:   int64(xllmProbeRecentBuckets) * recentBucketSeconds,
		SevenDayWindowSecs: int64(xllmProbeSevenDayHours) * 3600,
	}
	if len(entries) == 0 {
		return summary, nil
	}

	now := time.Now().Unix()
	recentStart := bucketStart(now-int64(xllmProbeRecentBuckets-1)*recentBucketSeconds, recentBucketSeconds)
	recentEnd := bucketStart(now, recentBucketSeconds)
	sevenDayStart := bucketStart(now-int64(xllmProbeSevenDayHours-1)*3600, xllmProbeHourBucketSeconds)
	sevenDayEnd := bucketStart(now, xllmProbeHourBucketSeconds)
	keys := xllmProbeEntryKeys(entries)

	recentRollupEnd := recentEnd + recentBucketSeconds - xllmProbeMinuteBucketSeconds
	recentRollups, err := model.GetXLLMProbeRollups(xllmProbeSchemaVersion, xllmProbeMinuteBucketSeconds, recentStart, recentRollupEnd, keys)
	if err != nil {
		return summary, err
	}
	sevenDayRollups, err := model.GetXLLMProbeRollups(xllmProbeSchemaVersion, xllmProbeHourBucketSeconds, sevenDayStart, sevenDayEnd, keys)
	if err != nil {
		return summary, err
	}
	recentSampleEnd := recentEnd + recentBucketSeconds - 1
	recentSamples, err := model.GetXLLMProbeSamples(xllmProbeSchemaVersion, recentStart, recentSampleEnd, keys)
	if err != nil {
		return summary, err
	}
	sevenDaySamples, err := model.GetXLLMProbeSamples(xllmProbeSchemaVersion, sevenDayStart, sevenDayEnd+xllmProbeHourBucketSeconds-1, keys)
	if err != nil {
		return summary, err
	}

	recentByEntry := groupXLLMProbeRollupsByInterval(recentRollups, recentBucketSeconds)
	sevenDayByEntry := groupXLLMProbeRollups(sevenDayRollups)
	recentP50ByEntry := groupXLLMProbeSampleP50ByInterval(recentSamples, recentBucketSeconds)
	sevenDayP50ByEntry := groupXLLMProbeSampleP50ByInterval(sevenDaySamples, xllmProbeHourBucketSeconds)
	items := make([]XLLMHomeMonitorItem, 0, len(entries))
	for _, entry := range entries {
		key := xllmProbeEntryKey(entry.GroupName, entry.ModelName)
		recentBuckets := fillXLLMProbeBuckets(recentByEntry[key], recentStart, recentEnd, recentBucketSeconds)
		sevenDayBuckets := fillXLLMProbeBuckets(sevenDayByEntry[key], sevenDayStart, sevenDayEnd, xllmProbeHourBucketSeconds)
		applyXLLMProbeBucketP50(recentBuckets, recentP50ByEntry[key])
		applyXLLMProbeBucketP50(sevenDayBuckets, sevenDayP50ByEntry[key])
		item := buildXLLMHomeMonitorItem(entry, recentBuckets, sevenDayBuckets)
		items = append(items, item)
		if item.LastTestedAt > summary.LastTestedAt {
			summary.LastTestedAt = item.LastTestedAt
		}
		switch item.Status {
		case "operational":
			summary.OperationalItems++
		case "degraded":
			summary.DegradedItems++
		case "unavailable":
			summary.UnavailableItems++
		default:
			summary.UnknownItems++
		}
	}
	summary.Items = items
	summary.TotalItems = len(items)
	summary.OverallStatus = xllmOverallStatus(summary)
	return summary, nil
}

func GetLatestXLLMProbeTTFT(groupName string, modelName string) (int64, int64, bool, error) {
	sample, err := model.GetLatestXLLMProbeSample(xllmProbeSchemaVersion, groupName, modelName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	if sample.TTFTMs == nil {
		return 0, sample.TestedAt, false, nil
	}
	return *sample.TTFTMs, sample.TestedAt, true, nil
}

func xllmProbeRecentBucketSeconds() int64 {
	seconds := int64(math.Round(XLLMHomeMonitorInterval().Seconds()))
	if seconds < xllmProbeMinuteBucketSeconds {
		return xllmProbeMinuteBucketSeconds
	}
	return seconds
}

func buildXLLMProbeRollup(sample *model.XLLMProbeSample, bucketSeconds int64) *model.XLLMProbeRollup {
	rollup := &model.XLLMProbeRollup{
		ProbeVersion:      xllmProbeSchemaVersion,
		BucketSeconds:     bucketSeconds,
		BucketTs:          bucketStart(sample.TestedAt, bucketSeconds),
		GroupName:         sample.GroupName,
		ModelName:         sample.ModelName,
		SelectedChannelID: sample.SelectedChannelID,
		SampleCount:       1,
		TotalSumMs:        sample.TotalMs,
		TotalCount:        1,
		MinTotalMs:        sample.TotalMs,
		MaxTotalMs:        sample.TotalMs,
		LastStatus:        sample.Status,
		LastTestedAt:      sample.TestedAt,
	}
	if sample.Status == XLLMProbeStatusSuccess {
		rollup.SuccessCount = 1
	} else {
		rollup.FailureCount = 1
	}
	if sample.TTFTMs != nil {
		rollup.TTFTSumMs = *sample.TTFTMs
		rollup.TTFTCount = 1
	}
	return rollup
}

func bucketStart(ts int64, bucketSeconds int64) int64 {
	if bucketSeconds <= 0 {
		return ts
	}
	return ts - (ts % bucketSeconds)
}

func xllmProbeEntryKeys(entries []XLLMHomeMonitorEntry) []model.XLLMProbeEntryKey {
	keys := make([]model.XLLMProbeEntryKey, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, model.XLLMProbeEntryKey{
			GroupName: entry.GroupName,
			ModelName: entry.ModelName,
		})
	}
	return keys
}

func xllmProbeEntryKey(groupName string, modelName string) model.XLLMProbeEntryKey {
	return model.XLLMProbeEntryKey{
		GroupName: groupName,
		ModelName: modelName,
	}
}

func groupXLLMProbeRollups(rollups []model.XLLMProbeRollup) map[model.XLLMProbeEntryKey]map[int64]model.XLLMProbeRollup {
	grouped := make(map[model.XLLMProbeEntryKey]map[int64]model.XLLMProbeRollup)
	for _, rollup := range rollups {
		key := xllmProbeEntryKey(rollup.GroupName, rollup.ModelName)
		if _, ok := grouped[key]; !ok {
			grouped[key] = make(map[int64]model.XLLMProbeRollup)
		}
		grouped[key][rollup.BucketTs] = rollup
	}
	return grouped
}

func groupXLLMProbeRollupsByInterval(rollups []model.XLLMProbeRollup, bucketSeconds int64) map[model.XLLMProbeEntryKey]map[int64]model.XLLMProbeRollup {
	grouped := make(map[model.XLLMProbeEntryKey]map[int64]model.XLLMProbeRollup)
	if bucketSeconds <= 0 {
		return grouped
	}
	for _, rollup := range rollups {
		key := xllmProbeEntryKey(rollup.GroupName, rollup.ModelName)
		if _, ok := grouped[key]; !ok {
			grouped[key] = make(map[int64]model.XLLMProbeRollup)
		}
		targetTs := bucketStart(rollup.BucketTs, bucketSeconds)
		merged := grouped[key][targetTs]
		if merged.BucketSeconds == 0 {
			merged.BucketSeconds = bucketSeconds
			merged.BucketTs = targetTs
			merged.GroupName = rollup.GroupName
			merged.ModelName = rollup.ModelName
			merged.SelectedChannelID = rollup.SelectedChannelID
		}
		mergeXLLMProbeRollup(&merged, rollup)
		grouped[key][targetTs] = merged
	}
	return grouped
}

func groupXLLMProbeSampleP50ByInterval(samples []model.XLLMProbeSample, bucketSeconds int64) map[model.XLLMProbeEntryKey]map[int64]int64 {
	groupedValues := make(map[model.XLLMProbeEntryKey]map[int64][]int64)
	if bucketSeconds <= 0 {
		return nil
	}
	for _, sample := range samples {
		if sample.TTFTMs == nil {
			continue
		}
		key := xllmProbeEntryKey(sample.GroupName, sample.ModelName)
		if _, ok := groupedValues[key]; !ok {
			groupedValues[key] = make(map[int64][]int64)
		}
		targetTs := bucketStart(sample.TestedAt, bucketSeconds)
		groupedValues[key][targetTs] = append(groupedValues[key][targetTs], *sample.TTFTMs)
	}

	result := make(map[model.XLLMProbeEntryKey]map[int64]int64, len(groupedValues))
	for key, buckets := range groupedValues {
		result[key] = make(map[int64]int64, len(buckets))
		for ts, values := range buckets {
			result[key][ts] = medianInt64(values)
		}
	}
	return result
}

func applyXLLMProbeBucketP50(buckets []XLLMHomeMonitorBucket, p50ByTs map[int64]int64) {
	if len(buckets) == 0 || len(p50ByTs) == 0 {
		return
	}
	for index := range buckets {
		if value, ok := p50ByTs[buckets[index].Ts]; ok {
			buckets[index].P50TTFTMs = value
		}
	}
}

func mergeXLLMProbeRollup(dst *model.XLLMProbeRollup, src model.XLLMProbeRollup) {
	dst.SampleCount += src.SampleCount
	dst.SuccessCount += src.SuccessCount
	dst.FailureCount += src.FailureCount
	dst.TTFTSumMs += src.TTFTSumMs
	dst.TTFTCount += src.TTFTCount
	dst.TotalSumMs += src.TotalSumMs
	dst.TotalCount += src.TotalCount
	if dst.MinTotalMs == 0 || (src.MinTotalMs > 0 && src.MinTotalMs < dst.MinTotalMs) {
		dst.MinTotalMs = src.MinTotalMs
	}
	if src.MaxTotalMs > dst.MaxTotalMs {
		dst.MaxTotalMs = src.MaxTotalMs
	}
	if src.LastTestedAt >= dst.LastTestedAt {
		dst.LastStatus = src.LastStatus
		dst.LastTestedAt = src.LastTestedAt
	}
}

func fillXLLMProbeBuckets(rollups map[int64]model.XLLMProbeRollup, startTs int64, endTs int64, bucketSeconds int64) []XLLMHomeMonitorBucket {
	if startTs > endTs || bucketSeconds <= 0 {
		return nil
	}
	buckets := make([]XLLMHomeMonitorBucket, 0, int((endTs-startTs)/bucketSeconds)+1)
	for ts := startTs; ts <= endTs; ts += bucketSeconds {
		rollup, ok := rollups[ts]
		if !ok {
			buckets = append(buckets, XLLMHomeMonitorBucket{
				Ts:     ts,
				Status: "unknown",
			})
			continue
		}
		buckets = append(buckets, xllmProbeBucketFromRollup(rollup))
	}
	return buckets
}

func xllmProbeBucketFromRollup(rollup model.XLLMProbeRollup) XLLMHomeMonitorBucket {
	status := "unknown"
	if rollup.SampleCount > 0 {
		if rollup.SuccessCount == rollup.SampleCount {
			status = "operational"
		} else if rollup.SuccessCount > 0 {
			status = "degraded"
		} else {
			status = "unavailable"
		}
	}
	return XLLMHomeMonitorBucket{
		Ts:           rollup.BucketTs,
		Status:       status,
		SuccessRate:  roundPercent(successRate(rollup.SuccessCount, rollup.SampleCount)),
		AvgTTFTMs:    average(rollup.TTFTSumMs, rollup.TTFTCount),
		P50TTFTMs:    average(rollup.TTFTSumMs, rollup.TTFTCount),
		SampleCount:  rollup.SampleCount,
		SuccessCount: rollup.SuccessCount,
		FailureCount: rollup.FailureCount,
		TTFTCount:    rollup.TTFTCount,
		LastTestedAt: rollup.LastTestedAt,
	}
}

func buildXLLMHomeMonitorItem(entry XLLMHomeMonitorEntry, recentBuckets []XLLMHomeMonitorBucket, sevenDayBuckets []XLLMHomeMonitorBucket) XLLMHomeMonitorItem {
	recentAgg := aggregateXLLMProbeBuckets(recentBuckets)
	sevenDayAgg := aggregateXLLMProbeBuckets(sevenDayBuckets)
	latestTTFT, latestTestedAt, hasLatestTTFT, err := GetLatestXLLMProbeTTFT(entry.GroupName, entry.ModelName)
	if err != nil {
		common.SysLog("failed to get latest X-LLM probe sample: " + err.Error())
	}
	status := "unknown"
	if recentAgg.SampleCount > 0 {
		if recentAgg.SuccessCount == recentAgg.SampleCount {
			status = "operational"
		} else if recentAgg.SuccessCount > 0 {
			status = "degraded"
		} else {
			status = "unavailable"
		}
	}
	if latestTestedAt > recentAgg.LastTestedAt {
		recentAgg.LastTestedAt = latestTestedAt
	}
	return XLLMHomeMonitorItem{
		GroupName:       entry.GroupName,
		ModelName:       entry.ModelName,
		DisplayName:     entry.DisplayName,
		Status:          status,
		SuccessRate60m:  roundPercent(successRate(recentAgg.SuccessCount, recentAgg.SampleCount)),
		SuccessRate7d:   roundPercent(successRate(sevenDayAgg.SuccessCount, sevenDayAgg.SampleCount)),
		AvgTTFTMs60m:    average(recentAgg.TTFTSumMs, recentAgg.TTFTCount),
		AvgTTFTMs7d:     average(sevenDayAgg.TTFTSumMs, sevenDayAgg.TTFTCount),
		P50TTFTMs60m:    recentAgg.P50TTFTMs,
		P50TTFTMs7d:     sevenDayAgg.P50TTFTMs,
		LatestTTFTMs:    latestTTFT,
		LastTestedAt:    recentAgg.LastTestedAt,
		RecentBuckets:   recentBuckets,
		SevenDayBuckets: sevenDayBuckets,
		Configured:      true,
		StreamOnly:      true,
		HasReliableTTFT: hasLatestTTFT || recentAgg.TTFTCount > 0 || sevenDayAgg.TTFTCount > 0,
	}
}

func aggregateXLLMProbeBuckets(buckets []XLLMHomeMonitorBucket) xllmProbeAggregate {
	agg := xllmProbeAggregate{}
	for _, bucket := range buckets {
		if bucket.SampleCount <= 0 {
			continue
		}
		agg.SampleCount += bucket.SampleCount
		agg.SuccessCount += bucket.SuccessCount
		agg.FailureCount += bucket.FailureCount
		if bucket.AvgTTFTMs > 0 && bucket.TTFTCount > 0 {
			agg.TTFTSumMs += bucket.AvgTTFTMs * bucket.TTFTCount
			agg.TTFTCount += bucket.TTFTCount
		}
		if bucket.P50TTFTMs > 0 {
			agg.P50Values = append(agg.P50Values, bucket.P50TTFTMs)
		}
		if bucket.LastTestedAt > agg.LastTestedAt {
			agg.LastTestedAt = bucket.LastTestedAt
		}
	}
	agg.P50TTFTMs = medianInt64(agg.P50Values)
	return agg
}

func successRate(successCount int64, sampleCount int64) float64 {
	if sampleCount <= 0 {
		return 0
	}
	return float64(successCount) / float64(sampleCount) * 100
}

func average(sum int64, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func medianInt64(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func roundPercent(value float64) float64 {
	return math.Round(value*100) / 100
}

func xllmOverallStatus(summary XLLMHomeMonitorSummary) string {
	if summary.TotalItems == 0 || summary.UnknownItems == summary.TotalItems {
		return "unknown"
	}
	if summary.UnavailableItems == summary.TotalItems {
		return "unavailable"
	}
	if summary.DegradedItems > 0 || summary.UnavailableItems > 0 || summary.UnknownItems > 0 {
		return "degraded"
	}
	return "operational"
}
