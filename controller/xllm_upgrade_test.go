package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXLLMHealthCheckWorkersPersistSuccessAndFailure(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousDisable, previousEnable := common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled
	previousInterval := common.RequestInterval
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = previousDisable, previousEnable
		common.RequestInterval = previousInterval
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.XLLMGroupStabilitySample{}))
	common.AutomaticDisableChannelEnabled, common.AutomaticEnableChannelEnabled = false, false
	common.RequestInterval = 0
	previousPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices)) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o-mini":0.001}`))
	withTieredBillingConfig(t, map[string]string{}, map[string]string{})
	service.InitHttpClient()
	user := &model.User{Id: 987654, Username: "stability-test", Group: "default", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	var upstreamRequests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bearer failed-test-channel" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"unavailable","type":"server_error"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"test","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	channels := []*model.Channel{
		{Name: "healthy", Type: constant.ChannelTypeOpenAI, Key: "healthy-test-channel", Models: "gpt-4o-mini", Group: "default", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled},
		{Name: "failed", Type: constant.ChannelTypeOpenAI, Key: "failed-test-channel", Models: "gpt-4o-mini", Group: "default", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled},
		{Name: "manual", Type: constant.ChannelTypeOpenAI, Key: "manual-test-channel", Models: "gpt-4o-mini", Group: "default", BaseURL: &upstream.URL, Status: common.ChannelStatusManuallyDisabled},
	}
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
	}

	summary := performChannelTests(context.Background(), "scheduled-test", channels, user.Id, false, true, 2, nil)
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 1, Failed: 1}, summary)
	assert.Equal(t, int32(2), upstreamRequests.Load())
	var samples []model.XLLMGroupStabilitySample
	require.NoError(t, db.Order("channel_id").Find(&samples).Error)
	require.Len(t, samples, 2)
	assert.True(t, samples[0].Success)
	assert.False(t, samples[1].Success)
	assert.NotEmpty(t, samples[1].ErrorCode)
	for _, sample := range samples {
		assert.Equal(t, "scheduled-test", sample.TaskID)
		assert.Equal(t, "default", sample.GroupNames)
		assert.Positive(t, sample.TestedAt)
		assert.Equal(t, common.ChannelStatusEnabled, sample.ChannelStatusBefore)
	}

	performChannelTests(context.Background(), "recovery-test", channels[:1], user.Id, false, false, 1, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	performChannelTests(ctx, "cancelled-test", channels, user.Id, false, true, 2, nil)
	var count int64
	require.NoError(t, db.Model(&model.XLLMGroupStabilitySample{}).Count(&count).Error)
	assert.Equal(t, int64(2), count, "recovery and cancelled runs must not add public stability samples")
}

func TestXLLMSelfPermissionsKeepFinanceAccessSeparateFromAdmin(t *testing.T) {
	setting := operation_setting.GetInvoiceSetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	setting.Enabled = true
	setting.OperatorUserIDs = []int{42}
	finance := calculateUserPermissions(42, common.RoleCommonUser)
	ordinary := calculateUserPermissions(43, common.RoleCommonUser)
	assert.Equal(t, true, finance["invoice_operator"])
	assert.Equal(t, false, ordinary["invoice_operator"])
	assert.Equal(t, true, ordinary["invoice_enabled"])
	setting.Enabled = false
	assert.Equal(t, false, calculateUserPermissions(42, common.RoleCommonUser)["invoice_enabled"])
}
