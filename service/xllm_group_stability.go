package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	xllmGroupStabilityRecentRuns      = 60
	xllmGroupStabilitySevenDayHours   = 168
	xllmGroupStabilityRetentionDays   = 10
	xllmGroupStabilitySummaryCacheTTl = time.Minute
)

type XLLMGroupStabilityRecordInput struct {
	TaskID              string
	Channel             *model.Channel
	Success             bool
	ErrorCode           string
	ResponseTimeMs      int64
	ChannelStatusBefore int
	ChannelStatusAfter  int
	TestedAt            int64
}

type XLLMGroupStabilitySummary struct {
	Enabled             bool                     `json:"enabled"`
	OverallStatus       string                   `json:"overall_status"`
	RecentWindowRuns    int                      `json:"recent_window_runs"`
	SevenDayWindowHours int                      `json:"seven_day_window_hours"`
	GeneratedAt         int64                    `json:"generated_at"`
	Items               []XLLMGroupStabilityItem `json:"items"`
}

type XLLMGroupStabilityItem struct {
	GroupName           string                     `json:"group_name"`
	DisplayName         string                     `json:"display_name"`
	Status              string                     `json:"status"`
	RecentSuccessRate   float64                    `json:"recent_success_rate"`
	SevenDaySuccessRate float64                    `json:"seven_day_success_rate"`
	LastTestedAt        int64                      `json:"last_tested_at"`
	RecentBuckets       []XLLMGroupStabilityBucket `json:"recent_buckets"`
	SevenDayBuckets     []XLLMGroupStabilityBucket `json:"seven_day_buckets"`
}

type XLLMGroupStabilityBucket struct {
	Ts            int64   `json:"ts"`
	Status        string  `json:"status"`
	SuccessRate   float64 `json:"success_rate"`
	AvailableRuns int     `json:"available_runs"`
	TotalRuns     int     `json:"total_runs"`
}

type xllmGroupTaskPoint struct {
	taskID    string
	testedAt  int64
	hasSample bool
	available bool
}

var xllmGroupStabilitySummaryCache = struct {
	sync.Mutex
	expiresAt time.Time
	summary   XLLMGroupStabilitySummary
}{}

func RecordXLLMGroupStabilitySample(input XLLMGroupStabilityRecordInput) error {
	if input.Channel == nil {
		return nil
	}
	groups := normalizedChannelGroups(input.Channel)
	if len(groups) == 0 {
		return nil
	}
	testedAt := input.TestedAt
	if testedAt <= 0 {
		testedAt = common.GetTimestamp()
	}
	err := model.RecordXLLMGroupStabilitySample(&model.XLLMGroupStabilitySample{
		TaskID:              strings.TrimSpace(input.TaskID),
		ChannelID:           input.Channel.Id,
		GroupNames:          strings.Join(groups, ","),
		Success:             input.Success,
		ErrorCode:           strings.TrimSpace(input.ErrorCode),
		ResponseTimeMs:      input.ResponseTimeMs,
		ChannelStatusBefore: input.ChannelStatusBefore,
		ChannelStatusAfter:  input.ChannelStatusAfter,
		TestedAt:            testedAt,
	})
	if err != nil {
		return err
	}
	invalidateXLLMGroupStabilitySummaryCache()
	return nil
}

func CleanupXLLMGroupStabilitySamples() error {
	cutoff := time.Now().Add(-time.Duration(xllmGroupStabilityRetentionDays) * 24 * time.Hour).Unix()
	return model.DeleteXLLMGroupStabilitySamplesBefore(cutoff)
}

func GetXLLMGroupStabilitySummary() (XLLMGroupStabilitySummary, error) {
	now := time.Now()
	xllmGroupStabilitySummaryCache.Lock()
	if now.Before(xllmGroupStabilitySummaryCache.expiresAt) {
		summary := xllmGroupStabilitySummaryCache.summary
		xllmGroupStabilitySummaryCache.Unlock()
		return summary, nil
	}
	xllmGroupStabilitySummaryCache.Unlock()

	summary, err := buildXLLMGroupStabilitySummary(now)
	if err != nil {
		return XLLMGroupStabilitySummary{}, err
	}

	xllmGroupStabilitySummaryCache.Lock()
	xllmGroupStabilitySummaryCache.summary = summary
	xllmGroupStabilitySummaryCache.expiresAt = now.Add(xllmGroupStabilitySummaryCacheTTl)
	xllmGroupStabilitySummaryCache.Unlock()

	return summary, nil
}

func buildXLLMGroupStabilitySummary(now time.Time) (XLLMGroupStabilitySummary, error) {
	config, err := LoadXLLMGroupStabilityConfig()
	if err != nil {
		return XLLMGroupStabilitySummary{}, err
	}
	summary := XLLMGroupStabilitySummary{
		Enabled:             xllmGroupStabilityConfigEnabled(config),
		OverallStatus:       "unknown",
		RecentWindowRuns:    xllmGroupStabilityRecentRuns,
		SevenDayWindowHours: xllmGroupStabilitySevenDayHours,
		GeneratedAt:         now.Unix(),
	}
	if !summary.Enabled {
		return summary, nil
	}

	startTs := now.Add(-time.Duration(xllmGroupStabilityRetentionDays) * 24 * time.Hour).Unix()
	samples, err := model.GetXLLMGroupStabilitySamplesSince(startTs)
	if err != nil {
		return XLLMGroupStabilitySummary{}, err
	}

	groupTasks, orderedTasks := buildXLLMGroupTaskPoints(samples, config.Groups)
	items := make([]XLLMGroupStabilityItem, 0, len(config.Groups))
	for _, group := range config.Groups {
		points := groupTasks[group.Group]
		recentBuckets := buildXLLMRecentBuckets(points, orderedTasks)
		sevenDayEndHour := xllmSevenDayEndHour(points, now)
		sevenDayBuckets := buildXLLMSevenDayBuckets(points, sevenDayEndHour)
		item := XLLMGroupStabilityItem{
			GroupName:           group.Group,
			DisplayName:         group.DisplayName,
			Status:              xllmGroupStatus(recentBuckets),
			RecentSuccessRate:   xllmBucketSuccessRate(recentBuckets),
			SevenDaySuccessRate: xllmBucketSuccessRate(sevenDayBuckets),
			LastTestedAt:        xllmLastTestedAt(recentBuckets),
			RecentBuckets:       recentBuckets,
			SevenDayBuckets:     sevenDayBuckets,
		}
		items = append(items, item)
	}
	summary.Items = items
	summary.OverallStatus = xllmOverallStatus(items)
	return summary, nil
}

func buildXLLMGroupTaskPoints(samples []model.XLLMGroupStabilitySample, groups []XLLMGroupStabilityGroupConfig) (map[string]map[string]xllmGroupTaskPoint, []xllmGroupTaskPoint) {
	enabledGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		enabledGroups[group.Group] = struct{}{}
	}

	groupTasks := make(map[string]map[string]xllmGroupTaskPoint, len(groups))
	for _, group := range groups {
		groupTasks[group.Group] = map[string]xllmGroupTaskPoint{}
	}
	globalTasks := map[string]xllmGroupTaskPoint{}

	for _, sample := range samples {
		taskID := strings.TrimSpace(sample.TaskID)
		if taskID == "" {
			taskID = fmt.Sprintf("sample:%d", sample.ID)
		}
		global := globalTasks[taskID]
		global.taskID = taskID
		global.hasSample = true
		if sample.TestedAt > global.testedAt {
			global.testedAt = sample.TestedAt
		}
		globalTasks[taskID] = global

		for _, group := range splitGroupNames(sample.GroupNames) {
			if _, ok := enabledGroups[group]; !ok {
				continue
			}
			point := groupTasks[group][taskID]
			point.taskID = taskID
			point.hasSample = true
			if sample.TestedAt > point.testedAt {
				point.testedAt = sample.TestedAt
			}
			if sample.Success {
				point.available = true
			}
			groupTasks[group][taskID] = point
		}
	}

	orderedTasks := make([]xllmGroupTaskPoint, 0, len(globalTasks))
	for _, point := range globalTasks {
		orderedTasks = append(orderedTasks, point)
	}
	sort.Slice(orderedTasks, func(i, j int) bool {
		if orderedTasks[i].testedAt == orderedTasks[j].testedAt {
			return orderedTasks[i].taskID < orderedTasks[j].taskID
		}
		return orderedTasks[i].testedAt < orderedTasks[j].testedAt
	})
	if len(orderedTasks) > xllmGroupStabilityRecentRuns {
		orderedTasks = orderedTasks[len(orderedTasks)-xllmGroupStabilityRecentRuns:]
	}
	return groupTasks, orderedTasks
}

func buildXLLMRecentBuckets(points map[string]xllmGroupTaskPoint, orderedTasks []xllmGroupTaskPoint) []XLLMGroupStabilityBucket {
	buckets := make([]XLLMGroupStabilityBucket, 0, xllmGroupStabilityRecentRuns)
	padding := xllmGroupStabilityRecentRuns - len(orderedTasks)
	for i := 0; i < padding; i++ {
		buckets = append(buckets, XLLMGroupStabilityBucket{Status: "unknown"})
	}
	for _, task := range orderedTasks {
		point := points[task.taskID]
		buckets = append(buckets, xllmTaskBucket(point, task.testedAt))
	}
	return buckets
}

func buildXLLMSevenDayBuckets(points map[string]xllmGroupTaskPoint, endHour int64) []XLLMGroupStabilityBucket {
	startHour := endHour - int64(xllmGroupStabilitySevenDayHours-1)*3600
	hourly := map[int64][]xllmGroupTaskPoint{}
	for _, point := range points {
		if !point.hasSample || point.testedAt < startHour {
			continue
		}
		hour := point.testedAt - (point.testedAt % 3600)
		hourly[hour] = append(hourly[hour], point)
	}

	buckets := make([]XLLMGroupStabilityBucket, 0, xllmGroupStabilitySevenDayHours)
	for hour := startHour; hour <= endHour; hour += 3600 {
		runs := hourly[hour]
		if len(runs) == 0 {
			buckets = append(buckets, XLLMGroupStabilityBucket{
				Ts:     hour,
				Status: "unknown",
			})
			continue
		}
		availableRuns := 0
		for _, run := range runs {
			if run.available {
				availableRuns++
			}
		}
		rate := float64(availableRuns) / float64(len(runs)) * 100
		status := xllmHourlyBucketStatus(rate)
		buckets = append(buckets, XLLMGroupStabilityBucket{
			Ts:            hour,
			Status:        status,
			SuccessRate:   roundPercent(rate),
			AvailableRuns: availableRuns,
			TotalRuns:     len(runs),
		})
	}
	return buckets
}

func xllmSevenDayEndHour(points map[string]xllmGroupTaskPoint, now time.Time) int64 {
	currentHour := now.Unix() - (now.Unix() % 3600)
	latestTestedAt := int64(0)
	for _, point := range points {
		if point.hasSample && point.testedAt > latestTestedAt {
			latestTestedAt = point.testedAt
		}
	}
	if latestTestedAt <= 0 {
		return currentHour
	}
	latestHour := latestTestedAt - (latestTestedAt % 3600)
	if latestHour > currentHour {
		return currentHour
	}
	return latestHour
}

func xllmHourlyBucketStatus(successRate float64) string {
	if successRate >= 100 {
		return "available"
	}
	if successRate < 50 {
		return "unavailable"
	}
	return "degraded"
}

func xllmTaskBucket(point xllmGroupTaskPoint, fallbackTs int64) XLLMGroupStabilityBucket {
	if !point.hasSample {
		return XLLMGroupStabilityBucket{
			Ts:     fallbackTs,
			Status: "unknown",
		}
	}
	if point.available {
		return XLLMGroupStabilityBucket{
			Ts:            point.testedAt,
			Status:        "available",
			SuccessRate:   100,
			AvailableRuns: 1,
			TotalRuns:     1,
		}
	}
	return XLLMGroupStabilityBucket{
		Ts:          point.testedAt,
		Status:      "unavailable",
		SuccessRate: 0,
		TotalRuns:   1,
	}
}

func xllmGroupStatus(buckets []XLLMGroupStabilityBucket) string {
	known := make([]XLLMGroupStabilityBucket, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.Status != "unknown" {
			known = append(known, bucket)
		}
	}
	if len(known) == 0 {
		return "unknown"
	}
	latest := known[len(known)-1]
	if latest.Status == "unavailable" {
		return "unavailable"
	}
	rate := xllmBucketSuccessRate(known)
	if rate < 99 {
		return "degraded"
	}
	return "operational"
}

func xllmOverallStatus(items []XLLMGroupStabilityItem) string {
	if len(items) == 0 {
		return "unknown"
	}
	hasKnown := false
	hasDegraded := false
	for _, item := range items {
		switch item.Status {
		case "unavailable":
			return "unavailable"
		case "degraded":
			hasKnown = true
			hasDegraded = true
		case "operational":
			hasKnown = true
		}
	}
	if !hasKnown {
		return "unknown"
	}
	if hasDegraded {
		return "degraded"
	}
	return "operational"
}

func xllmBucketSuccessRate(buckets []XLLMGroupStabilityBucket) float64 {
	total := 0
	available := 0
	for _, bucket := range buckets {
		if bucket.Status == "unknown" || bucket.TotalRuns == 0 {
			continue
		}
		total += bucket.TotalRuns
		available += bucket.AvailableRuns
	}
	if total == 0 {
		return 0
	}
	return roundPercent(float64(available) / float64(total) * 100)
}

func xllmLastTestedAt(buckets []XLLMGroupStabilityBucket) int64 {
	for i := len(buckets) - 1; i >= 0; i-- {
		if buckets[i].Status != "unknown" {
			return buckets[i].Ts
		}
	}
	return 0
}

func normalizedChannelGroups(channel *model.Channel) []string {
	groups := channel.GetGroups()
	if len(groups) == 0 {
		groups = []string{"default"}
	}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	return normalized
}

func splitGroupNames(groupNames string) []string {
	rawGroups := strings.Split(strings.Trim(groupNames, ","), ",")
	groups := make([]string, 0, len(rawGroups))
	for _, group := range rawGroups {
		group = strings.TrimSpace(group)
		if group != "" {
			groups = append(groups, group)
		}
	}
	return groups
}

func roundPercent(value float64) float64 {
	return math.Round(value*100) / 100
}

func invalidateXLLMGroupStabilitySummaryCache() {
	xllmGroupStabilitySummaryCache.Lock()
	xllmGroupStabilitySummaryCache.expiresAt = time.Time{}
	xllmGroupStabilitySummaryCache.Unlock()
}
