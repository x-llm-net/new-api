package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// XLLMProbeRollup stores aggregated homepage monitor probe data. BucketSeconds
// is 60 for minute buckets and 3600 for hour buckets.
type XLLMProbeRollup struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	ProbeVersion      int    `json:"probe_version" gorm:"not null;default:3;uniqueIndex:idx_xllm_probe_rollup_bucket_v3,priority:1"`
	BucketSeconds     int64  `json:"bucket_seconds" gorm:"uniqueIndex:idx_xllm_probe_rollup_bucket_v3,priority:2;index"`
	BucketTs          int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_xllm_probe_rollup_bucket_v3,priority:3;index:idx_xllm_probe_rollup_lookup_v3,priority:3"`
	GroupName         string `json:"group_name" gorm:"size:64;not null;uniqueIndex:idx_xllm_probe_rollup_bucket_v3,priority:4;index:idx_xllm_probe_rollup_lookup_v3,priority:1"`
	ModelName         string `json:"model_name" gorm:"size:128;not null;uniqueIndex:idx_xllm_probe_rollup_bucket_v3,priority:5;index:idx_xllm_probe_rollup_lookup_v3,priority:2"`
	SelectedChannelID int    `json:"selected_channel_id" gorm:"index"`
	SampleCount       int64  `json:"sample_count" gorm:"default:0"`
	SuccessCount      int64  `json:"success_count" gorm:"default:0"`
	FailureCount      int64  `json:"failure_count" gorm:"default:0"`
	TTFTSumMs         int64  `json:"ttft_sum_ms" gorm:"column:ttft_sum_ms;default:0"`
	TTFTCount         int64  `json:"ttft_count" gorm:"column:ttft_count;default:0"`
	TotalSumMs        int64  `json:"total_sum_ms" gorm:"default:0"`
	TotalCount        int64  `json:"total_count" gorm:"default:0"`
	MinTotalMs        int64  `json:"min_total_ms" gorm:"default:0"`
	MaxTotalMs        int64  `json:"max_total_ms" gorm:"default:0"`
	LastStatus        string `json:"last_status" gorm:"size:32"`
	LastTestedAt      int64  `json:"last_tested_at" gorm:"index"`
}

func (XLLMProbeRollup) TableName() string {
	return "xllm_probe_rollups"
}

func UpsertXLLMProbeRollup(rollup *XLLMProbeRollup) error {
	if rollup == nil || rollup.SampleCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "probe_version"},
			{Name: "bucket_seconds"},
			{Name: "bucket_ts"},
			{Name: "group_name"},
			{Name: "model_name"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"sample_count":  gorm.Expr("xllm_probe_rollups.sample_count + ?", rollup.SampleCount),
			"success_count": gorm.Expr("xllm_probe_rollups.success_count + ?", rollup.SuccessCount),
			"failure_count": gorm.Expr("xllm_probe_rollups.failure_count + ?", rollup.FailureCount),
			"ttft_sum_ms":   gorm.Expr("xllm_probe_rollups.ttft_sum_ms + ?", rollup.TTFTSumMs),
			"ttft_count":    gorm.Expr("xllm_probe_rollups.ttft_count + ?", rollup.TTFTCount),
			"total_sum_ms":  gorm.Expr("xllm_probe_rollups.total_sum_ms + ?", rollup.TotalSumMs),
			"total_count":   gorm.Expr("xllm_probe_rollups.total_count + ?", rollup.TotalCount),
			"min_total_ms": gorm.Expr(
				"CASE WHEN xllm_probe_rollups.min_total_ms = 0 OR ? < xllm_probe_rollups.min_total_ms THEN ? ELSE xllm_probe_rollups.min_total_ms END",
				rollup.MinTotalMs,
				rollup.MinTotalMs,
			),
			"max_total_ms": gorm.Expr(
				"CASE WHEN ? > xllm_probe_rollups.max_total_ms THEN ? ELSE xllm_probe_rollups.max_total_ms END",
				rollup.MaxTotalMs,
				rollup.MaxTotalMs,
			),
			"last_status": gorm.Expr(
				"CASE WHEN ? >= xllm_probe_rollups.last_tested_at THEN ? ELSE xllm_probe_rollups.last_status END",
				rollup.LastTestedAt,
				rollup.LastStatus,
			),
			"selected_channel_id": gorm.Expr(
				"CASE WHEN ? >= xllm_probe_rollups.last_tested_at THEN ? ELSE xllm_probe_rollups.selected_channel_id END",
				rollup.LastTestedAt,
				rollup.SelectedChannelID,
			),
			"last_tested_at": gorm.Expr(
				"CASE WHEN ? > xllm_probe_rollups.last_tested_at THEN ? ELSE xllm_probe_rollups.last_tested_at END",
				rollup.LastTestedAt,
				rollup.LastTestedAt,
			),
		}),
	}).Create(rollup).Error
}

func GetXLLMProbeRollups(probeVersion int, bucketSeconds int64, startTs int64, endTs int64, entries []XLLMProbeEntryKey) ([]XLLMProbeRollup, error) {
	rollups := make([]XLLMProbeRollup, 0)
	if bucketSeconds <= 0 || startTs > endTs || len(entries) == 0 {
		return rollups, nil
	}
	query := DB.Model(&XLLMProbeRollup{}).
		Where("probe_version = ? AND bucket_seconds = ? AND bucket_ts >= ? AND bucket_ts <= ?", probeVersion, bucketSeconds, startTs, endTs)
	query = applyXLLMProbeEntryFilter(query, entries)
	err := query.Order("bucket_ts asc").Find(&rollups).Error
	return rollups, err
}

type XLLMProbeEntryKey struct {
	GroupName string
	ModelName string
}

func applyXLLMProbeEntryFilter(query *gorm.DB, entries []XLLMProbeEntryKey) *gorm.DB {
	if len(entries) == 1 {
		return query.Where("group_name = ? AND model_name = ?", entries[0].GroupName, entries[0].ModelName)
	}
	filter := DB.Session(&gorm.Session{NewDB: true}).Where("group_name = ? AND model_name = ?", entries[0].GroupName, entries[0].ModelName)
	for _, entry := range entries[1:] {
		filter = filter.Or("group_name = ? AND model_name = ?", entry.GroupName, entry.ModelName)
	}
	return query.Where(filter)
}
