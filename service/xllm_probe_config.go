package service

import (
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const xllmHomeMonitorConfigEnv = "XLLM_HOME_MONITOR_CONFIG"
const xllmHomeMonitorDefaultConfigPath = "config/xllm-home-monitor.json"

type XLLMHomeMonitorEntry struct {
	GroupName   string `json:"group"`
	ModelName   string `json:"model"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

type xllmHomeMonitorConfig struct {
	Enabled         bool                   `json:"enabled"`
	IntervalMinutes float64                `json:"interval_minutes"`
	Entries         []XLLMHomeMonitorEntry `json:"entries"`
}

var xllmHomeMonitorConfigCache = struct {
	sync.Mutex
	path    string
	modTime time.Time
	size    int64
	config  xllmHomeMonitorConfig
	entries []XLLMHomeMonitorEntry
}{}

func GetXLLMHomeMonitorConfig() xllmHomeMonitorConfig {
	configPath := common.GetEnvOrDefaultString(xllmHomeMonitorConfigEnv, xllmHomeMonitorDefaultConfigPath)
	stat, err := os.Stat(configPath)
	if err != nil {
		return xllmHomeMonitorConfig{}
	}

	xllmHomeMonitorConfigCache.Lock()
	defer xllmHomeMonitorConfigCache.Unlock()
	if xllmHomeMonitorConfigCache.path == configPath &&
		xllmHomeMonitorConfigCache.modTime.Equal(stat.ModTime()) &&
		xllmHomeMonitorConfigCache.size == stat.Size() {
		return cloneXLLMHomeMonitorConfig(xllmHomeMonitorConfigCache.config)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		xllmHomeMonitorConfigCache.path = configPath
		xllmHomeMonitorConfigCache.modTime = stat.ModTime()
		xllmHomeMonitorConfigCache.size = stat.Size()
		xllmHomeMonitorConfigCache.config = xllmHomeMonitorConfig{}
		xllmHomeMonitorConfigCache.entries = nil
		return xllmHomeMonitorConfig{}
	}

	config := xllmHomeMonitorConfig{}
	if err := common.Unmarshal(content, &config); err != nil {
		common.SysLog("failed to parse X-LLM home monitor config: " + err.Error())
		xllmHomeMonitorConfigCache.path = configPath
		xllmHomeMonitorConfigCache.modTime = stat.ModTime()
		xllmHomeMonitorConfigCache.size = stat.Size()
		xllmHomeMonitorConfigCache.config = xllmHomeMonitorConfig{}
		xllmHomeMonitorConfigCache.entries = nil
		return xllmHomeMonitorConfig{}
	}
	config.Entries = normalizeXLLMHomeMonitorEntries(config.Entries)
	xllmHomeMonitorConfigCache.path = configPath
	xllmHomeMonitorConfigCache.modTime = stat.ModTime()
	xllmHomeMonitorConfigCache.size = stat.Size()
	xllmHomeMonitorConfigCache.config = config
	xllmHomeMonitorConfigCache.entries = config.Entries
	return cloneXLLMHomeMonitorConfig(config)
}

func GetXLLMHomeMonitorEntries() []XLLMHomeMonitorEntry {
	config := GetXLLMHomeMonitorConfig()
	return append([]XLLMHomeMonitorEntry(nil), config.Entries...)
}

func XLLMHomeMonitorEnabled() bool {
	config := GetXLLMHomeMonitorConfig()
	return config.Enabled && len(config.Entries) > 0
}

func XLLMHomeMonitorInterval() time.Duration {
	config := GetXLLMHomeMonitorConfig()
	minutes := config.IntervalMinutes
	if minutes <= 0 {
		minutes = 5
	}
	return time.Duration(minutes * float64(time.Minute))
}

func cloneXLLMHomeMonitorConfig(config xllmHomeMonitorConfig) xllmHomeMonitorConfig {
	config.Entries = append([]XLLMHomeMonitorEntry(nil), config.Entries...)
	return config
}

func normalizeXLLMHomeMonitorEntries(entries []XLLMHomeMonitorEntry) []XLLMHomeMonitorEntry {
	normalized := make([]XLLMHomeMonitorEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		entry.GroupName = strings.TrimSpace(entry.GroupName)
		entry.ModelName = strings.TrimSpace(entry.ModelName)
		entry.DisplayName = strings.TrimSpace(entry.DisplayName)
		if !entry.Enabled || entry.GroupName == "" || entry.ModelName == "" {
			continue
		}
		if entry.DisplayName == "" {
			entry.DisplayName = entry.GroupName
		}
		key := xllmHomeMonitorEntryConfigKey(entry.GroupName, entry.ModelName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, entry)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		if normalized[i].GroupName != normalized[j].GroupName {
			return normalized[i].GroupName < normalized[j].GroupName
		}
		return normalized[i].ModelName < normalized[j].ModelName
	})
	return normalized
}

func xllmHomeMonitorEntryConfigKey(groupName string, modelName string) string {
	return strings.ToLower(strings.TrimSpace(groupName)) + "\x00" + strings.ToLower(strings.TrimSpace(modelName))
}
