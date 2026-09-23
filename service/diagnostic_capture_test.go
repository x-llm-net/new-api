package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticCaptureRuleMatching(t *testing.T) {
	config := diagnosticCaptureConfig{Rules: []diagnosticCaptureRule{
		{
			Name:            "model-and-channel",
			RequestedModels: []string{"gemini-3.1-pro-preview", "gemini-3.1-pro"},
			ChannelIDs:      []int{136, 166},
			UserIDs:         []int{9259},
		},
		{
			Name:           "mapped-model",
			UpstreamModels: []string{"gemini-3.1-pro-high"},
		},
	}}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3.1-pro-preview",
		UserId:          9259,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         136,
			UpstreamModelName: "gemini-3.1-pro-high",
		},
	}

	target := diagnosticCaptureTarget{
		requestedModel: info.OriginModelName,
		upstreamModel:  info.UpstreamModelName,
		channelID:      info.ChannelId,
		userID:         info.UserId,
		protocol:       string(types.RelayFormatGemini),
	}
	matched := config.match(target)
	require.NotNil(t, matched)
	assert.Equal(t, "model-and-channel", matched.Name)

	info.UserId = 42
	target.userID = info.UserId
	matched = config.match(target)
	require.NotNil(t, matched)
	assert.Equal(t, "mapped-model", matched.Name)

	info.UpstreamModelName = "different-model"
	target.upstreamModel = info.UpstreamModelName
	assert.Nil(t, config.match(target))
}

func TestNormalizeDiagnosticCaptureConfigRejectsUnsafeWildcard(t *testing.T) {
	config := diagnosticCaptureConfig{
		Enabled:   true,
		ExpiresAt: time.Now().Add(time.Hour),
		Rules:     []diagnosticCaptureRule{{Name: "everything"}},
	}

	err := normalizeDiagnosticCaptureConfig(&config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one filter")
}

func TestNormalizeDiagnosticCaptureConfigRejectsOversizedBodyLimit(t *testing.T) {
	config := diagnosticCaptureConfig{
		Enabled:          true,
		ExpiresAt:        time.Now().Add(time.Hour),
		MaxRequestBytes:  maxDiagnosticBodyBytes + 1,
		MaxResponseBytes: 1,
		Rules: []diagnosticCaptureRule{{
			Name:            "target",
			RequestedModels: []string{"gemini-test"},
		}},
	}

	err := normalizeDiagnosticCaptureConfig(&config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_request_bytes")
}

func TestDiagnosticCaptureSampleIsDeterministic(t *testing.T) {
	first := diagnosticCaptureSample("rule", "request-123", 0.5)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, diagnosticCaptureSample("rule", "request-123", 0.5))
	}
	assert.True(t, diagnosticCaptureSample("rule", "request-123", 1))
	assert.False(t, diagnosticCaptureSample("rule", "request-123", 0))
}

func TestDiagnosticCaptureResponseWrapperForwardsAndBoundsBody(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	attempt := &diagnosticCaptureAttempt{
		manager:       manager,
		metadata:      diagnosticCaptureMetadata{Rule: "bounded", RequestID: "request-123"},
		requestBody:   []byte("request"),
		responseLimit: 4,
	}
	response := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 10,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("0123456789")),
	}

	WrapDiagnosticCaptureResponse(attempt, response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "0123456789", string(body))

	item := <-manager.queue
	manager.pendingBytes.Add(-item.pendingBytes)
	assert.Equal(t, []byte("0123"), item.responseBody)
	assert.Equal(t, int64(10), item.metadata.ResponseBodyBytes)
	assert.True(t, item.metadata.ResponseTruncated)
	assert.True(t, item.metadata.ResponseComplete)
	assert.Equal(t, http.StatusOK, item.metadata.UpstreamStatus)
}

func TestDiagnosticCaptureResponseWrapperMarksEarlyClose(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	attempt := &diagnosticCaptureAttempt{
		manager:       manager,
		metadata:      diagnosticCaptureMetadata{Rule: "early-close"},
		responseLimit: 100,
	}
	response := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 6,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("abcdef")),
	}

	WrapDiagnosticCaptureResponse(attempt, response)
	buffer := make([]byte, 2)
	_, err := response.Body.Read(buffer)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	item := <-manager.queue
	manager.pendingBytes.Add(-item.pendingBytes)
	assert.Equal(t, []byte("ab"), item.responseBody)
	assert.False(t, item.metadata.ResponseComplete)
	assert.False(t, item.metadata.ResponseTruncated)
}

func TestBeginDiagnosticCaptureAttemptCapturesFilteredClientBody(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	manager.config.Store(&diagnosticCaptureConfig{
		Enabled:          true,
		ExpiresAt:        time.Now().Add(time.Hour),
		MaxRequestBytes:  5,
		MaxResponseBytes: 10,
		Rules: []diagnosticCaptureRule{{
			Name:            "target",
			RequestedModels: []string{"gemini-client-model"},
			ChannelIDs:      []int{136},
			SampleRate:      1,
		}},
	})
	setTestDiagnosticCaptureManager(t, manager)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("abcdefgh"))
	c.Set("token_name", "customer-token")
	c.Set(string(constant.ContextKeyOriginalModel), "gemini-client-model")
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	_, err = storage.Seek(3, io.SeekStart)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		RequestId:       "request-filtered",
		UserId:          7,
		TokenId:         8,
		UsingGroup:      "Gemini",
		OriginModelName: "gemini-test",
		RelayFormat:     types.RelayFormatOpenAI,
		RetryIndex:      2,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         136,
			ChannelType:       24,
			UpstreamModelName: "gemini-upstream",
		},
	}

	attempt := BeginDiagnosticCaptureAttempt(c, info)
	require.NotNil(t, attempt)
	assert.Equal(t, []byte("abcde"), attempt.requestBody)
	assert.Equal(t, int64(8), attempt.metadata.ClientRequestBodyBytes)
	assert.True(t, attempt.metadata.ClientRequestTruncated)
	currentOffset, err := storage.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(3), currentOffset)
	assert.Equal(t, "gemini-client-model", attempt.metadata.RequestedModel)
	assert.Equal(t, "gemini-upstream", attempt.metadata.UpstreamModel)
	assert.Equal(t, 136, attempt.metadata.ChannelID)
	assert.Equal(t, "openai", attempt.metadata.Protocol)
}

func TestBeginDiagnosticCaptureAttemptSkipsExpiredConfig(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	manager.config.Store(&diagnosticCaptureConfig{
		Enabled:          true,
		ExpiresAt:        time.Now().Add(-time.Second),
		MaxRequestBytes:  10,
		MaxResponseBytes: 10,
		Rules: []diagnosticCaptureRule{{
			Name:            "expired",
			RequestedModels: []string{"gemini-test"},
			SampleRate:      1,
		}},
	})
	setTestDiagnosticCaptureManager(t, manager)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-test",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         136,
			UpstreamModelName: "gemini-test",
		},
	}

	assert.Nil(t, BeginDiagnosticCaptureAttempt(c, info))
}

func TestDiagnosticCaptureQueueFullDropsWithoutBlocking(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	first := &diagnosticCaptureItem{}
	second := &diagnosticCaptureItem{}

	assert.True(t, manager.enqueue(first))
	assert.False(t, manager.enqueue(second))
	assert.Equal(t, int64(1), manager.droppedQueue.Load())
	queued := <-manager.queue
	manager.pendingBytes.Add(-queued.pendingBytes)
}

func TestDiagnosticCapturePendingByteLimit(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 10,
	}

	assert.False(t, manager.enqueue(&diagnosticCaptureItem{requestBody: []byte("too large")}))
	assert.Empty(t, manager.queue)
	assert.Zero(t, manager.pendingBytes.Load())
}

func TestDiagnosticCaptureConfigHotReloadAndRemoval(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	manager := &diagnosticCaptureManager{configPath: configPath}
	configJSON := fmt.Sprintf(`{
  "version": 1,
  "enabled": true,
  "expires_at": %q,
  "rules": [{"name":"gemini","requested_models":["gemini-test"]}]
}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	require.NoError(t, os.WriteFile(configPath, []byte(configJSON), 0o600))

	manager.reloadConfig()
	loaded := manager.config.Load()
	require.NotNil(t, loaded)
	assert.True(t, loaded.Enabled)
	assert.Equal(t, int64(defaultDiagnosticRequestBytes), loaded.MaxRequestBytes)
	assert.Equal(t, float64(1), loaded.Rules[0].SampleRate)

	require.NoError(t, os.Remove(configPath))
	manager.reloadConfig()
	assert.Nil(t, manager.config.Load())
}

func TestDiagnosticCaptureWritesBodiesAndCleansOldest(t *testing.T) {
	root := t.TempDir()
	manager := &diagnosticCaptureManager{dir: root}
	manager.config.Store(&diagnosticCaptureConfig{
		MaxFiles:        1,
		MaxTotalGB:      1,
		RetentionHours:  24,
		DeleteBatchSize: 1,
	})
	firstTime := time.Now().Add(-time.Minute)
	first := &diagnosticCaptureItem{
		metadata: diagnosticCaptureMetadata{
			Version:    1,
			Rule:       "gemini/model",
			RequestID:  "request:first",
			CreatedAt:  firstTime,
			FinishedAt: firstTime.Add(time.Second),
			ChannelID:  136,
			RetryIndex: 0,
		},
		requestBody:  []byte("request-one"),
		responseBody: []byte("response-one"),
	}
	firstPath, _, err := manager.write(first)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(firstPath)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	metadata, requestBody, responseBody := readDiagnosticCaptureFile(t, firstPath)
	assert.Equal(t, "request:first", metadata.RequestID)
	assert.Equal(t, []byte("request-one"), requestBody)
	assert.Equal(t, []byte("response-one"), responseBody)

	secondTime := time.Now()
	second := &diagnosticCaptureItem{
		metadata: diagnosticCaptureMetadata{
			Version:    1,
			Rule:       "gemini",
			RequestID:  "request-second",
			CreatedAt:  secondTime,
			FinishedAt: secondTime.Add(time.Second),
			ChannelID:  166,
		},
	}
	secondPath, _, err := manager.write(second)
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(firstPath, firstTime, firstTime))
	require.NoError(t, os.Chtimes(secondPath, secondTime, secondTime))

	require.NoError(t, manager.cleanup(time.Now()))
	_, err = os.Stat(firstPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(secondPath)
	assert.NoError(t, err)
}

func TestFinishDiagnosticCaptureTransportError(t *testing.T) {
	manager := &diagnosticCaptureManager{
		queue:         make(chan *diagnosticCaptureItem, 1),
		queueMaxBytes: 1 << 20,
	}
	attempt := &diagnosticCaptureAttempt{
		manager:     manager,
		metadata:    diagnosticCaptureMetadata{Rule: "transport"},
		requestBody: []byte("request"),
	}

	FinishDiagnosticCaptureTransportError(attempt, errors.New("dial timeout"))
	item := <-manager.queue
	manager.pendingBytes.Add(-item.pendingBytes)
	assert.Equal(t, "dial timeout", item.metadata.TransportError)
	assert.False(t, item.metadata.FinishedAt.IsZero())
}

func TestDiagnosticTransportErrorOmitsUpstreamURL(t *testing.T) {
	err := &url.Error{
		Op:  "Post",
		URL: "https://user:secret@example.com/v1/messages",
		Err: errors.New("connection reset"),
	}

	message := diagnosticTransportError(err)
	assert.Equal(t, "Post: connection reset", message)
	assert.NotContains(t, message, "secret")
	assert.NotContains(t, message, "example.com")
}

func TestDiagnosticCaptureCleanupDefaultsWithoutConfig(t *testing.T) {
	root := t.TempDir()
	manager := &diagnosticCaptureManager{dir: root}
	oldDirectory := filepath.Join(root, "20260920", "12", "old")
	require.NoError(t, os.MkdirAll(oldDirectory, 0o700))
	path := filepath.Join(oldDirectory, "expired.capture")
	require.NoError(t, os.WriteFile(path, []byte("capture"), 0o600))
	expired := time.Now().Add(-(defaultDiagnosticCaptureHours + 1) * time.Hour)
	require.NoError(t, os.Chtimes(path, expired, expired))

	require.NoError(t, manager.cleanup(time.Now()))
	_, err := os.Stat(path)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func setTestDiagnosticCaptureManager(t *testing.T, manager *diagnosticCaptureManager) {
	t.Helper()
	diagnosticCaptureManagerMu.Lock()
	previous := activeDiagnosticCapture
	activeDiagnosticCapture = manager
	diagnosticCaptureManagerMu.Unlock()
	t.Cleanup(func() {
		diagnosticCaptureManagerMu.Lock()
		activeDiagnosticCapture = previous
		diagnosticCaptureManagerMu.Unlock()
	})
}

func readDiagnosticCaptureFile(t *testing.T, path string) (diagnosticCaptureMetadata, []byte, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte(diagnosticCaptureFormatHeader)))
	withoutHeader := data[len(diagnosticCaptureFormatHeader):]
	requestIndex := bytes.Index(withoutHeader, []byte(diagnosticCaptureRequestDelimiter))
	require.GreaterOrEqual(t, requestIndex, 0)
	var metadata diagnosticCaptureMetadata
	require.NoError(t, common.Unmarshal(withoutHeader[:requestIndex], &metadata))
	bodyStart := requestIndex + len(diagnosticCaptureRequestDelimiter)
	responseIndex := bytes.Index(withoutHeader[bodyStart:], []byte(diagnosticCaptureResponseDelimiter))
	require.GreaterOrEqual(t, responseIndex, 0)
	requestBody := withoutHeader[bodyStart : bodyStart+responseIndex]
	responseBody := withoutHeader[bodyStart+responseIndex+len(diagnosticCaptureResponseDelimiter):]
	return metadata, requestBody, responseBody
}
