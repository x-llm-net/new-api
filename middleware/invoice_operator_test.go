package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestInvoiceOperatorAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setting := operation_setting.GetInvoiceSetting()
	previous := append([]int(nil), setting.OperatorUserIDs...)
	setting.OperatorUserIDs = []int{42}
	t.Cleanup(func() {
		setting.OperatorUserIDs = previous
	})

	tests := []struct {
		name       string
		userID     int
		role       int
		wantStatus int
	}{
		{
			name:       "configured operator",
			userID:     42,
			role:       common.RoleCommonUser,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "root user",
			userID:     100,
			role:       common.RoleRootUser,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "ordinary user",
			userID:     7,
			role:       common.RoleCommonUser,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("id", test.userID)
				c.Set("role", test.role)
				c.Next()
			})
			router.Use(InvoiceOperatorAuth())
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			router.ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodGet, "/", nil),
			)
			assert.Equal(t, test.wantStatus, recorder.Code)
		})
	}
}
