package service

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	errorCaptureContextKey     = "xllm_error_capture"
	errorCaptureFormatHeader   = "XLLM-ERROR-CAPTURE-V1\n"
	errorCaptureBodyDelimiter  = "\n---REQUEST-BODY---\n"
	errorCaptureCleanupPeriod  = 10 * time.Minute
	defaultErrorCaptureDir     = "/data/error-captures"
	defaultErrorCaptureQueue   = 1000
	defaultErrorCaptureQueueMB = 256
	defaultErrorCaptureFiles   = 100000
	defaultErrorCaptureGB      = 50
	defaultErrorCaptureHours   = 24
	defaultErrorCaptureBatch   = 1000
)

type errorCaptureConfig struct {
	Enabled         bool
	Dir             string
	QueueSize       int
	QueueMaxBytes   int64
	MaxFiles        int64
	MaxBytes        int64
	Retention       time.Duration
	DeleteBatchSize int
}

type errorCaptureAttempt struct {
	Retry              int             `json:"retry"`
	ChannelID          int             `json:"channel_id"`
	ChannelName        string          `json:"channel_name"`
	ChannelType        int             `json:"channel_type"`
	StatusCode         int             `json:"status_code"`
	ErrorType          types.ErrorType `json:"error_type"`
	ErrorCode          types.ErrorCode `json:"error_code"`
	Error              string          `json:"error"`
	UpstreamRequestID  string          `json:"upstream_request_id,omitempty"`
	ResponseBody       string          `json:"response_body,omitempty"`
	ResponseBodyBase64 string          `json:"response_body_base64,omitempty"`
}

type errorCaptureMetadata struct {
	Version          int                   `json:"version"`
	RequestID        string                `json:"request_id"`
	CreatedAt        time.Time             `json:"created_at"`
	FinishedAt       time.Time             `json:"finished_at"`
	UserID           int                   `json:"user_id"`
	Username         string                `json:"username,omitempty"`
	TokenName        string                `json:"token_name,omitempty"`
	Model            string                `json:"model,omitempty"`
	Group            string                `json:"group,omitempty"`
	Method           string                `json:"method,omitempty"`
	RequestPath      string                `json:"request_path,omitempty"`
	RequestBodyBytes int64                 `json:"request_body_bytes"`
	FinalSuccess     bool                  `json:"final_success"`
	FinalStatus      int                   `json:"final_status"`
	FinalErrorType   types.ErrorType       `json:"final_error_type,omitempty"`
	FinalErrorCode   types.ErrorCode       `json:"final_error_code,omitempty"`
	FinalError       string                `json:"final_error,omitempty"`
	Attempts         []errorCaptureAttempt `json:"attempts,omitempty"`
}

type errorCaptureState struct {
	metadata errorCaptureMetadata
}

type errorCaptureItem struct {
	metadata     errorCaptureMetadata
	body         common.BodyStorage
	pendingBytes int64
}

type errorCaptureFile struct {
	path    string
	size    int64
	modTime time.Time
}

type errorCaptureManager struct {
	config       errorCaptureConfig
	queue        chan *errorCaptureItem
	pendingBytes atomic.Int64
	fileCount    atomic.Int64
	fileBytes    atomic.Int64
	cleanupMu    sync.Mutex
	workerWG     sync.WaitGroup
	closeOnce    sync.Once
	droppedQueue atomic.Int64
	droppedBytes atomic.Int64
	lastDropLog  atomic.Int64
}

var (
	errorCaptureManagerMu sync.RWMutex
	activeErrorCapture    *errorCaptureManager
)

func loadErrorCaptureConfig() errorCaptureConfig {
	queueSize := positiveEnv("ERROR_CAPTURE_QUEUE_SIZE", defaultErrorCaptureQueue)
	queueMaxMB := positiveEnv("ERROR_CAPTURE_QUEUE_MAX_MB", defaultErrorCaptureQueueMB)
	maxFiles := positiveEnv("ERROR_CAPTURE_MAX_FILES", defaultErrorCaptureFiles)
	maxGB := positiveEnv("ERROR_CAPTURE_MAX_GB", defaultErrorCaptureGB)
	retentionHours := positiveEnv("ERROR_CAPTURE_RETENTION_HOURS", defaultErrorCaptureHours)
	deleteBatch := positiveEnv("ERROR_CAPTURE_DELETE_BATCH", defaultErrorCaptureBatch)
	dir := strings.TrimSpace(common.GetEnvOrDefaultString("ERROR_CAPTURE_DIR", defaultErrorCaptureDir))
	if dir == "" {
		dir = defaultErrorCaptureDir
	}
	return errorCaptureConfig{
		Enabled:         common.GetEnvOrDefaultBool("ERROR_CAPTURE_ENABLED", false),
		Dir:             dir,
		QueueSize:       queueSize,
		QueueMaxBytes:   int64(queueMaxMB) << 20,
		MaxFiles:        int64(maxFiles),
		MaxBytes:        int64(maxGB) << 30,
		Retention:       time.Duration(retentionHours) * time.Hour,
		DeleteBatchSize: deleteBatch,
	}
}

func positiveEnv(name string, fallback int) int {
	value := common.GetEnvOrDefault(name, fallback)
	if value <= 0 {
		common.SysError(fmt.Sprintf("%s must be positive, using default value: %d", name, fallback))
		return fallback
	}
	return value
}

func StartErrorCapture() {
	config := loadErrorCaptureConfig()
	if !config.Enabled {
		return
	}
	manager, err := newErrorCaptureManager(config)
	if err != nil {
		common.SysError("failed to start error capture: " + err.Error())
		return
	}
	errorCaptureManagerMu.Lock()
	activeErrorCapture = manager
	errorCaptureManagerMu.Unlock()
	common.SysLog(fmt.Sprintf(
		"error capture enabled: dir=%s queue=%d pending_mb=%d max_files=%d max_gb=%d retention_hours=%d",
		config.Dir,
		config.QueueSize,
		config.QueueMaxBytes>>20,
		config.MaxFiles,
		config.MaxBytes>>30,
		int(config.Retention/time.Hour),
	))
}

func newErrorCaptureManager(config errorCaptureConfig) (*errorCaptureManager, error) {
	if err := os.MkdirAll(config.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create capture directory: %w", err)
	}
	if err := os.Chmod(config.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure capture directory: %w", err)
	}
	manager := &errorCaptureManager{
		config: config,
		queue:  make(chan *errorCaptureItem, config.QueueSize),
	}
	if err := manager.cleanup(time.Now()); err != nil {
		return nil, fmt.Errorf("initial capture cleanup: %w", err)
	}
	manager.workerWG.Add(1)
	go manager.run()
	return manager, nil
}

func currentErrorCaptureManager() *errorCaptureManager {
	errorCaptureManagerMu.RLock()
	defer errorCaptureManagerMu.RUnlock()
	return activeErrorCapture
}

func BeginErrorCapture(c *gin.Context) {
	if c == nil || currentErrorCaptureManager() == nil {
		return
	}
	createdAt := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	metadata := errorCaptureMetadata{
		Version:   1,
		RequestID: c.GetString(common.RequestIdKey),
		CreatedAt: createdAt,
		UserID:    c.GetInt("id"),
		Username:  c.GetString("username"),
		TokenName: c.GetString("token_name"),
		Model:     c.GetString("original_model"),
		Group:     c.GetString("group"),
	}
	if c.Request != nil {
		metadata.Method = c.Request.Method
		if c.Request.URL != nil {
			metadata.RequestPath = c.Request.URL.Path
		}
	}
	c.Set(errorCaptureContextKey, &errorCaptureState{metadata: metadata})
}

func RecordErrorCaptureAttempt(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	if c == nil || err == nil {
		return
	}
	value, exists := c.Get(errorCaptureContextKey)
	if !exists {
		return
	}
	state, ok := value.(*errorCaptureState)
	if !ok || state == nil {
		return
	}
	responseBody := err.DiagnosticResponseBody()
	attempt := errorCaptureAttempt{
		Retry:             len(state.metadata.Attempts),
		ChannelID:         channelError.ChannelId,
		ChannelName:       channelError.ChannelName,
		ChannelType:       channelError.ChannelType,
		StatusCode:        err.StatusCode,
		ErrorType:         err.GetErrorType(),
		ErrorCode:         err.GetErrorCode(),
		Error:             err.Error(),
		UpstreamRequestID: c.GetString(common.UpstreamRequestIdKey),
	}
	if utf8.Valid(responseBody) {
		attempt.ResponseBody = string(responseBody)
	} else if len(responseBody) > 0 {
		attempt.ResponseBodyBase64 = base64.StdEncoding.EncodeToString(responseBody)
	}
	state.metadata.Attempts = append(state.metadata.Attempts, attempt)
}

func FinalizeErrorCapture(c *gin.Context, finalErr *types.NewAPIError) {
	manager := currentErrorCaptureManager()
	if c == nil || manager == nil {
		return
	}
	value, exists := c.Get(errorCaptureContextKey)
	if !exists {
		return
	}
	c.Set(errorCaptureContextKey, nil)
	state, ok := value.(*errorCaptureState)
	if !ok || state == nil {
		return
	}
	if finalErr == nil && len(state.metadata.Attempts) == 0 {
		return
	}

	state.metadata.FinishedAt = time.Now()
	state.metadata.FinalSuccess = finalErr == nil
	if c.Writer != nil {
		state.metadata.FinalStatus = c.Writer.Status()
	}
	if finalErr != nil {
		state.metadata.FinalStatus = finalErr.StatusCode
		state.metadata.FinalErrorType = finalErr.GetErrorType()
		state.metadata.FinalErrorCode = finalErr.GetErrorCode()
		state.metadata.FinalError = finalErr.Error()
	}

	var bodyStorage common.BodyStorage
	if value, ok := c.Get(common.KeyBodyStorage); ok && value != nil {
		bodyStorage, _ = value.(common.BodyStorage)
	}
	if bodyStorage != nil {
		state.metadata.RequestBodyBytes = bodyStorage.Size()
	}

	item := &errorCaptureItem{
		metadata: state.metadata,
		body:     bodyStorage,
	}
	if manager.enqueue(item) && bodyStorage != nil {
		c.Set(common.KeyBodyStorage, nil)
	}
}

func (m *errorCaptureManager) enqueue(item *errorCaptureItem) bool {
	if item == nil {
		return false
	}
	pendingBytes := item.metadata.RequestBodyBytes
	pendingBytes += int64(
		len(item.metadata.RequestID) +
			len(item.metadata.Username) +
			len(item.metadata.TokenName) +
			len(item.metadata.Model) +
			len(item.metadata.Group) +
			len(item.metadata.Method) +
			len(item.metadata.RequestPath) +
			len(item.metadata.FinalError),
	)
	for _, attempt := range item.metadata.Attempts {
		pendingBytes += int64(
			len(attempt.ChannelName) +
				len(attempt.Error) +
				len(attempt.UpstreamRequestID) +
				len(attempt.ResponseBody) +
				len(attempt.ResponseBodyBase64),
		)
	}
	if pendingBytes < 1 {
		pendingBytes = 1
	}
	for {
		current := m.pendingBytes.Load()
		if current+pendingBytes > m.config.QueueMaxBytes {
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

func (m *errorCaptureManager) reportDrop(queueFull bool) {
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
		"error captures dropped: queue_full=%d pending_bytes_limit=%d",
		m.droppedQueue.Load(),
		m.droppedBytes.Load(),
	))
}

func (m *errorCaptureManager) run() {
	defer m.workerWG.Done()
	ticker := time.NewTicker(errorCaptureCleanupPeriod)
	defer ticker.Stop()
	for {
		select {
		case item, ok := <-m.queue:
			if !ok {
				return
			}
			m.writeAndRelease(item)
		case now := <-ticker.C:
			if err := m.cleanup(now); err != nil {
				common.SysError("error capture cleanup failed: " + err.Error())
			}
		}
	}
}

func (m *errorCaptureManager) writeAndRelease(item *errorCaptureItem) {
	defer m.pendingBytes.Add(-item.pendingBytes)
	if item.body != nil {
		defer item.body.Close()
	}
	_, size, err := m.write(item)
	if err != nil {
		common.SysError("failed to write error capture: " + err.Error())
		return
	}
	m.fileCount.Add(1)
	m.fileBytes.Add(size)
	if m.fileCount.Load() > m.config.MaxFiles || m.fileBytes.Load() > m.config.MaxBytes {
		if err := m.cleanup(time.Now()); err != nil {
			common.SysError("error capture capacity cleanup failed: " + err.Error())
		}
	}
}

func (m *errorCaptureManager) write(item *errorCaptureItem) (string, int64, error) {
	metadataJSON, err := common.Marshal(item.metadata)
	if err != nil {
		return "", 0, fmt.Errorf("marshal metadata: %w", err)
	}
	directory := filepath.Join(
		m.config.Dir,
		item.metadata.CreatedAt.Format("20060102"),
		item.metadata.CreatedAt.Format("15"),
	)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", 0, fmt.Errorf("create hourly directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", 0, fmt.Errorf("secure hourly directory: %w", err)
	}

	requestID := safeCaptureName(item.metadata.RequestID)
	if requestID == "" {
		requestID = fmt.Sprintf("capture-%d", item.metadata.CreatedAt.UnixNano())
	}
	targetPath := filepath.Join(directory, requestID+".capture")
	temporary, err := os.CreateTemp(directory, ".capture-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temporary capture: %w", err)
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
		return "", 0, fmt.Errorf("secure temporary capture: %w", err)
	}

	writer := bufio.NewWriterSize(temporary, 64<<10)
	if _, err := writer.WriteString(errorCaptureFormatHeader); err != nil {
		return "", 0, err
	}
	if _, err := writer.Write(metadataJSON); err != nil {
		return "", 0, err
	}
	if _, err := writer.WriteString(errorCaptureBodyDelimiter); err != nil {
		return "", 0, err
	}
	if item.body != nil {
		if _, err := item.body.Seek(0, io.SeekStart); err != nil {
			return "", 0, fmt.Errorf("seek request body: %w", err)
		}
		if _, err := io.Copy(writer, item.body); err != nil {
			return "", 0, fmt.Errorf("write request body: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return "", 0, fmt.Errorf("flush capture: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", 0, fmt.Errorf("close capture: %w", err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return "", 0, fmt.Errorf("publish capture: %w", err)
	}
	keepTemporary = false
	info, err := os.Stat(targetPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat capture: %w", err)
	}
	return targetPath, info.Size(), nil
}

func safeCaptureName(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), ".")
}

func (m *errorCaptureManager) cleanup(now time.Time) error {
	m.cleanupMu.Lock()
	defer m.cleanupMu.Unlock()

	files, err := scanErrorCaptureFiles(m.config.Dir)
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
	deleted := make(map[string]struct{})
	cutoff := now.Add(-m.config.Retention)
	for _, file := range files {
		if !file.modTime.Before(cutoff) {
			continue
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove expired capture %s: %w", file.path, err)
		}
		deleted[file.path] = struct{}{}
		count--
		totalBytes -= file.size
	}

	if count > m.config.MaxFiles || totalBytes > m.config.MaxBytes {
		deletedForCapacity := 0
		for _, file := range files {
			if _, exists := deleted[file.path]; exists {
				continue
			}
			if deletedForCapacity >= m.config.DeleteBatchSize && count <= m.config.MaxFiles && totalBytes <= m.config.MaxBytes {
				break
			}
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove old capture %s: %w", file.path, err)
			}
			deletedForCapacity++
			count--
			totalBytes -= file.size
		}
	}
	m.fileCount.Store(count)
	m.fileBytes.Store(totalBytes)
	removeEmptyCaptureDirectories(m.config.Dir)
	return nil
}

func scanErrorCaptureFiles(root string) ([]errorCaptureFile, error) {
	files := make([]errorCaptureFile, 0)
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
		files = append(files, errorCaptureFile{path: path, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	return files, err
}

func removeEmptyCaptureDirectories(root string) {
	directories := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() && path != root {
			directories = append(directories, path)
		}
		return nil
	})
	sort.Slice(directories, func(i, j int) bool {
		return len(directories[i]) > len(directories[j])
	})
	for _, directory := range directories {
		_ = os.Remove(directory)
	}
}

func (m *errorCaptureManager) close() {
	m.closeOnce.Do(func() {
		close(m.queue)
		m.workerWG.Wait()
	})
}
