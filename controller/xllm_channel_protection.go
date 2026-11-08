package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// A dedicated, revocable credential grants only this operation, not dashboard
// access. Configuration is read on every call; absent/invalid config fails closed.
type xllmProtectionConfig struct {
	Enabled       bool                     `json:"enabled"`
	ChannelGroup  string                   `json:"channel_group,omitempty"`
	ChannelGroups []string                 `json:"channel_groups,omitempty"`
	SampleGroups  []string                 `json:"sample_groups"`
	Interval      int64                    `json:"interval"`
	Window        int64                    `json:"window"`
	SlowMS        float64                  `json:"slow_ms"`
	SlowRate      float64                  `json:"slow_rate"`
	MinSamples    int                      `json:"min_samples"`
	Fallbacks     []xllmProtectionFallback `json:"fallbacks"`
}

type xllmProtectionFallback struct {
	Name       string `json:"name"`
	Channels   []int  `json:"channels"`
	MinEnabled int    `json:"min_enabled"`
}

type xllmProtectionChannel struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     int    `json:"status"`
	StatusTime int64  `json:"status_time"`
}

type xllmProtectionEvidence struct {
	ChannelID   int   `json:"channel_id"`
	StatusTime  int64 `json:"status_time"`
	Total       int   `json:"total"`
	Slow        int   `json:"slow"`
	Unknown     int   `json:"unknown"`
	WindowStart int64 `json:"window_start"`
	WindowEnd   int64 `json:"window_end"`
}

var xllmProtectionLock sync.Mutex

func xllmProtectionSettings(c *gin.Context) (xllmProtectionConfig, string, bool) {
	var config xllmProtectionConfig
	path := os.Getenv("XLLM_CHANNEL_PROTECTION_CONFIG")
	if path == "" {
		path = "/data/xllm-channel-protection.json"
	}
	key, keyErr := os.ReadFile(path + ".key")
	provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	expectedHash, suppliedHash := sha256.Sum256([]byte(strings.TrimSpace(string(key)))), sha256.Sum256([]byte(provided))
	if keyErr != nil || len(strings.TrimSpace(string(key))) < 32 || !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(expectedHash[:], suppliedHash[:]) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
		return config, "", false
	}
	body, err := os.ReadFile(path)
	if err == nil {
		err = common.Unmarshal(body, &config)
	}
	// An explicit list takes precedence; keep existing single-group policies valid.
	if len(config.ChannelGroups) == 0 && config.ChannelGroup != "" {
		config.ChannelGroups = []string{config.ChannelGroup}
	}
	if err != nil || !validXLLMProtectionConfig(config) {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "channel protection configuration unavailable"})
		return config, "", false
	}
	version := fmt.Sprintf("%x", sha256.Sum256(body))
	c.Header("Cache-Control", "no-store")
	return config, version, true
}

func validXLLMProtectionConfig(c xllmProtectionConfig) bool {
	if len(c.ChannelGroups) == 0 || len(c.SampleGroups) == 0 || c.Interval < 60 || c.Interval > 3600 || c.Window < 60 || c.Window > 900 || c.MinSamples < 20 || c.SlowMS <= 0 || math.IsNaN(c.SlowMS) || math.IsInf(c.SlowMS, 0) || c.SlowRate <= 0 || c.SlowRate >= 1 || math.IsNaN(c.SlowRate) || len(c.Fallbacks) == 0 {
		return false
	}
	for _, group := range c.SampleGroups {
		if strings.TrimSpace(group) == "" {
			return false
		}
	}
	for _, group := range c.ChannelGroups {
		if strings.TrimSpace(group) == "" {
			return false
		}
	}
	for _, f := range c.Fallbacks {
		ids := map[int]bool{}
		for _, id := range f.Channels {
			if id <= 0 || ids[id] {
				return false
			}
			ids[id] = true
		}
		if f.Name == "" || f.MinEnabled < 1 || f.MinEnabled > len(ids) {
			return false
		}
	}
	return true
}

func xllmProtectionChannels(groups []string) ([]xllmProtectionChannel, error) {
	var channels []model.Channel
	// Never select or expose channel keys, headers, URLs, or other secrets.
	if err := model.DB.Select("id", "name", "status", "group", "other_info").Find(&channels).Error; err != nil {
		return nil, err
	}
	result := make([]xllmProtectionChannel, 0)
	for _, ch := range channels {
		inScope := false
		for group := range strings.SplitSeq(ch.Group, ",") {
			if slices.Contains(groups, group) {
				inScope = true
				break
			}
		}
		if !inScope {
			continue
		}
		changed, _ := ch.GetOtherInfo()["status_time"].(float64)
		result = append(result, xllmProtectionChannel{ch.Id, ch.Name, ch.Status, int64(changed)})
	}
	return result, nil
}

// GetXLLMChannelProtection supplies the collector with one authoritative policy
// and a non-secret status snapshot, including recovery timestamps.
func GetXLLMChannelProtection(c *gin.Context) {
	config, version, ok := xllmProtectionSettings(c)
	if !ok {
		return
	}
	channels, err := xllmProtectionChannels(config.ChannelGroups)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "config": config, "version": version, "channels": channels, "now": time.Now().Unix()})
}

func ApplyXLLMChannelProtection(c *gin.Context) {
	config, version, ok := xllmProtectionSettings(c)
	if !ok {
		return
	}
	var req struct {
		Version    string                   `json:"version"`
		Candidates []xllmProtectionEvidence `json:"candidates"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32768)
	if c.ShouldBindJSON(&req) != nil || len(req.Candidates) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	if !config.Enabled || req.Version != version {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "policy changed or disabled"})
		return
	}
	xllmProtectionLock.Lock()
	defer xllmProtectionLock.Unlock()
	results := make([]gin.H, 0, len(req.Candidates))
	for _, e := range req.Candidates {
		now := time.Now().Unix()
		if e.Total < config.MinSamples || e.Unknown < 0 || e.Unknown > e.Total || e.Total-e.Unknown < config.MinSamples || e.Slow < 0 || e.Slow > e.Total-e.Unknown || float64(e.Unknown)/float64(e.Total) > .05 || float64(e.Slow)/float64(e.Total) <= config.SlowRate || e.WindowEnd > now || e.WindowEnd < now-120 || e.WindowStart < e.WindowEnd-config.Window || e.WindowStart >= e.WindowEnd || e.WindowStart <= e.StatusTime {
			results = append(results, gin.H{"channel_id": e.ChannelID, "result": "invalid_evidence"})
			continue
		}
		channels, err := xllmProtectionChannels(config.ChannelGroups)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "results": results})
			return
		}
		idx := slices.IndexFunc(channels, func(ch xllmProtectionChannel) bool { return ch.ID == e.ChannelID })
		if idx < 0 || channels[idx].Status != common.ChannelStatusEnabled || channels[idx].StatusTime != e.StatusTime {
			results = append(results, gin.H{"channel_id": e.ChannelID, "result": "channel_changed"})
			continue
		}
		blocked := ""
		for _, f := range config.Fallbacks {
			if !slices.Contains(f.Channels, e.ChannelID) {
				continue
			}
			remaining := 0
			for _, ch := range channels {
				if ch.ID != e.ChannelID && ch.Status == common.ChannelStatusEnabled && slices.Contains(f.Channels, ch.ID) {
					remaining++
				}
			}
			if remaining < f.MinEnabled {
				blocked = f.Name
				break
			}
		}
		if blocked != "" {
			results = append(results, gin.H{"channel_id": e.ChannelID, "result": "fallback_retained", "fallback": blocked})
			continue
		}
		reason := fmt.Sprintf("xllm-channel-protection: %.1f%% (%d/%d) over %.0fms", float64(e.Slow)*100/float64(e.Total), e.Slow, e.Total, config.SlowMS)
		changed := model.AutoDisableChannelIfCurrent(e.ChannelID, e.StatusTime, reason)
		if !changed {
			model.InitChannelCache()
			results = append(results, gin.H{"channel_id": e.ChannelID, "result": "channel_changed"})
			continue
		}
		model.InitChannelCache()
		closeActiveChannelWebSockets([]int{e.ChannelID})
		model.RecordAuditLog(c, model.AuditLog{Username: "xllm-channel-protection", ActorRole: common.RoleAdminUser, AuthMethod: "monitor_key", Category: model.AuditCategoryOperation, Action: "channel.protection_auto_disable", Success: true, Content: fmt.Sprintf("channel=%d window=%d-%d %s", e.ChannelID, e.WindowStart, e.WindowEnd, reason)})
		common.SysLog(reason + fmt.Sprintf(" channel=%d", e.ChannelID))
		results = append(results, gin.H{"channel_id": e.ChannelID, "result": "disabled", "at": time.Now().Unix()})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "results": results})
}
