package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorCaptureDefaults(t *testing.T) {
	t.Setenv("ERROR_CAPTURE_QUEUE_SIZE", "")
	t.Setenv("ERROR_CAPTURE_QUEUE_MAX_MB", "")

	config := loadErrorCaptureConfig()

	require.Equal(t, 1000, config.QueueSize)
	require.Equal(t, int64(256<<20), config.QueueMaxBytes)
}

func TestErrorCaptureWritesExactBodyAndRetryHistory(t *testing.T) {
	root := t.TempDir()
	manager := startTestErrorCaptureManager(t, root)
	body := []byte(`{"model":"gemini-test","messages":[{"role":"user","content":"raw prompt"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Set(common.RequestIdKey, "request-123")
	c.Set("id", 42)
	c.Set("username", "capture-user")
	c.Set("token_name", "capture-token")
	c.Set("original_model", "gemini-test")
	c.Set("group", "test-group")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)

	BeginErrorCapture(c)
	firstErr := types.NewOpenAIError(errors.New("first failure"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	firstErr.SetDiagnosticResponseBody([]byte(`{"error":"rate limited"}`))
	c.Set(common.UpstreamRequestIdKey, "upstream-first")
	RecordErrorCaptureAttempt(c, types.ChannelError{ChannelId: 85, ChannelName: "first", ChannelType: 1}, firstErr)

	secondErr := types.NewOpenAIError(errors.New("second failure"), types.ErrorCodeBadResponseStatusCode, http.StatusServiceUnavailable)
	secondErr.SetDiagnosticResponseBody([]byte(`{"error":"unavailable"}`))
	c.Set(common.UpstreamRequestIdKey, "upstream-second")
	RecordErrorCaptureAttempt(c, types.ChannelError{ChannelId: 87, ChannelName: "second", ChannelType: 2}, secondErr)

	FinalizeErrorCapture(c, nil)
	stopTestErrorCaptureManager(t, manager)

	files, err := filepath.Glob(filepath.Join(root, "*", "*", "request-123.capture"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	metadata, capturedBody := readCaptureFile(t, files[0])
	require.Equal(t, body, capturedBody)
	require.True(t, metadata.FinalSuccess)
	require.Equal(t, int64(len(body)), metadata.RequestBodyBytes)
	require.Len(t, metadata.Attempts, 2)
	assert.Equal(t, 85, metadata.Attempts[0].ChannelID)
	assert.Equal(t, "upstream-first", metadata.Attempts[0].UpstreamRequestID)
	assert.Equal(t, `{"error":"rate limited"}`, metadata.Attempts[0].ResponseBody)
	assert.Equal(t, 87, metadata.Attempts[1].ChannelID)
	assert.Equal(t, "upstream-second", metadata.Attempts[1].UpstreamRequestID)
	assert.Equal(t, `{"error":"unavailable"}`, metadata.Attempts[1].ResponseBody)
	_, exists := c.Get(common.KeyBodyStorage)
	assert.True(t, !exists || c.MustGet(common.KeyBodyStorage) == nil)
}

func TestErrorCaptureWritesFinalErrorWithoutChannelAttempt(t *testing.T) {
	root := t.TempDir()
	manager := startTestErrorCaptureManager(t, root)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"bad":true}`))
	c.Set(common.RequestIdKey, "request-local-error")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	BeginErrorCapture(c)

	finalErr := types.NewErrorWithStatusCode(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	FinalizeErrorCapture(c, finalErr)
	stopTestErrorCaptureManager(t, manager)

	files, err := filepath.Glob(filepath.Join(root, "*", "*", "request-local-error.capture"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	metadata, _ := readCaptureFile(t, files[0])
	require.False(t, metadata.FinalSuccess)
	require.Equal(t, http.StatusBadRequest, metadata.FinalStatus)
	require.Equal(t, "invalid request", metadata.FinalError)
	require.Empty(t, metadata.Attempts)
}

func TestErrorCaptureQueueFullDropsWithoutBlocking(t *testing.T) {
	manager := &errorCaptureManager{
		config: errorCaptureConfig{QueueMaxBytes: 1024},
		queue:  make(chan *errorCaptureItem, 1),
	}
	first := &errorCaptureItem{metadata: errorCaptureMetadata{RequestBodyBytes: 1}}
	second := &errorCaptureItem{metadata: errorCaptureMetadata{RequestBodyBytes: 1}}

	require.True(t, manager.enqueue(first))
	require.False(t, manager.enqueue(second))
	require.Len(t, manager.queue, 1)

	queued := <-manager.queue
	manager.pendingBytes.Add(-queued.pendingBytes)
}

func TestErrorCapturePendingByteLimit(t *testing.T) {
	manager := &errorCaptureManager{
		config: errorCaptureConfig{QueueMaxBytes: 5},
		queue:  make(chan *errorCaptureItem, 2),
	}

	require.False(t, manager.enqueue(&errorCaptureItem{metadata: errorCaptureMetadata{RequestBodyBytes: 6}}))
	require.Empty(t, manager.queue)
	require.Zero(t, manager.pendingBytes.Load())
}

func TestErrorCapturePreservesNonUTF8ResponseBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	state := &errorCaptureState{metadata: errorCaptureMetadata{}}
	c.Set(errorCaptureContextKey, state)
	err := types.NewOpenAIError(errors.New("binary response"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	body := []byte{0xff, 0x00, 0xfe}
	err.SetDiagnosticResponseBody(body)

	RecordErrorCaptureAttempt(c, types.ChannelError{ChannelId: 1}, err)

	require.Len(t, state.metadata.Attempts, 1)
	require.Empty(t, state.metadata.Attempts[0].ResponseBody)
	decoded, decodeErr := base64.StdEncoding.DecodeString(state.metadata.Attempts[0].ResponseBodyBase64)
	require.NoError(t, decodeErr)
	require.Equal(t, body, decoded)
}

func TestErrorCaptureCleanupDeletesOldestBatch(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "20260920", "12")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	now := time.Now()
	for i := 0; i < 5; i++ {
		path := filepath.Join(directory, "request-"+string(rune('a'+i))+".capture")
		require.NoError(t, os.WriteFile(path, []byte("capture"), 0o600))
		modified := now.Add(time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(path, modified, modified))
	}
	manager := &errorCaptureManager{config: errorCaptureConfig{
		Dir:             root,
		MaxFiles:        3,
		MaxBytes:        math.MaxInt64,
		Retention:       24 * time.Hour,
		DeleteBatchSize: 2,
	}}

	require.NoError(t, manager.cleanup(now.Add(10*time.Minute)))
	files, err := scanErrorCaptureFiles(root)
	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, int64(3), manager.fileCount.Load())
	for _, file := range files {
		assert.NotContains(t, file.path, "request-a.capture")
		assert.NotContains(t, file.path, "request-b.capture")
	}
}

func TestErrorCaptureCleanupDeletesExpiredFiles(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "20260919", "12")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	path := filepath.Join(directory, "expired.capture")
	require.NoError(t, os.WriteFile(path, []byte("capture"), 0o600))
	now := time.Now()
	expired := now.Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(path, expired, expired))
	manager := &errorCaptureManager{config: errorCaptureConfig{
		Dir:             root,
		MaxFiles:        100,
		MaxBytes:        math.MaxInt64,
		Retention:       24 * time.Hour,
		DeleteBatchSize: 10,
	}}

	require.NoError(t, manager.cleanup(now))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Zero(t, manager.fileCount.Load())
}

func startTestErrorCaptureManager(t *testing.T, root string) *errorCaptureManager {
	t.Helper()
	manager, err := newErrorCaptureManager(errorCaptureConfig{
		Enabled:         true,
		Dir:             root,
		QueueSize:       10,
		QueueMaxBytes:   10 << 20,
		MaxFiles:        100,
		MaxBytes:        100 << 20,
		Retention:       24 * time.Hour,
		DeleteBatchSize: 10,
	})
	require.NoError(t, err)
	errorCaptureManagerMu.Lock()
	require.Nil(t, activeErrorCapture)
	activeErrorCapture = manager
	errorCaptureManagerMu.Unlock()
	return manager
}

func stopTestErrorCaptureManager(t *testing.T, manager *errorCaptureManager) {
	t.Helper()
	errorCaptureManagerMu.Lock()
	if activeErrorCapture == manager {
		activeErrorCapture = nil
	}
	errorCaptureManagerMu.Unlock()
	manager.close()
}

func readCaptureFile(t *testing.T, path string) (errorCaptureMetadata, []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte(errorCaptureFormatHeader)))
	parts := bytes.SplitN(data[len(errorCaptureFormatHeader):], []byte(errorCaptureBodyDelimiter), 2)
	require.Len(t, parts, 2)
	var metadata errorCaptureMetadata
	require.NoError(t, common.Unmarshal(parts[0], &metadata))
	return metadata, parts[1]
}
