package controller

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type xllmRelayChannelRequest struct {
	SourceRef         string            `json:"sourceRef"`
	ExternalChannelID string            `json:"externalChannelId"`
	ConfigVersion     int               `json:"configVersion"`
	ConfigChecksum    string            `json:"configChecksum"`
	Enabled           bool              `json:"enabled"`
	Group             string            `json:"group"`
	MultiplierBps     int               `json:"multiplierBps"`
	BaseURL           string            `json:"baseUrl"`
	APIKey            string            `json:"apiKey"`
	Models            map[string]string `json:"models"`
}

func UpsertXLLMRelayChannel(c *gin.Context) {
	request := xllmRelayChannelRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		xllmRelayError(c, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	if err := validateXLLMRelayChannelRequest(c.Param("sourceRef"), request); err != nil {
		xllmRelayError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result, err := model.UpsertXLLMRelayChannel(model.XLLMRelayChannelInput{
		SourceRef:         request.SourceRef,
		ExternalChannelID: request.ExternalChannelID,
		ConfigVersion:     request.ConfigVersion,
		ConfigChecksum:    request.ConfigChecksum,
		Enabled:           request.Enabled,
		Group:             request.Group,
		MultiplierBps:     request.MultiplierBps,
		BaseURL:           request.BaseURL,
		APIKey:            request.APIKey,
		Models:            request.Models,
	})
	if err != nil {
		switch {
		case errors.Is(err, model.ErrXLLMChannelBindingNotFound):
			xllmRelayError(c, http.StatusNotFound, "binding_not_found", "Channel binding not found")
		case errors.Is(err, model.ErrXLLMConfigVersionConflict):
			xllmRelayError(c, http.StatusConflict, "config_conflict", "Config version conflicts with the applied channel")
		default:
			common.SysError("xllm relay channel sync failed: " + err.Error())
			xllmRelayError(c, http.StatusInternalServerError, "internal_error", "Channel sync failed")
		}
		return
	}

	model.InitChannelCache()
	service.ResetProxyClientCache()
	c.JSON(http.StatusOK, gin.H{
		"id":            strconv.Itoa(result.ChannelID),
		"configVersion": result.ConfigVersion,
		"applied":       result.Applied,
	})
}

func validateXLLMRelayChannelRequest(pathSourceRef string, request xllmRelayChannelRequest) error {
	if request.SourceRef == "" || request.SourceRef != pathSourceRef || len(request.SourceRef) > 255 || !strings.HasPrefix(request.SourceRef, "llmhub:") {
		return errors.New("sourceRef does not match the request path")
	}
	if request.ConfigVersion <= 0 {
		return errors.New("configVersion must be positive")
	}
	if !request.Enabled {
		channelID, err := strconv.Atoi(request.ExternalChannelID)
		if err != nil || channelID <= 0 {
			return errors.New("externalChannelId must identify the existing channel")
		}
		return nil
	}
	if request.ConfigChecksum == "" || len(request.ConfigChecksum) > 128 {
		return errors.New("configChecksum is required")
	}
	if request.Group == "" || len(request.Group) > 64 || !strings.HasPrefix(request.Group, "lhg_") {
		return errors.New("group must be an LLMHub relay group")
	}
	if request.MultiplierBps < 0 || request.MultiplierBps > 1_000_000 {
		return errors.New("multiplierBps is out of range")
	}
	parsedURL, err := url.ParseRequestURI(request.BaseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return errors.New("baseUrl must be an absolute HTTP URL")
	}
	if request.APIKey == "" || len(request.APIKey) > 4096 {
		return errors.New("apiKey is required")
	}
	if len(request.Models) == 0 || len(request.Models) > 1000 {
		return errors.New("models must contain between 1 and 1000 entries")
	}
	for canonical, upstream := range request.Models {
		if canonical == "" || upstream == "" || len(canonical) > 256 || len(upstream) > 256 {
			return errors.New("models contains an invalid model name")
		}
	}
	return nil
}

func xllmRelayError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
