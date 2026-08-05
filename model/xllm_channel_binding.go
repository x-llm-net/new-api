package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"gorm.io/gorm"
)

var (
	ErrXLLMChannelBindingNotFound = errors.New("xllm channel binding not found")
	ErrXLLMConfigVersionConflict  = errors.New("xllm config version conflict")
)

type XLLMChannelBinding struct {
	ID             int64  `json:"id" gorm:"primary_key"`
	SourceRef      string `json:"source_ref" gorm:"type:varchar(255);uniqueIndex;not null"`
	ChannelID      int    `json:"channel_id" gorm:"uniqueIndex;not null"`
	ConfigVersion  int    `json:"config_version" gorm:"not null"`
	ConfigChecksum string `json:"config_checksum" gorm:"type:varchar(128);not null"`
	Active         bool   `json:"active" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (XLLMChannelBinding) TableName() string {
	return "xllm_channel_bindings"
}

type XLLMRelayChannelInput struct {
	SourceRef         string
	ExternalChannelID string
	ConfigVersion     int
	ConfigChecksum    string
	Enabled           bool
	Group             string
	MultiplierBps     int
	BaseURL           string
	APIKey            string
	Models            map[string]string
}

type XLLMRelayChannelResult struct {
	ChannelID     int
	ConfigVersion int
	Applied       bool
}

func IsActiveXLLMRelayChannel(channelID int) (bool, error) {
	var count int64
	err := DB.Model(&XLLMChannelBinding{}).
		Where("channel_id = ? AND active = ?", channelID, true).
		Count(&count).Error
	return count == 1, err
}

func UpsertXLLMRelayChannel(input XLLMRelayChannelInput) (XLLMRelayChannelResult, error) {
	result := XLLMRelayChannelResult{}
	groupRatioJSON := ""
	err := DB.Transaction(func(tx *gorm.DB) error {
		binding := XLLMChannelBinding{}
		err := lockForUpdate(tx).
			Where("source_ref = ?", input.SourceRef).
			First(&binding).Error
		bindingExists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !bindingExists && !input.Enabled {
			return ErrXLLMChannelBindingNotFound
		}
		if bindingExists {
			result.ChannelID = binding.ChannelID
			result.ConfigVersion = binding.ConfigVersion
			if input.ExternalChannelID != "" && input.ExternalChannelID != fmt.Sprintf("%d", binding.ChannelID) {
				return ErrXLLMConfigVersionConflict
			}
			if input.ConfigVersion < binding.ConfigVersion {
				return nil
			}
			if input.ConfigVersion == binding.ConfigVersion {
				if input.Enabled && input.ConfigChecksum != binding.ConfigChecksum {
					return ErrXLLMConfigVersionConflict
				}
				return nil
			}
		}

		status := common.ChannelStatusManuallyDisabled
		if input.Enabled {
			status = common.ChannelStatusEnabled
		}
		channel := Channel{}
		if bindingExists {
			if err := lockForUpdate(tx).First(&channel, "id = ?", binding.ChannelID).Error; err != nil {
				return err
			}
		} else {
			channel = Channel{
				Type:        constant.ChannelTypeOpenAI,
				Name:        "LLMHub " + input.SourceRef,
				CreatedTime: common.GetTimestamp(),
			}
		}

		if input.Enabled {
			modelNames := make([]string, 0, len(input.Models))
			for modelName := range input.Models {
				modelNames = append(modelNames, modelName)
			}
			sort.Strings(modelNames)
			mappingJSON, err := common.Marshal(input.Models)
			if err != nil {
				return err
			}
			baseURL := strings.TrimRight(input.BaseURL, "/")
			baseURL = strings.TrimSuffix(baseURL, "/v1")
			modelMapping := string(mappingJSON)
			channel.Type = constant.ChannelTypeOpenAI
			channel.Name = "LLMHub " + input.SourceRef
			channel.Key = input.APIKey
			channel.Status = status
			channel.BaseURL = &baseURL
			channel.Models = strings.Join(modelNames, ",")
			channel.Group = input.Group
			channel.ModelMapping = &modelMapping
		} else {
			channel.Status = status
		}

		if bindingExists {
			if err := tx.Save(&channel).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Create(&channel).Error; err != nil {
				return err
			}
			binding = XLLMChannelBinding{
				SourceRef: input.SourceRef,
				ChannelID: channel.Id,
			}
		}
		if err := channel.UpdateAbilities(tx); err != nil {
			return err
		}

		binding.ConfigVersion = input.ConfigVersion
		if input.ConfigChecksum != "" {
			binding.ConfigChecksum = input.ConfigChecksum
		}
		binding.Active = input.Enabled
		if bindingExists {
			if err := tx.Save(&binding).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&binding).Error; err != nil {
			return err
		}

		if input.Enabled {
			ratioJSON, err := persistXLLMGroupRatio(tx, input.Group, input.MultiplierBps)
			if err != nil {
				return err
			}
			groupRatioJSON = ratioJSON
		}
		result = XLLMRelayChannelResult{
			ChannelID:     channel.Id,
			ConfigVersion: input.ConfigVersion,
			Applied:       true,
		}
		return nil
	})
	if err != nil {
		return XLLMRelayChannelResult{}, err
	}
	if groupRatioJSON != "" {
		if err := updateOptionMap("GroupRatio", groupRatioJSON); err != nil {
			return XLLMRelayChannelResult{}, err
		}
	} else {
		option := Option{}
		if err := DB.Where("key = ?", "GroupRatio").First(&option).Error; err == nil && option.Value != "" {
			if err := updateOptionMap("GroupRatio", option.Value); err != nil {
				return XLLMRelayChannelResult{}, err
			}
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return XLLMRelayChannelResult{}, err
		}
	}
	return result, nil
}

func persistXLLMGroupRatio(tx *gorm.DB, group string, multiplierBps int) (string, error) {
	ratios := ratio_setting.GetGroupRatioCopy()
	option := Option{}
	err := lockForUpdate(tx).Where("key = ?", "GroupRatio").First(&option).Error
	if err == nil && option.Value != "" {
		if err := common.Unmarshal([]byte(option.Value), &ratios); err != nil {
			return "", err
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	ratios[group] = float64(multiplierBps) / 10_000
	value, err := common.Marshal(ratios)
	if err != nil {
		return "", err
	}
	option.Key = "GroupRatio"
	option.Value = string(value)
	if err := tx.Save(&option).Error; err != nil {
		return "", err
	}
	return option.Value, nil
}
