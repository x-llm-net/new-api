package model

// XLLMProbeSample stores one homepage monitor probe result. It is intentionally
// X-LLM-owned so upstream channel-test behavior can change without losing
// homepage history.
type XLLMProbeSample struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	ProbeVersion      int    `json:"probe_version" gorm:"not null;default:3;index:idx_xllm_probe_sample_lookup_v3,priority:1"`
	TaskID            string `json:"task_id" gorm:"size:64;index"`
	SelectedChannelID int    `json:"selected_channel_id" gorm:"index"`
	GroupName         string `json:"group_name" gorm:"size:64;not null;index:idx_xllm_probe_sample_lookup_v3,priority:2"`
	ModelName         string `json:"model_name" gorm:"size:128;not null;index:idx_xllm_probe_sample_lookup_v3,priority:3"`
	Status            string `json:"status" gorm:"size:32;index"`
	ErrorCode         string `json:"error_code" gorm:"size:128"`
	TTFTMs            *int64 `json:"ttft_ms" gorm:"column:ttft_ms"`
	TotalMs           int64  `json:"total_ms"`
	TestedAt          int64  `json:"tested_at" gorm:"index:idx_xllm_probe_sample_lookup_v3,priority:4;index"`
	CreatedAt         int64  `json:"created_at"`
}

func (XLLMProbeSample) TableName() string {
	return "xllm_probe_samples"
}

func CreateXLLMProbeSample(sample *XLLMProbeSample) error {
	if sample == nil {
		return nil
	}
	return DB.Create(sample).Error
}

func GetLatestXLLMProbeSample(probeVersion int, groupName string, modelName string) (*XLLMProbeSample, error) {
	var sample XLLMProbeSample
	err := DB.Where("probe_version = ? AND group_name = ? AND model_name = ?", probeVersion, groupName, modelName).
		Order("tested_at desc, id desc").
		First(&sample).Error
	if err != nil {
		return nil, err
	}
	return &sample, nil
}

func GetXLLMProbeSamples(probeVersion int, startTs int64, endTs int64, entries []XLLMProbeEntryKey) ([]XLLMProbeSample, error) {
	samples := make([]XLLMProbeSample, 0)
	if startTs > endTs || len(entries) == 0 {
		return samples, nil
	}
	query := DB.Model(&XLLMProbeSample{}).
		Where("probe_version = ? AND tested_at >= ? AND tested_at <= ?", probeVersion, startTs, endTs)
	query = applyXLLMProbeEntryFilter(query, entries)
	err := query.Order("tested_at asc").Find(&samples).Error
	return samples, err
}
