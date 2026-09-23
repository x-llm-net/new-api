package service

import (
	"bufio"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const (
	diagnosticCaptureFormatHeader      = "XLLM-DIAGNOSTIC-CAPTURE-V1\n"
	diagnosticCaptureRequestDelimiter  = "\n---CLIENT-REQUEST-BODY---\n"
	diagnosticCaptureResponseDelimiter = "\n---UPSTREAM-RESPONSE-BODY---\n"
	diagnosticCaptureReloadPeriod      = 2 * time.Second
	diagnosticCaptureCleanupPeriod     = 10 * time.Minute
	defaultDiagnosticCaptureDir        = "/data/diagnostic-captures"
	defaultDiagnosticCaptureQueue      = 1000
	defaultDiagnosticCaptureQueueMB    = 256
	defaultDiagnosticCaptureFiles      = 10000
	defaultDiagnosticCaptureGB         = 2
	defaultDiagnosticCaptureHours      = 24
	defaultDiagnosticCaptureBatch      = 1000
	defaultDiagnosticRequestBytes      = 512 << 10
	defaultDiagnosticResponseBytes     = 1 << 20
	maxDiagnosticConfigBytes           = 1 << 20
	maxDiagnosticRules                 = 100
	maxDiagnosticFilterValues          = 1000
	maxDiagnosticBodyBytes             = 100 << 20
)

type diagnosticCaptureConfig struct {
	Version          int                     `json:"version"`
	Enabled          bool                    `json:"enabled"`
	ExpiresAt        time.Time               `json:"expires_at"`
	Rules            []diagnosticCaptureRule `json:"rules"`
	MaxFiles         int64                   `json:"max_files"`
	MaxTotalGB       int64                   `json:"max_total_gb"`
	RetentionHours   int                     `json:"retention_hours"`
	DeleteBatchSize  int                     `json:"delete_batch_size"`
	MaxRequestBytes  int64                   `json:"max_request_bytes"`
	MaxResponseBytes int64                   `json:"max_response_bytes"`
}

type diagnosticCaptureRule struct {
	Name            string   `json:"name"`
	RequestedModels []string `json:"requested_models,omitempty"`
	UpstreamModels  []string `json:"upstream_models,omitempty"`
	ChannelIDs      []int    `json:"channel_ids,omitempty"`
	UserIDs         []int    `json:"user_ids,omitempty"`
	TokenIDs        []int    `json:"token_ids,omitempty"`
	Groups          []string `json:"groups,omitempty"`
	Protocols       []string `json:"protocols,omitempty"`
	SampleRate      float64  `json:"sample_rate,omitempty"`
}

type diagnosticCaptureMetadata struct {
	Version                    int       `json:"version"`
	Rule                       string    `json:"rule"`
	RequestID                  string    `json:"request_id"`
	CreatedAt                  time.Time `json:"created_at"`
	FinishedAt                 time.Time `json:"finished_at"`
	RetryIndex                 int       `json:"retry_index"`
	UserID                     int       `json:"user_id"`
	TokenID                    int       `json:"token_id"`
	TokenName                  string    `json:"token_name,omitempty"`
	Group                      string    `json:"group,omitempty"`
	RequestedModel             string    `json:"requested_model,omitempty"`
	UpstreamModel              string    `json:"upstream_model,omitempty"`
	ChannelID                  int       `json:"channel_id"`
	ChannelType                int       `json:"channel_type"`
	Protocol                   string    `json:"protocol,omitempty"`
	Stream                     bool      `json:"stream"`
	Method                     string    `json:"method,omitempty"`
	RequestPath                string    `json:"request_path,omitempty"`
	UpstreamStatus             int       `json:"upstream_status"`
	UpstreamRequestID          string    `json:"upstream_request_id,omitempty"`
	ClientRequestBodyBytes     int64     `json:"client_request_body_bytes"`
	ClientRequestCapturedBytes int64     `json:"client_request_captured_bytes"`
	ClientRequestTruncated     bool      `json:"client_request_truncated"`
	ClientRequestCaptureError  string    `json:"client_request_capture_error,omitempty"`
	ResponseBodyBytes          int64     `json:"response_body_bytes"`
	ResponseCapturedBytes      int64     `json:"response_captured_bytes"`
	ResponseTruncated          bool      `json:"response_truncated"`
	ResponseComplete           bool      `json:"response_complete"`
	ResponseReadError          string    `json:"response_read_error,omitempty"`
	TransportError             string    `json:"transport_error,omitempty"`
}

type diagnosticCaptureItem struct {
	metadata     diagnosticCaptureMetadata
	requestBody  []byte
	responseBody []byte
	pendingBytes int64
}

type diagnosticCaptureFile struct {
	path    string
	size    int64
	modTime time.Time
}

type diagnosticCaptureManager struct {
	dir           string
	configPath    string
	queue         chan *diagnosticCaptureItem
	queueMaxBytes int64
	config        atomic.Pointer[diagnosticCaptureConfig]
	pendingBytes  atomic.Int64
	fileCount     atomic.Int64
	fileBytes     atomic.Int64
	cleanupMu     sync.Mutex
	workerWG      sync.WaitGroup
	closeOnce     sync.Once
	stop          chan struct{}
	lastConfigID  string
	droppedQueue  atomic.Int64
	droppedBytes  atomic.Int64
	lastDropLog   atomic.Int64
	lastLoadLog   atomic.Int64
}

type diagnosticCaptureAttempt struct {
	manager       *diagnosticCaptureManager
	metadata      diagnosticCaptureMetadata
	requestBody   []byte
	responseLimit int64
}

type diagnosticCaptureTarget struct {
	requestedModel string
	upstreamModel  string
	channelID      int
	userID         int
	tokenID        int
	group          string
	protocol       string
}

type diagnosticCaptureResponseBody struct {
	body          io.ReadCloser
	attempt       *diagnosticCaptureAttempt
	contentLength int64
	mu            sync.Mutex
	responseBody  []byte
	responseBytes int64
	complete      bool
	readError     string
	finishOnce    sync.Once
}

var (
	diagnosticCaptureManagerMu sync.RWMutex
	activeDiagnosticCapture    *diagnosticCaptureManager
)

func StartDiagnosticCapture() {
	dir := strings.TrimSpace(common.GetEnvOrDefaultString("DIAGNOSTIC_CAPTURE_DIR", defaultDiagnosticCaptureDir))
	if dir == "" {
		dir = defaultDiagnosticCaptureDir
	}
	configPath := strings.TrimSpace(common.GetEnvOrDefaultString("DIAGNOSTIC_CAPTURE_CONFIG", filepath.Join(dir, "config.json")))
	queueSize := positiveEnv("DIAGNOSTIC_CAPTURE_QUEUE_SIZE", defaultDiagnosticCaptureQueue)
	queueMaxMB := positiveEnv("DIAGNOSTIC_CAPTURE_QUEUE_MAX_MB", defaultDiagnosticCaptureQueueMB)

	manager, err := newDiagnosticCaptureManager(dir, configPath, queueSize, int64(queueMaxMB)<<20)
	if err != nil {
		common.SysError("failed to start diagnostic capture: " + err.Error())
		return
	}
	diagnosticCaptureManagerMu.Lock()
	if activeDiagnosticCapture != nil {
		diagnosticCaptureManagerMu.Unlock()
		manager.close()
		return
	}
	activeDiagnosticCapture = manager
	diagnosticCaptureManagerMu.Unlock()
	common.SysLog(fmt.Sprintf("diagnostic capture watcher started: config=%s dir=%s", configPath, dir))
}

func newDiagnosticCaptureManager(dir string, configPath string, queueSize int, queueMaxBytes int64) (*diagnosticCaptureManager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create diagnostic capture directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure diagnostic capture directory: %w", err)
	}
	manager := &diagnosticCaptureManager{
		dir:           dir,
		configPath:    configPath,
		queue:         make(chan *diagnosticCaptureItem, queueSize),
		queueMaxBytes: queueMaxBytes,
		stop:          make(chan struct{}),
	}
	manager.reloadConfig()
	if err := manager.cleanup(time.Now()); err != nil {
		return nil, fmt.Errorf("initial diagnostic capture cleanup: %w", err)
	}
	manager.workerWG.Add(1)
	go manager.run()
	return manager, nil
}

func currentDiagnosticCaptureManager() *diagnosticCaptureManager {
	diagnosticCaptureManagerMu.RLock()
	defer diagnosticCaptureManagerMu.RUnlock()
	return activeDiagnosticCapture
}

func BeginDiagnosticCaptureAttempt(c *gin.Context, info *relaycommon.RelayInfo) *diagnosticCaptureAttempt {
	manager := currentDiagnosticCaptureManager()
	if manager == nil || c == nil || info == nil || info.ChannelMeta == nil {
		return nil
	}
	config := manager.config.Load()
	if config == nil || !config.Enabled || config.ExpiresAt.IsZero() || !time.Now().Before(config.ExpiresAt) {
		return nil
	}

	requestedModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if requestedModel == "" {
		requestedModel = info.OriginModelName
	}
	protocol := string(info.GetFinalRequestRelayFormat())
	target := diagnosticCaptureTarget{
		requestedModel: requestedModel,
		upstreamModel:  info.UpstreamModelName,
		channelID:      info.ChannelId,
		userID:         info.UserId,
		tokenID:        info.TokenId,
		group:          info.UsingGroup,
		protocol:       protocol,
	}
	rule := config.match(target)
	if rule == nil || !diagnosticCaptureSample(rule.Name, info.RequestId, rule.SampleRate) {
		return nil
	}

	createdAt := time.Now()
	if !info.StartTime.IsZero() {
		createdAt = info.StartTime
	}
	metadata := diagnosticCaptureMetadata{
		Version:        1,
		Rule:           rule.Name,
		RequestID:      info.RequestId,
		CreatedAt:      createdAt,
		RetryIndex:     info.RetryIndex,
		UserID:         info.UserId,
		TokenID:        info.TokenId,
		TokenName:      c.GetString("token_name"),
		Group:          info.UsingGroup,
		RequestedModel: requestedModel,
		UpstreamModel:  info.UpstreamModelName,
		ChannelID:      info.ChannelId,
		ChannelType:    info.ChannelType,
		Protocol:       protocol,
		Stream:         info.IsStream,
	}
	if c.Request != nil {
		metadata.Method = c.Request.Method
		if c.Request.URL != nil {
			metadata.RequestPath = c.Request.URL.Path
		}
	}

	requestBody, totalBytes, truncated, err := captureDiagnosticRequestBody(c, config.MaxRequestBytes)
	metadata.ClientRequestBodyBytes = totalBytes
	metadata.ClientRequestCapturedBytes = int64(len(requestBody))
	metadata.ClientRequestTruncated = truncated
	if err != nil {
		metadata.ClientRequestCaptureError = err.Error()
	}
	return &diagnosticCaptureAttempt{
		manager:       manager,
		metadata:      metadata,
		requestBody:   requestBody,
		responseLimit: config.MaxResponseBytes,
	}
}

func WrapDiagnosticCaptureResponse(attempt *diagnosticCaptureAttempt, resp *http.Response) {
	if attempt == nil || resp == nil || resp.Body == nil {
		return
	}
	attempt.metadata.UpstreamStatus = resp.StatusCode
	attempt.metadata.UpstreamRequestID = resp.Header.Get(common.RequestIdKey)
	resp.Body = &diagnosticCaptureResponseBody{
		body:          resp.Body,
		attempt:       attempt,
		contentLength: resp.ContentLength,
		responseBody:  make([]byte, 0, minInt64(attempt.responseLimit, 64<<10)),
	}
}

func FinishDiagnosticCaptureTransportError(attempt *diagnosticCaptureAttempt, err error) {
	if attempt == nil {
		return
	}
	if err != nil {
		attempt.metadata.TransportError = diagnosticTransportError(err)
	}
	attempt.metadata.FinishedAt = time.Now()
	attempt.manager.enqueue(&diagnosticCaptureItem{
		metadata:    attempt.metadata,
		requestBody: attempt.requestBody,
	})
}

func captureDiagnosticRequestBody(c *gin.Context, limit int64) ([]byte, int64, bool, error) {
	if c == nil || limit <= 0 {
		return nil, 0, false, nil
	}
	value, exists := c.Get(common.KeyBodyStorage)
	if !exists || value == nil {
		return nil, 0, false, errors.New("client request body storage is unavailable")
	}
	storage, ok := value.(common.BodyStorage)
	if !ok {
		return nil, 0, false, errors.New("client request body storage has unexpected type")
	}
	totalBytes := storage.Size()
	currentOffset, err := storage.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, totalBytes, totalBytes > limit, fmt.Errorf("read current client request offset: %w", err)
	}
	defer func() {
		_, _ = storage.Seek(currentOffset, io.SeekStart)
	}()
	if _, err := storage.Seek(0, io.SeekStart); err != nil {
		return nil, totalBytes, totalBytes > limit, fmt.Errorf("seek client request body: %w", err)
	}
	captureBytes := minInt64(totalBytes, limit)
	body := make([]byte, captureBytes)
	if captureBytes > 0 {
		if _, err := io.ReadFull(storage, body); err != nil {
			return nil, totalBytes, totalBytes > limit, fmt.Errorf("read client request body: %w", err)
		}
	}
	return body, totalBytes, totalBytes > limit, nil
}

func (b *diagnosticCaptureResponseBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	b.mu.Lock()
	if n > 0 {
		b.responseBytes += int64(n)
		remaining := b.attempt.responseLimit - int64(len(b.responseBody))
		if remaining > 0 {
			captured := minInt64(int64(n), remaining)
			b.responseBody = append(b.responseBody, p[:captured]...)
		}
	}
	if err == io.EOF {
		b.complete = true
	} else if err != nil {
		b.readError = err.Error()
	}
	shouldFinish := err != nil
	b.mu.Unlock()
	if shouldFinish {
		b.finish()
	}
	return n, err
}

func (b *diagnosticCaptureResponseBody) Close() error {
	err := b.body.Close()
	b.mu.Lock()
	if err != nil && b.readError == "" {
		b.readError = err.Error()
	}
	if b.contentLength >= 0 && b.responseBytes >= b.contentLength {
		b.complete = true
	}
	b.mu.Unlock()
	b.finish()
	return err
}

func (b *diagnosticCaptureResponseBody) finish() {
	b.finishOnce.Do(func() {
		b.mu.Lock()
		responseBody := append([]byte(nil), b.responseBody...)
		responseBytes := b.responseBytes
		complete := b.complete
		readError := b.readError
		b.mu.Unlock()

		metadata := b.attempt.metadata
		metadata.FinishedAt = time.Now()
		metadata.ResponseBodyBytes = responseBytes
		metadata.ResponseCapturedBytes = int64(len(responseBody))
		metadata.ResponseTruncated = responseBytes > int64(len(responseBody))
		metadata.ResponseComplete = complete
		metadata.ResponseReadError = readError
		b.attempt.manager.enqueue(&diagnosticCaptureItem{
			metadata:     metadata,
			requestBody:  b.attempt.requestBody,
			responseBody: responseBody,
		})
	})
}

func (c *diagnosticCaptureConfig) match(target diagnosticCaptureTarget) *diagnosticCaptureRule {
	for i := range c.Rules {
		rule := &c.Rules[i]
		if !containsStringOrWildcard(rule.RequestedModels, target.requestedModel) ||
			!containsStringOrWildcard(rule.UpstreamModels, target.upstreamModel) ||
			!containsIntOrWildcard(rule.ChannelIDs, target.channelID) ||
			!containsIntOrWildcard(rule.UserIDs, target.userID) ||
			!containsIntOrWildcard(rule.TokenIDs, target.tokenID) ||
			!containsStringOrWildcard(rule.Groups, target.group) ||
			!containsStringOrWildcard(rule.Protocols, target.protocol) {
			continue
		}
		return rule
	}
	return nil
}

func diagnosticTransportError(err error) string {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		if urlError.Err == nil {
			return urlError.Op
		}
		return fmt.Sprintf("%s: %v", urlError.Op, urlError.Err)
	}
	return err.Error()
}

func containsStringOrWildcard(values []string, target string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsIntOrWildcard(values []int, target int) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func diagnosticCaptureSample(ruleName string, requestID string, rate float64) bool {
	if rate >= 1 {
		return true
	}
	if rate <= 0 {
		return false
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(ruleName))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(requestID))
	const buckets = uint64(1_000_000)
	return hasher.Sum64()%buckets < uint64(rate*float64(buckets))
}

func (m *diagnosticCaptureManager) reloadConfig() {
	stat, err := os.Stat(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			m.lastConfigID = ""
			m.config.Store(nil)
			return
		}
		m.disableConfig("stat diagnostic capture config: " + err.Error())
		return
	}
	if stat.Size() > maxDiagnosticConfigBytes {
		m.disableConfig(fmt.Sprintf("diagnostic capture config exceeds %d bytes", maxDiagnosticConfigBytes))
		return
	}
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		m.disableConfig("read diagnostic capture config: " + err.Error())
		return
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write(data)
	configID := fmt.Sprintf("%x", hasher.Sum64())
	if configID == m.lastConfigID {
		return
	}
	m.lastConfigID = configID
	var config diagnosticCaptureConfig
	if err := common.Unmarshal(data, &config); err != nil {
		m.disableConfig("parse diagnostic capture config: " + err.Error())
		return
	}
	if err := normalizeDiagnosticCaptureConfig(&config); err != nil {
		m.disableConfig("validate diagnostic capture config: " + err.Error())
		return
	}
	m.config.Store(&config)
	common.SysLog(fmt.Sprintf(
		"diagnostic capture config loaded: enabled=%t rules=%d expires_at=%s",
		config.Enabled,
		len(config.Rules),
		config.ExpiresAt.Format(time.RFC3339),
	))
}

func normalizeDiagnosticCaptureConfig(config *diagnosticCaptureConfig) error {
	if config.Version == 0 {
		config.Version = 1
	}
	if config.Version != 1 {
		return fmt.Errorf("unsupported version %d", config.Version)
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = defaultDiagnosticCaptureFiles
	}
	if config.MaxFiles > 1_000_000 {
		return errors.New("max_files cannot exceed 1000000")
	}
	if config.MaxTotalGB <= 0 {
		config.MaxTotalGB = defaultDiagnosticCaptureGB
	}
	if config.MaxTotalGB > 1000 {
		return errors.New("max_total_gb cannot exceed 1000")
	}
	if config.RetentionHours <= 0 {
		config.RetentionHours = defaultDiagnosticCaptureHours
	}
	if config.RetentionHours > 720 {
		return errors.New("retention_hours cannot exceed 720")
	}
	if config.DeleteBatchSize <= 0 {
		config.DeleteBatchSize = defaultDiagnosticCaptureBatch
	}
	if config.DeleteBatchSize > 100000 {
		return errors.New("delete_batch_size cannot exceed 100000")
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = defaultDiagnosticRequestBytes
	}
	if config.MaxRequestBytes > maxDiagnosticBodyBytes {
		return fmt.Errorf("max_request_bytes cannot exceed %d", maxDiagnosticBodyBytes)
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultDiagnosticResponseBytes
	}
	if config.MaxResponseBytes > maxDiagnosticBodyBytes {
		return fmt.Errorf("max_response_bytes cannot exceed %d", maxDiagnosticBodyBytes)
	}
	if config.Enabled && config.ExpiresAt.IsZero() {
		return errors.New("enabled capture requires expires_at")
	}
	if len(config.Rules) > maxDiagnosticRules {
		return fmt.Errorf("rules cannot exceed %d", maxDiagnosticRules)
	}
	seenNames := make(map[string]struct{}, len(config.Rules))
	for i := range config.Rules {
		rule := &config.Rules[i]
		rule.Name = strings.TrimSpace(rule.Name)
		if rule.Name == "" {
			return fmt.Errorf("rule %d requires a name", i)
		}
		if len(rule.Name) > 100 {
			return fmt.Errorf("rule %q name cannot exceed 100 bytes", rule.Name)
		}
		if _, exists := seenNames[rule.Name]; exists {
			return fmt.Errorf("duplicate rule name %q", rule.Name)
		}
		seenNames[rule.Name] = struct{}{}
		if len(rule.RequestedModels) == 0 && len(rule.UpstreamModels) == 0 && len(rule.ChannelIDs) == 0 &&
			len(rule.UserIDs) == 0 && len(rule.TokenIDs) == 0 && len(rule.Groups) == 0 && len(rule.Protocols) == 0 {
			return fmt.Errorf("rule %q must contain at least one filter", rule.Name)
		}
		filterValues := len(rule.RequestedModels) + len(rule.UpstreamModels) + len(rule.ChannelIDs) + len(rule.UserIDs) + len(rule.TokenIDs) + len(rule.Groups) + len(rule.Protocols)
		if filterValues > maxDiagnosticFilterValues {
			return fmt.Errorf("rule %q filter values cannot exceed %d", rule.Name, maxDiagnosticFilterValues)
		}
		if rule.SampleRate == 0 {
			rule.SampleRate = 1
		}
		if rule.SampleRate < 0 || rule.SampleRate > 1 {
			return fmt.Errorf("rule %q sample_rate must be between 0 and 1", rule.Name)
		}
	}
	return nil
}

func (m *diagnosticCaptureManager) disableConfig(message string) {
	m.lastConfigID = ""
	m.config.Store(nil)
	now := time.Now().Unix()
	last := m.lastLoadLog.Load()
	if now-last < 60 || !m.lastLoadLog.CompareAndSwap(last, now) {
		return
	}
	common.SysError(message + "; diagnostic capture disabled")
}

func (m *diagnosticCaptureManager) enqueue(item *diagnosticCaptureItem) bool {
	if item == nil {
		return false
	}
	pendingBytes := int64(len(item.requestBody) + len(item.responseBody) + 1024)
	for {
		current := m.pendingBytes.Load()
		if current+pendingBytes > m.queueMaxBytes {
			m.reportDrop(false)
			return false
		}
		if m.pendingBytes.CompareAndSwap(current, current+pendingBytes) {
			break
		}
	}
	item.pendingBytes = pendingBytes
	select {
	case m.queue <- item:
		return true
	default:
		m.pendingBytes.Add(-pendingBytes)
		m.reportDrop(true)
		return false
	}
}

func (m *diagnosticCaptureManager) reportDrop(queueFull bool) {
	if queueFull {
		m.droppedQueue.Add(1)
	} else {
		m.droppedBytes.Add(1)
	}
	now := time.Now().Unix()
	last := m.lastDropLog.Load()
	if now-last < 60 || !m.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	common.SysError(fmt.Sprintf(
		"diagnostic captures dropped: queue_full=%d pending_bytes_limit=%d",
		m.droppedQueue.Load(),
		m.droppedBytes.Load(),
	))
}

func (m *diagnosticCaptureManager) run() {
	defer m.workerWG.Done()
	reloadTicker := time.NewTicker(diagnosticCaptureReloadPeriod)
	cleanupTicker := time.NewTicker(diagnosticCaptureCleanupPeriod)
	defer reloadTicker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case item := <-m.queue:
			m.writeAndRelease(item)
		case <-reloadTicker.C:
			m.reloadConfig()
		case now := <-cleanupTicker.C:
			if err := m.cleanup(now); err != nil {
				common.SysError("diagnostic capture cleanup failed: " + err.Error())
			}
		case <-m.stop:
			for {
				select {
				case item := <-m.queue:
					m.writeAndRelease(item)
				default:
					return
				}
			}
		}
	}
}

func (m *diagnosticCaptureManager) writeAndRelease(item *diagnosticCaptureItem) {
	if item == nil {
		return
	}
	defer m.pendingBytes.Add(-item.pendingBytes)
	_, size, err := m.write(item)
	if err != nil {
		common.SysError("failed to write diagnostic capture: " + err.Error())
		return
	}
	m.fileCount.Add(1)
	m.fileBytes.Add(size)
	maxFiles, maxBytes, _, _ := m.capacitySettings()
	if m.fileCount.Load() > maxFiles || m.fileBytes.Load() > maxBytes {
		if err := m.cleanup(time.Now()); err != nil {
			common.SysError("diagnostic capture capacity cleanup failed: " + err.Error())
		}
	}
}

func (m *diagnosticCaptureManager) write(item *diagnosticCaptureItem) (string, int64, error) {
	metadataJSON, err := common.Marshal(item.metadata)
	if err != nil {
		return "", 0, fmt.Errorf("marshal diagnostic metadata: %w", err)
	}
	ruleName := safeCaptureName(item.metadata.Rule)
	if ruleName == "" {
		ruleName = "unnamed"
	}
	directory := filepath.Join(m.dir, item.metadata.CreatedAt.Format("20060102"), item.metadata.CreatedAt.Format("15"), ruleName)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", 0, fmt.Errorf("create diagnostic capture directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", 0, fmt.Errorf("secure diagnostic capture directory: %w", err)
	}
	requestID := safeCaptureName(item.metadata.RequestID)
	if requestID == "" {
		requestID = "capture"
	}
	filename := fmt.Sprintf("%s-r%d-c%d-%d.capture", requestID, item.metadata.RetryIndex, item.metadata.ChannelID, item.metadata.FinishedAt.UnixNano())
	targetPath := filepath.Join(directory, filename)
	temporary, err := os.CreateTemp(directory, ".capture-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temporary diagnostic capture: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return "", 0, fmt.Errorf("secure temporary diagnostic capture: %w", err)
	}
	writer := bufio.NewWriterSize(temporary, 64<<10)
	if _, err := writer.WriteString(diagnosticCaptureFormatHeader); err != nil {
		return "", 0, err
	}
	if _, err := writer.Write(metadataJSON); err != nil {
		return "", 0, err
	}
	if _, err := writer.WriteString(diagnosticCaptureRequestDelimiter); err != nil {
		return "", 0, err
	}
	if _, err := writer.Write(item.requestBody); err != nil {
		return "", 0, err
	}
	if _, err := writer.WriteString(diagnosticCaptureResponseDelimiter); err != nil {
		return "", 0, err
	}
	if _, err := writer.Write(item.responseBody); err != nil {
		return "", 0, err
	}
	if err := writer.Flush(); err != nil {
		return "", 0, fmt.Errorf("flush diagnostic capture: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", 0, fmt.Errorf("close diagnostic capture: %w", err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return "", 0, fmt.Errorf("publish diagnostic capture: %w", err)
	}
	keepTemporary = false
	info, err := os.Stat(targetPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat diagnostic capture: %w", err)
	}
	return targetPath, info.Size(), nil
}

func (m *diagnosticCaptureManager) capacitySettings() (int64, int64, time.Duration, int) {
	config := m.config.Load()
	if config == nil {
		return defaultDiagnosticCaptureFiles, int64(defaultDiagnosticCaptureGB) << 30, defaultDiagnosticCaptureHours * time.Hour, defaultDiagnosticCaptureBatch
	}
	return config.MaxFiles, config.MaxTotalGB << 30, time.Duration(config.RetentionHours) * time.Hour, config.DeleteBatchSize
}

func (m *diagnosticCaptureManager) cleanup(now time.Time) error {
	m.cleanupMu.Lock()
	defer m.cleanupMu.Unlock()
	files, err := scanDiagnosticCaptureFiles(m.dir)
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path < files[j].path
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	count := int64(len(files))
	var totalBytes int64
	for _, file := range files {
		totalBytes += file.size
	}
	maxFiles, maxBytes, retention, deleteBatchSize := m.capacitySettings()
	deleted := make(map[string]struct{})
	cutoff := now.Add(-retention)
	for _, file := range files {
		if !file.modTime.Before(cutoff) {
			continue
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove expired diagnostic capture %s: %w", file.path, err)
		}
		deleted[file.path] = struct{}{}
		count--
		totalBytes -= file.size
	}
	if count > maxFiles || totalBytes > maxBytes {
		deletedForCapacity := 0
		for _, file := range files {
			if _, exists := deleted[file.path]; exists {
				continue
			}
			if deletedForCapacity >= deleteBatchSize && count <= maxFiles && totalBytes <= maxBytes {
				break
			}
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old diagnostic capture %s: %w", file.path, err)
			}
			deletedForCapacity++
			count--
			totalBytes -= file.size
		}
	}
	m.fileCount.Store(count)
	m.fileBytes.Store(totalBytes)
	removeEmptyCaptureDirectories(m.dir)
	return nil
}

func scanDiagnosticCaptureFiles(root string) ([]diagnosticCaptureFile, error) {
	files := make([]diagnosticCaptureFile, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".capture" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, diagnosticCaptureFile{path: path, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	return files, err
}

func (m *diagnosticCaptureManager) close() {
	m.closeOnce.Do(func() {
		close(m.stop)
		m.workerWG.Wait()
	})
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
