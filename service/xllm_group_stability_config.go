package service

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const defaultXLLMGroupStabilityConfigPath = "data/xllm-group-stability.json"

type XLLMGroupStabilityConfig struct {
	Enabled *bool                           `json:"enabled,omitempty"`
	Groups  []XLLMGroupStabilityGroupConfig `json:"groups"`
}

type XLLMGroupStabilityGroupConfig struct {
	Group       string `json:"group"`
	DisplayName string `json:"display_name"`
	Enabled     *bool  `json:"enabled,omitempty"`
	SortOrder   int    `json:"sort_order"`
}

var xllmGroupStabilityConfigCache = struct {
	sync.Mutex
	path    string
	modTime time.Time
	size    int64
	config  XLLMGroupStabilityConfig
	loaded  bool
}{}

func LoadXLLMGroupStabilityConfig() (XLLMGroupStabilityConfig, error) {
	path := xllmGroupStabilityConfigPath()
	stat, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return XLLMGroupStabilityConfig{Enabled: boolPtr(false)}, nil
		}
		return XLLMGroupStabilityConfig{}, err
	}

	xllmGroupStabilityConfigCache.Lock()
	defer xllmGroupStabilityConfigCache.Unlock()

	if xllmGroupStabilityConfigCache.loaded &&
		xllmGroupStabilityConfigCache.path == path &&
		xllmGroupStabilityConfigCache.modTime.Equal(stat.ModTime()) &&
		xllmGroupStabilityConfigCache.size == stat.Size() {
		return xllmGroupStabilityConfigCache.config, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return XLLMGroupStabilityConfig{}, err
	}
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	config := XLLMGroupStabilityConfig{}
	if err := common.Unmarshal(content, &config); err != nil {
		return XLLMGroupStabilityConfig{}, err
	}
	config = normalizeXLLMGroupStabilityConfig(config)

	xllmGroupStabilityConfigCache.path = path
	xllmGroupStabilityConfigCache.modTime = stat.ModTime()
	xllmGroupStabilityConfigCache.size = stat.Size()
	xllmGroupStabilityConfigCache.config = config
	xllmGroupStabilityConfigCache.loaded = true

	return config, nil
}

func xllmGroupStabilityConfigPath() string {
	path := strings.TrimSpace(common.GetEnvOrDefaultString("XLLM_GROUP_STABILITY_CONFIG", defaultXLLMGroupStabilityConfigPath))
	if path == "" {
		path = defaultXLLMGroupStabilityConfigPath
	}
	return filepath.Clean(path)
}

func normalizeXLLMGroupStabilityConfig(config XLLMGroupStabilityConfig) XLLMGroupStabilityConfig {
	seen := map[string]struct{}{}
	groups := make([]XLLMGroupStabilityGroupConfig, 0, len(config.Groups))
	for _, group := range config.Groups {
		group.Group = strings.TrimSpace(group.Group)
		group.DisplayName = strings.TrimSpace(group.DisplayName)
		if group.Group == "" || !xllmGroupStabilityConfigEntryEnabled(group.Enabled) {
			continue
		}
		if _, ok := seen[group.Group]; ok {
			continue
		}
		if group.DisplayName == "" {
			group.DisplayName = group.Group
		}
		seen[group.Group] = struct{}{}
		groups = append(groups, group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].SortOrder == groups[j].SortOrder {
			return groups[i].Group < groups[j].Group
		}
		return groups[i].SortOrder < groups[j].SortOrder
	})
	config.Groups = groups
	return config
}

func xllmGroupStabilityConfigEnabled(config XLLMGroupStabilityConfig) bool {
	return xllmGroupStabilityConfigEntryEnabled(config.Enabled) && len(config.Groups) > 0
}

func xllmGroupStabilityConfigEntryEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func boolPtr(value bool) *bool {
	return &value
}
