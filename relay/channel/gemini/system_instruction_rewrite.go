package gemini

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type systemInstructionRewriteRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

const (
	maxSystemRewriteConfigBytes = 64 << 10
	maxSystemRewriteRules       = 32
	maxSystemRewritePhraseBytes = 4096
)

// Built-in rules remain the startup default when no valid file has been loaded.
var geminiSystemInstructionRewriteRules = []systemInstructionRewriteRule{
	{
		From: "deep literary foundation",
		To:   "profound literary background",
	},
	{
		From: "本提示词的说明文字是中文，但你的全部输出（narrator、char、npc 的 text 正文）必须是泰语。",
		To:   "本提示词是中文，你的全部输出（narrator、char 和 npc 的 text 正文）必须是泰语。",
	},
	{
		From: "本提示词的说明文字是中文，但你的全部输出（narrator、char 和 npc 的 text 正文）必须是泰语。",
		To:   "本提示词是中文，你的全部输出（narrator、char 和 npc 的 text 正文）必须是泰语。",
	},
}

var (
	activeSystemInstructionRewriteRules atomic.Pointer[[]systemInstructionRewriteRule]
	systemInstructionRewriteStart       sync.Once
)

type systemInstructionRewriteLoader struct {
	path         string
	lastData     []byte
	lastParseErr error
}

// StartSystemInstructionRewriteReload starts one watcher; requests only read its
// immutable memory snapshot. A valid file replaces the entire built-in table.
func StartSystemInstructionRewriteReload() {
	systemInstructionRewriteStart.Do(func() {
		loader := systemInstructionRewriteLoader{
			path: common.GetEnvOrDefaultString("GEMINI_SYSTEM_REWRITE_CONFIG", "/data/gemini-system-rewrites.json"),
		}
		lastError := ""
		reload := func() {
			if err := loader.reload(); err != nil {
				if err.Error() != lastError {
					common.SysError("Gemini system instruction rewrite config unchanged: " + err.Error())
				}
				lastError = err.Error()
				return
			}
			lastError = ""
		}
		reload()
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				reload()
			}
		}()
	})
}

func (l *systemInstructionRewriteLoader) reload() error {
	file, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) && activeSystemInstructionRewriteRules.Load() == nil {
			return nil
		}
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSystemRewriteConfigBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxSystemRewriteConfigBytes {
		return fmt.Errorf("rewrite config exceeds %d bytes", maxSystemRewriteConfigBytes)
	}
	if l.lastData != nil && bytes.Equal(data, l.lastData) {
		return l.lastParseErr
	}
	l.lastData = data
	rules, err := parseSystemInstructionRewriteRules(data)
	l.lastParseErr = err
	if err != nil {
		return err
	}
	activeSystemInstructionRewriteRules.Store(&rules)
	common.SysLog(fmt.Sprintf("Gemini system instruction rewrite config loaded: path=%s rules=%d", l.path, len(rules)))
	return nil
}

func parseSystemInstructionRewriteRules(data []byte) ([]systemInstructionRewriteRule, error) {
	var config struct {
		Enabled *bool                          `json:"enabled"`
		Rules   []systemInstructionRewriteRule `json:"rules"`
	}
	if err := common.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid rewrite config JSON: %w", err)
	}
	if config.Enabled == nil {
		return nil, fmt.Errorf("rewrite config requires enabled")
	}
	if !*config.Enabled {
		return nil, nil
	}
	if config.Rules == nil || len(config.Rules) > maxSystemRewriteRules {
		return nil, fmt.Errorf("rewrite config requires an array of at most %d rules", maxSystemRewriteRules)
	}
	seen := make(map[string]bool, len(config.Rules))
	for i, rule := range config.Rules {
		if strings.TrimSpace(rule.From) == "" || strings.TrimSpace(rule.To) == "" ||
			len(rule.From) > maxSystemRewritePhraseBytes || len(rule.To) > maxSystemRewritePhraseBytes {
			return nil, fmt.Errorf("rewrite rule %d requires nonempty from/to of at most %d bytes", i+1, maxSystemRewritePhraseBytes)
		}
		if seen[rule.From] {
			return nil, fmt.Errorf("rewrite rule %d duplicates a source phrase", i+1)
		}
		seen[rule.From] = true
	}
	return config.Rules, nil
}

func rewriteGeminiSystemInstructions(c *gin.Context, request *dto.GeminiChatRequest) {
	if request == nil || request.SystemInstructions == nil {
		return
	}
	rules := geminiSystemInstructionRewriteRules
	if snapshot := activeSystemInstructionRewriteRules.Load(); snapshot != nil {
		rules = *snapshot
	}
	if len(rules) == 0 {
		return
	}

	for i := range request.SystemInstructions.Parts {
		part := &request.SystemInstructions.Parts[i]
		original := part.Text
		if original == "" {
			continue
		}

		rewritten := original
		matchedRules := make([]string, 0, len(rules))
		for _, rule := range rules {
			if strings.Contains(rewritten, rule.From) {
				rewritten = strings.ReplaceAll(rewritten, rule.From, rule.To)
				matchedRules = append(matchedRules, rule.From)
			}
		}

		if rewritten == original {
			continue
		}

		part.Text = rewritten
		logger.LogDebug(c, "Gemini system instruction rewrite: rules=%q original=%q rewritten=%q", matchedRules, original, rewritten)
	}
}
