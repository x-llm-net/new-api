package controller

import (
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	modelMonitorStatusOperational = "operational"
	modelMonitorStatusDegraded    = "degraded"
	modelMonitorStatusUnavailable = "unavailable"
	modelMonitorStatusUnknown     = "unknown"

	modelMonitorHistoryLimit  = 120
	minModelMonitorStaleAfter = int64(15 * 60)
)

type modelMonitorRouteSummary struct {
	Status       int   `json:"status"`
	TestTime     int64 `json:"test_time"`
	ResponseTime int   `json:"response_time"`
}

type publicModelMonitorModel struct {
	ModelName         string `json:"model_name"`
	Status            string `json:"status"`
	AvailableRoutes   int    `json:"available_routes"`
	TotalRoutes       int    `json:"total_routes"`
	TestedRoutes      int    `json:"tested_routes"`
	UnavailableRoutes int    `json:"unavailable_routes"`
	UnknownRoutes     int    `json:"unknown_routes"`
	MinLatencyMs      int    `json:"min_latency_ms"`
	AvgLatencyMs      int    `json:"avg_latency_ms"`
	LastTestTime      int64  `json:"last_test_time"`
}

type publicModelMonitorGroup struct {
	GroupName         string                     `json:"group_name"`
	Status            string                     `json:"status"`
	AvailableRoutes   int                        `json:"available_routes"`
	TotalRoutes       int                        `json:"total_routes"`
	TestedRoutes      int                        `json:"tested_routes"`
	UnavailableRoutes int                        `json:"unavailable_routes"`
	UnknownRoutes     int                        `json:"unknown_routes"`
	ModelCount        int                        `json:"model_count"`
	ModelNames        []string                   `json:"model_names"`
	Routes            []modelMonitorRouteSummary `json:"routes"`
	MinLatencyMs      int                        `json:"min_latency_ms"`
	AvgLatencyMs      int                        `json:"avg_latency_ms"`
	LastTestTime      int64                      `json:"last_test_time"`
}

type publicModelMonitorTask struct {
	Status    model.SystemTaskStatus `json:"status"`
	Result    any                    `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
	CreatedAt int64                  `json:"created_at"`
	UpdatedAt int64                  `json:"updated_at"`
}

type publicModelMonitorProbe struct {
	Status      string                 `json:"status"`
	TaskStatus  model.SystemTaskStatus `json:"task_status"`
	SuccessRate float64                `json:"success_rate"`
	Tested      int                    `json:"tested"`
	Succeeded   int                    `json:"succeeded"`
	Failed      int                    `json:"failed"`
	Disabled    int                    `json:"disabled"`
	Enabled     int                    `json:"enabled"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

type publicModelMonitorSummary struct {
	Models            []publicModelMonitorModel `json:"models"`
	Groups            []publicModelMonitorGroup `json:"groups"`
	History           []publicModelMonitorProbe `json:"history"`
	TotalModels       int                       `json:"total_models"`
	OperationalModels int                       `json:"operational_models"`
	DegradedModels    int                       `json:"degraded_models"`
	UnavailableModels int                       `json:"unavailable_models"`
	UnknownModels     int                       `json:"unknown_models"`
	AvailableRoutes   int                       `json:"available_routes"`
	TotalRoutes       int                       `json:"total_routes"`
	LastTestTime      int64                     `json:"last_test_time"`
	StaleAfterSeconds int64                     `json:"stale_after_seconds"`
	LatestChannelTest *publicModelMonitorTask   `json:"latest_channel_test,omitempty"`
}

type modelMonitorGroupAccumulator struct {
	Routes     []modelMonitorRouteSummary
	ModelNames map[string]struct{}
}

func GetModelMonitorSummary(c *gin.Context) {
	var channels []*model.Channel
	if err := model.DB.Omit("key", "base_url", "balance").Find(&channels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	latestTask, err := model.GetLatestSystemTask(model.SystemTaskTypeChannelTest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	history, err := getModelMonitorProbeHistory(modelMonitorHistoryLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	monitorSetting := operation_setting.GetMonitorSetting()
	channels = selectChannelsForAutomaticTest(channels, monitorSetting.ChannelTestMode)
	summary := buildModelMonitorSummary(common.GetTimestamp(), modelMonitorStaleAfterSeconds(), channels, latestTask, history)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

func modelMonitorStaleAfterSeconds() int64 {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	seconds := int64(minutes * 3 * 60)
	if seconds < minModelMonitorStaleAfter {
		return minModelMonitorStaleAfter
	}
	return seconds
}

func getModelMonitorProbeHistory(limit int) ([]publicModelMonitorProbe, error) {
	if limit <= 0 {
		limit = modelMonitorHistoryLimit
	}
	var tasks []*model.SystemTask
	if err := model.DB.Where("type = ?", model.SystemTaskTypeChannelTest).
		Order("id desc").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	history := make([]publicModelMonitorProbe, 0, len(tasks))
	for i := len(tasks) - 1; i >= 0; i-- {
		history = append(history, buildModelMonitorProbe(tasks[i]))
	}
	return history, nil
}

func buildModelMonitorSummary(now int64, staleAfterSeconds int64, channels []*model.Channel, latestTask *model.SystemTask, history []publicModelMonitorProbe) publicModelMonitorSummary {
	routesByModel := map[string][]modelMonitorRouteSummary{}
	groupsByName := map[string]*modelMonitorGroupAccumulator{}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		modelNames := channel.GetModels()
		groupNames := channel.GetGroups()
		if len(groupNames) == 0 {
			groupNames = []string{"default"}
		}
		route := modelMonitorRouteSummary{
			Status:       channel.Status,
			TestTime:     channel.TestTime,
			ResponseTime: channel.ResponseTime,
		}

		for _, modelName := range modelNames {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			routesByModel[modelName] = append(routesByModel[modelName], route)
		}

		for _, groupName := range groupNames {
			groupName = strings.TrimSpace(groupName)
			if groupName == "" {
				groupName = "default"
			}
			group, ok := groupsByName[groupName]
			if !ok {
				group = &modelMonitorGroupAccumulator{
					ModelNames: map[string]struct{}{},
				}
				groupsByName[groupName] = group
			}
			group.Routes = append(group.Routes, route)
			for _, modelName := range modelNames {
				modelName = strings.TrimSpace(modelName)
				if modelName != "" {
					group.ModelNames[modelName] = struct{}{}
				}
			}
		}
	}

	summary := publicModelMonitorSummary{
		StaleAfterSeconds: staleAfterSeconds,
		History:           history,
	}
	if latestTask != nil {
		task := latestTask.ToResponse()
		summary.LatestChannelTest = &publicModelMonitorTask{
			Status:    task.Status,
			Result:    task.Result,
			Error:     task.Error,
			CreatedAt: task.CreatedAt,
			UpdatedAt: task.UpdatedAt,
		}
	}

	models := make([]publicModelMonitorModel, 0, len(routesByModel))
	for modelName, routes := range routesByModel {
		item := buildModelMonitorModel(modelName, routes, now, staleAfterSeconds)
		models = append(models, item)
		if item.LastTestTime > summary.LastTestTime {
			summary.LastTestTime = item.LastTestTime
		}
		switch item.Status {
		case modelMonitorStatusOperational:
			summary.OperationalModels++
		case modelMonitorStatusDegraded:
			summary.DegradedModels++
		case modelMonitorStatusUnavailable:
			summary.UnavailableModels++
		default:
			summary.UnknownModels++
		}
	}

	sort.Slice(models, func(i, j int) bool {
		leftPriority := modelMonitorStatusPriority(models[i].Status)
		rightPriority := modelMonitorStatusPriority(models[j].Status)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if models[i].AvailableRoutes != models[j].AvailableRoutes {
			return models[i].AvailableRoutes > models[j].AvailableRoutes
		}
		if models[i].LastTestTime != models[j].LastTestTime {
			return models[i].LastTestTime > models[j].LastTestTime
		}
		return models[i].ModelName < models[j].ModelName
	})
	summary.Models = models
	summary.TotalModels = len(models)

	groups := make([]publicModelMonitorGroup, 0, len(groupsByName))
	for groupName, group := range groupsByName {
		item := buildModelMonitorGroup(groupName, group, now, staleAfterSeconds)
		groups = append(groups, item)
		summary.TotalRoutes += item.TotalRoutes
		summary.AvailableRoutes += item.AvailableRoutes
		if item.LastTestTime > summary.LastTestTime {
			summary.LastTestTime = item.LastTestTime
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		leftPriority := modelMonitorStatusPriority(groups[i].Status)
		rightPriority := modelMonitorStatusPriority(groups[j].Status)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if groups[i].AvailableRoutes != groups[j].AvailableRoutes {
			return groups[i].AvailableRoutes > groups[j].AvailableRoutes
		}
		return groups[i].GroupName < groups[j].GroupName
	})
	summary.Groups = groups
	return summary
}

func buildModelMonitorGroup(groupName string, group *modelMonitorGroupAccumulator, now int64, staleAfterSeconds int64) publicModelMonitorGroup {
	item := publicModelMonitorGroup{
		GroupName:    groupName,
		Status:       modelMonitorStatusUnknown,
		TotalRoutes:  len(group.Routes),
		ModelNames:   make([]string, 0, len(group.ModelNames)),
		Routes:       make([]modelMonitorRouteSummary, 0, len(group.Routes)),
		MinLatencyMs: 0,
	}
	item.Routes = append(item.Routes, group.Routes...)
	for modelName := range group.ModelNames {
		item.ModelNames = append(item.ModelNames, modelName)
	}
	sort.Strings(item.ModelNames)
	item.ModelCount = len(item.ModelNames)

	totalLatency := 0
	for _, route := range group.Routes {
		if route.TestTime > item.LastTestTime {
			item.LastTestTime = route.TestTime
		}
		if route.TestTime > 0 {
			item.TestedRoutes++
		}
		if route.Status != common.ChannelStatusEnabled {
			item.UnavailableRoutes++
			continue
		}
		if route.TestTime <= 0 || now-route.TestTime > staleAfterSeconds {
			item.UnknownRoutes++
			continue
		}
		item.AvailableRoutes++
		totalLatency += route.ResponseTime
		if item.MinLatencyMs == 0 || route.ResponseTime < item.MinLatencyMs {
			item.MinLatencyMs = route.ResponseTime
		}
	}

	if item.AvailableRoutes > 0 {
		item.AvgLatencyMs = totalLatency / item.AvailableRoutes
		item.Status = modelMonitorStatusOperational
		if item.AvailableRoutes < item.TotalRoutes {
			item.Status = modelMonitorStatusDegraded
		}
		return item
	}
	if item.TotalRoutes > 0 && item.UnavailableRoutes == item.TotalRoutes {
		item.Status = modelMonitorStatusUnavailable
	}
	return item
}

func buildModelMonitorModel(modelName string, routes []modelMonitorRouteSummary, now int64, staleAfterSeconds int64) publicModelMonitorModel {
	item := publicModelMonitorModel{
		ModelName:    modelName,
		Status:       modelMonitorStatusUnknown,
		TotalRoutes:  len(routes),
		MinLatencyMs: 0,
	}

	totalLatency := 0
	for _, route := range routes {
		if route.TestTime > item.LastTestTime {
			item.LastTestTime = route.TestTime
		}
		if route.TestTime > 0 {
			item.TestedRoutes++
		}
		if route.Status != common.ChannelStatusEnabled {
			item.UnavailableRoutes++
			continue
		}
		if route.TestTime <= 0 || now-route.TestTime > staleAfterSeconds {
			item.UnknownRoutes++
			continue
		}
		item.AvailableRoutes++
		totalLatency += route.ResponseTime
		if item.MinLatencyMs == 0 || route.ResponseTime < item.MinLatencyMs {
			item.MinLatencyMs = route.ResponseTime
		}
	}

	if item.AvailableRoutes > 0 {
		item.AvgLatencyMs = totalLatency / item.AvailableRoutes
		item.Status = modelMonitorStatusOperational
		if item.AvailableRoutes < item.TotalRoutes {
			item.Status = modelMonitorStatusDegraded
		}
		return item
	}
	if item.TotalRoutes > 0 && item.UnavailableRoutes == item.TotalRoutes {
		item.Status = modelMonitorStatusUnavailable
	}
	return item
}

func buildModelMonitorProbe(task *model.SystemTask) publicModelMonitorProbe {
	probe := publicModelMonitorProbe{
		Status:     modelMonitorStatusUnknown,
		TaskStatus: task.Status,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
	}
	if task.Status == model.SystemTaskStatusFailed {
		probe.Status = modelMonitorStatusUnavailable
		return probe
	}
	if task.Status != model.SystemTaskStatusSucceeded {
		return probe
	}

	summary := channelTestSummary{}
	if strings.TrimSpace(task.Result) != "" {
		_ = common.Unmarshal([]byte(task.Result), &summary)
	}
	probe.Tested = summary.Tested
	probe.Succeeded = summary.Succeeded
	probe.Failed = summary.Failed
	probe.Disabled = summary.Disabled
	probe.Enabled = summary.Enabled
	if probe.Tested <= 0 {
		return probe
	}

	probe.SuccessRate = float64(probe.Succeeded) / float64(probe.Tested) * 100
	if probe.Succeeded == probe.Tested && probe.Disabled == 0 {
		probe.Status = modelMonitorStatusOperational
		return probe
	}
	if probe.Succeeded > 0 {
		probe.Status = modelMonitorStatusDegraded
		return probe
	}
	probe.Status = modelMonitorStatusUnavailable
	return probe
}

func modelMonitorStatusPriority(status string) int {
	switch status {
	case modelMonitorStatusOperational:
		return 0
	case modelMonitorStatusDegraded:
		return 1
	case modelMonitorStatusUnknown:
		return 2
	case modelMonitorStatusUnavailable:
		return 3
	default:
		return 4
	}
}
