package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func protectionTestConfig(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "guard.json")
	t.Setenv("XLLM_CHANNEL_PROTECTION_CONFIG", path)
	key := strings.Repeat("a", 64)
	require.NoError(t, os.WriteFile(path+".key", []byte(key), 0600))
	require.NoError(t, os.WriteFile(path, []byte(`{"enabled":true,"channel_group":"gemini","sample_groups":["busy"],"interval":300,"window":300,"slow_ms":15000,"slow_rate":0.5,"min_samples":50,"fallbacks":[{"name":"main","channels":[1,2],"min_enabled":1},{"name":"argon","channels":[3,4],"min_enabled":1}]}`), 0600))
	return path, key
}

func protectionRequest(t *testing.T, method, key string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := common.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/api/xllm/channel-protection", bytes.NewReader(raw))
	c.Request.Header.Set("Authorization", "Bearer "+key)
	c.Request.Header.Set("Content-Type", "application/json")
	if method == http.MethodGet {
		GetXLLMChannelProtection(c)
	} else {
		ApplyXLLMChannelProtection(c)
	}
	var response map[string]any
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response), w.Body.String())
	return w.Code, response
}

func TestXLLMProtectionAuthenticationFailsClosed(t *testing.T) {
	path, key := protectionTestConfig(t)
	for _, value := range []string{"", "wrong", key + "x"} {
		status, _ := protectionRequest(t, http.MethodPost, value, nil)
		require.Equal(t, http.StatusUnauthorized, status)
	}
	require.NoError(t, os.WriteFile(path+".key", []byte(strings.Repeat("b", 64)), 0600))
	status, _ := protectionRequest(t, http.MethodPost, key, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	require.NoError(t, os.Remove(path+".key"))
	status, _ = protectionRequest(t, http.MethodPost, key, nil)
	require.Equal(t, http.StatusUnauthorized, status)
}

func TestXLLMProtectionDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("test database unavailable")
			}
			db, _ := newAuditTestDatabase(t, dialect.kind, dsn)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			oldMemory, oldRedis := common.MemoryCacheEnabled, common.RedisEnabled
			model.DB, model.LOG_DB = db, db
			common.MemoryCacheEnabled, common.RedisEnabled = false, false
			kind := common.DatabaseTypeSQLite
			if dialect.kind == "mysql" {
				kind = common.DatabaseTypeMySQL
			}
			if dialect.kind == "postgres" {
				kind = common.DatabaseTypePostgreSQL
			}
			common.SetDatabaseTypes(kind, kind)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.AuditLog{}))
			var version string
			query := "SELECT version()"
			if dialect.kind == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.MemoryCacheEnabled, common.RedisEnabled = oldMemory, oldRedis
				common.SetDatabaseTypes(oldMain, oldLog)
				connection, err := db.DB()
				if err == nil {
					require.NoError(t, connection.Close())
				}
			})
			path, key := protectionTestConfig(t)
			for i := range 10 {
				id := i + 1
				group, status := "gemini", common.ChannelStatusEnabled
				if id == 5 {
					group = "unrelated"
				}
				if id == 6 {
					status = common.ChannelStatusManuallyDisabled
				}
				switch id {
				case 7, 8:
					group = "codex"
				case 9:
					group = "gemini,codex"
				case 10:
					group = "codex-extra"
				}
				ch := model.Channel{Id: id, Name: "test", Type: 1, Key: "secret-must-never-appear", Group: group, Models: "model", Status: status}
				require.NoError(t, db.Create(&ch).Error)
				require.NoError(t, ch.AddAbilities(db))
			}
			status, snapshot := protectionRequest(t, http.MethodGet, key, nil)
			require.Equal(t, http.StatusOK, status)
			encoded, _ := common.Marshal(snapshot)
			require.NotContains(t, string(encoded), "secret-must-never-appear")
			require.NotContains(t, string(encoded), key)
			assert.Len(t, snapshot["channels"], 6)
			now := time.Now().Unix()
			evidence := func(id int) xllmProtectionEvidence {
				return xllmProtectionEvidence{ChannelID: id, Total: 100, Slow: 80, WindowStart: now - 300, WindowEnd: now}
			}
			bad := evidence(1)
			bad.Slow = 50
			_, resp := protectionRequest(t, http.MethodPost, key, map[string]any{"version": snapshot["version"], "candidates": []xllmProtectionEvidence{bad}})
			require.Contains(t, resp["results"].([]any)[0], "result")
			require.Equal(t, "invalid_evidence", resp["results"].([]any)[0].(map[string]any)["result"])
			candidates := []xllmProtectionEvidence{evidence(1), evidence(2), evidence(3), evidence(4), evidence(5), evidence(6), evidence(7)}
			_, resp = protectionRequest(t, http.MethodPost, key, map[string]any{"version": snapshot["version"], "candidates": candidates})
			expected := []string{"disabled", "fallback_retained", "disabled", "fallback_retained", "channel_changed", "channel_changed", "channel_changed"}
			for i, item := range resp["results"].([]any) {
				require.Equal(t, expected[i], item.(map[string]any)["result"])
			}
			for id, want := range map[int]int{1: 3, 2: 1, 3: 3, 4: 1, 5: 1, 6: 2} {
				var ch model.Channel
				require.NoError(t, db.First(&ch, id).Error)
				require.Equal(t, want, ch.Status)
				var abilities []model.Ability
				require.NoError(t, db.Where("channel_id = ?", id).Find(&abilities).Error)
				for _, a := range abilities {
					require.Equal(t, want == 1, a.Enabled)
				}
			}
			// A replay never reclassifies a manually disabled channel as recoverable.
			require.True(t, model.UpdateChannelStatus(1, "", common.ChannelStatusManuallyDisabled, "operator"))
			require.False(t, model.AutoDisableChannelIfCurrent(1, 0, "stale"))
			var ch model.Channel
			require.NoError(t, db.First(&ch, 1).Error)
			require.Equal(t, common.ChannelStatusManuallyDisabled, ch.Status)
			old := common.AutomaticEnableChannelEnabled
			common.AutomaticEnableChannelEnabled = true
			t.Cleanup(func() { common.AutomaticEnableChannelEnabled = old })
			require.True(t, service.ShouldEnableChannel(nil, common.ChannelStatusAutoDisabled))
			require.False(t, service.ShouldEnableChannel(nil, common.ChannelStatusManuallyDisabled))
			// Recovery refreshes status_time; old measurements are rejected.
			require.True(t, model.UpdateChannelStatus(3, "", common.ChannelStatusEnabled, ""))
			require.False(t, model.AutoDisableChannelIfCurrent(3, 0, "old window"))
			status, _ = protectionRequest(t, http.MethodPost, key, map[string]any{"version": "stale", "candidates": candidates})
			require.Equal(t, http.StatusConflict, status)

			// Adding a second group hot-reloads its scope and keeps separate fallbacks.
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			var policy xllmProtectionConfig
			require.NoError(t, common.Unmarshal(raw, &policy))
			policy.ChannelGroups = []string{"gemini", "codex"}
			policy.Fallbacks = append(policy.Fallbacks, xllmProtectionFallback{Name: "codex", Channels: []int{7, 8}, MinEnabled: 1})
			raw, err = common.Marshal(policy)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, raw, 0600))
			status, _ = protectionRequest(t, http.MethodPost, key, map[string]any{"version": snapshot["version"], "candidates": []xllmProtectionEvidence{evidence(7)}})
			assert.Equal(t, http.StatusConflict, status)
			status, snapshot = protectionRequest(t, http.MethodGet, key, nil)
			require.Equal(t, http.StatusOK, status)
			var ids []int
			for _, item := range snapshot["channels"].([]any) {
				ids = append(ids, int(item.(map[string]any)["id"].(float64)))
			}
			assert.ElementsMatch(t, []int{1, 2, 3, 4, 6, 7, 8, 9}, ids)
			now = time.Now().Unix()
			status, resp = protectionRequest(t, http.MethodPost, key, map[string]any{"version": snapshot["version"], "candidates": []xllmProtectionEvidence{evidence(7), evidence(8), evidence(10)}})
			require.Equal(t, http.StatusOK, status)
			results := resp["results"].([]any)
			require.Len(t, results, 3)
			for i, want := range []string{"disabled", "fallback_retained", "channel_changed"} {
				assert.Equal(t, want, results[i].(map[string]any)["result"])
			}
			for id, want := range map[int]int{2: 1, 4: 1, 7: 3, 8: 1, 10: 1} {
				var current model.Channel
				require.NoError(t, db.First(&current, id).Error)
				assert.Equal(t, want, current.Status)
			}

			// The explicit list replaces legacy scope rather than silently adding it.
			policy.ChannelGroups = []string{"codex"}
			raw, err = common.Marshal(policy)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, raw, 0600))
			status, snapshot = protectionRequest(t, http.MethodGet, key, nil)
			require.Equal(t, http.StatusOK, status)
			assert.Len(t, snapshot["channels"], 3)
			status, resp = protectionRequest(t, http.MethodPost, key, map[string]any{"version": snapshot["version"], "candidates": []xllmProtectionEvidence{evidence(2)}})
			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, "channel_changed", resp["results"].([]any)[0].(map[string]any)["result"])
			policy.ChannelGroups = []string{"codex", " "}
			raw, err = common.Marshal(policy)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, raw, 0600))
			status, _ = protectionRequest(t, http.MethodGet, key, nil)
			assert.Equal(t, http.StatusServiceUnavailable, status)
		})
	}
}
