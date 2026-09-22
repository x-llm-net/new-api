package gemini

import (
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type systemInstructionRewriteRule struct {
	from string
	to   string
}

// Changes to this startup-loaded table require a service restart.
var geminiSystemInstructionRewriteRules = []systemInstructionRewriteRule{
	{
		from: "deep literary foundation",
		to:   "profound literary background",
	},
}

func rewriteGeminiSystemInstructions(c *gin.Context, request *dto.GeminiChatRequest) {
	if request == nil || request.SystemInstructions == nil {
		return
	}

	for i := range request.SystemInstructions.Parts {
		part := &request.SystemInstructions.Parts[i]
		original := part.Text
		if original == "" {
			continue
		}

		rewritten := original
		matchedRules := make([]string, 0, len(geminiSystemInstructionRewriteRules))
		for _, rule := range geminiSystemInstructionRewriteRules {
			if strings.Contains(rewritten, rule.from) {
				rewritten = strings.ReplaceAll(rewritten, rule.from, rule.to)
				matchedRules = append(matchedRules, rule.from)
			}
		}

		if rewritten == original {
			continue
		}

		part.Text = rewritten
		logger.LogDebug(c, "Gemini system instruction rewrite: rules=%q original=%q rewritten=%q", matchedRules, original, rewritten)
	}
}
