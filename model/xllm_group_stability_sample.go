package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type XLLMGroupStabilitySample struct {
	ID                  int64  `json:"id" gorm:"primary_key"`
	TaskID              string `json:"task_id" gorm:"type:varchar(64);index"`
	ChannelID           int    `json:"channel_id" gorm:"index"`
	GroupNames          string `json:"group_names" gorm:"type:varchar(1024)"`
	Success             bool   `json:"success" gorm:"index"`
	ErrorCode           string `json:"error_code" gorm:"type:varchar(128)"`
	ResponseTimeMs      int64  `json:"response_time_ms"`
	ChannelStatusBefore int    `json:"channel_status_before"`
	ChannelStatusAfter  int    `json:"channel_status_after"`
	TestedAt            int64  `json:"tested_at" gorm:"bigint;index"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;index"`
}

func (XLLMGroupStabilitySample) TableName() string {
	return "xllm_group_stability_samples"
}

func (sample *XLLMGroupStabilitySample) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if sample.TestedAt == 0 {
		sample.TestedAt = now
	}
	if sample.CreatedAt == 0 {
		sample.CreatedAt = now
	}
	return nil
}

func RecordXLLMGroupStabilitySample(sample *XLLMGroupStabilitySample) error {
	if sample == nil || sample.ChannelID <= 0 || sample.GroupNames == "" {
		return nil
	}
	return DB.Create(sample).Error
}

func BatchRecordXLLMGroupStabilitySamples(samples []XLLMGroupStabilitySample) error {
	if len(samples) == 0 {
		return nil
	}
	return DB.CreateInBatches(samples, 100).Error
}

func GetXLLMGroupStabilitySamplesSince(startTs int64) ([]XLLMGroupStabilitySample, error) {
	var samples []XLLMGroupStabilitySample
	err := DB.Where("tested_at >= ?", startTs).
		Order("tested_at ASC, id ASC").
		Find(&samples).Error
	return samples, err
}

func GetXLLMGroupStabilityTaskIDsSince(startTs int64) (map[string]struct{}, error) {
	taskIDs := []string{}
	err := DB.Model(&XLLMGroupStabilitySample{}).
		Where("tested_at >= ? AND task_id <> ?", startTs, "").
		Distinct().
		Pluck("task_id", &taskIDs).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		result[taskID] = struct{}{}
	}
	return result, nil
}

func DeleteXLLMGroupStabilitySamplesBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where("tested_at < ?", cutoffTs).Delete(&XLLMGroupStabilitySample{}).Error
}
