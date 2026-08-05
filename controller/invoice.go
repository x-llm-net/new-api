package controller

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type createInvoiceApplicationRequest struct {
	AmountCents    int64  `json:"amount_cents"`
	TitleType      string `json:"title_type"`
	TitleName      string `json:"title_name"`
	TaxNumber      string `json:"tax_number"`
	Email          string `json:"email"`
	IdempotencyKey string `json:"idempotency_key"`
}

type rejectInvoiceApplicationRequest struct {
	Reason string `json:"reason"`
}

type updateInvoiceConfigRequest struct {
	Enabled         bool   `json:"enabled"`
	MinAmountCents  int64  `json:"min_amount_cents"`
	HistoryCutoff   int64  `json:"history_cutoff"`
	InvoiceContent  string `json:"invoice_content"`
	OperatorUserIDs []int  `json:"operator_user_ids"`
}

func GetInvoiceSummary(c *gin.Context) {
	userID := c.GetInt("id")
	setting := operation_setting.GetInvoiceSetting()
	if setting.Enabled {
		if setting.HistoryCutoff <= 0 {
			common.ApiErrorMsg(c, "开票功能尚未完成起始时间配置")
			return
		}
		if _, err := model.ReconcileInvoiceCreditsForUser(userID, setting.HistoryCutoff); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	summary, err := model.GetInvoiceSummary(userID, setting.Enabled, setting.MinAmountCents)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func ListMyInvoiceApplications(c *gin.Context) {
	page, size := invoicePagination(c)
	applications, total, err := model.ListUserInvoiceApplications(c.GetInt("id"), (page-1)*size, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": applications,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func CreateInvoiceApplication(c *gin.Context) {
	setting := operation_setting.GetInvoiceSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "开票申请暂未开放")
		return
	}
	if setting.HistoryCutoff <= 0 || strings.TrimSpace(setting.InvoiceContent) == "" {
		common.ApiErrorMsg(c, "开票功能配置不完整")
		return
	}
	var request createInvoiceApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "申请参数无效")
		return
	}
	request.TitleType = strings.TrimSpace(request.TitleType)
	request.TitleName = strings.TrimSpace(request.TitleName)
	request.TaxNumber = strings.TrimSpace(request.TaxNumber)
	request.Email = strings.TrimSpace(request.Email)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.AmountCents < setting.MinAmountCents {
		common.ApiErrorMsg(c, fmt.Sprintf("申请金额不能低于 %.2f 元", float64(setting.MinAmountCents)/100))
		return
	}
	if request.TitleType != model.InvoiceTitleTypePersonal && request.TitleType != model.InvoiceTitleTypeCompany {
		common.ApiErrorMsg(c, "发票抬头类型无效")
		return
	}
	if request.TitleName == "" || utf8.RuneCountInString(request.TitleName) > 191 {
		common.ApiErrorMsg(c, "发票抬头不能为空且不能超过 191 个字符")
		return
	}
	if request.TitleType == model.InvoiceTitleTypeCompany && request.TaxNumber == "" {
		common.ApiErrorMsg(c, "企业抬头必须填写税号")
		return
	}
	if request.TitleType == model.InvoiceTitleTypePersonal {
		request.TaxNumber = ""
	}
	if utf8.RuneCountInString(request.TaxNumber) > 64 {
		common.ApiErrorMsg(c, "税号不能超过 64 个字符")
		return
	}
	address, err := mail.ParseAddress(request.Email)
	if err != nil || !strings.EqualFold(address.Address, request.Email) || utf8.RuneCountInString(request.Email) > 191 {
		common.ApiErrorMsg(c, "电子邮箱格式无效")
		return
	}
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 128 {
		common.ApiErrorMsg(c, "重复提交标识无效")
		return
	}

	userID := c.GetInt("id")
	if _, err := model.ReconcileInvoiceCreditsForUser(userID, setting.HistoryCutoff); err != nil {
		common.ApiError(c, err)
		return
	}
	application, _, err := model.CreateInvoiceApplication(model.CreateInvoiceApplicationParams{
		UserId:         userID,
		AmountCents:    request.AmountCents,
		TitleType:      request.TitleType,
		TitleName:      request.TitleName,
		TaxNumber:      request.TaxNumber,
		Email:          request.Email,
		InvoiceContent: strings.TrimSpace(setting.InvoiceContent),
		IdempotencyKey: request.IdempotencyKey,
	})
	if errors.Is(err, model.ErrInvoiceInsufficientAmount) {
		common.ApiErrorMsg(c, "当前可开票金额不足")
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, application)
}

func CancelMyInvoiceApplication(c *gin.Context) {
	applicationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || applicationID <= 0 {
		common.ApiErrorMsg(c, "申请编号无效")
		return
	}
	if err := model.CancelInvoiceApplication(c.GetInt("id"), applicationID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DownloadMyInvoice(c *gin.Context) {
	applicationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || applicationID <= 0 {
		c.Status(http.StatusNotFound)
		return
	}
	application, err := model.GetIssuedInvoiceApplicationForUser(c.GetInt("id"), applicationID)
	if err != nil || application.InvoiceFileKey == "" {
		c.Status(http.StatusNotFound)
		return
	}
	path, err := service.InvoicePDFPath(application.InvoiceFileKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(path, fmt.Sprintf("invoice-%s.pdf", application.RequestNo))
}

func ListOperatorInvoiceApplications(c *gin.Context) {
	page, size := invoicePagination(c)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !isInvoiceApplicationStatus(status) {
		common.ApiErrorMsg(c, "申请状态无效")
		return
	}
	applications, total, err := model.ListOperatorInvoiceApplications(status, (page-1)*size, size)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": applications,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func IssueInvoiceApplication(c *gin.Context) {
	applicationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || applicationID <= 0 {
		common.ApiErrorMsg(c, "申请编号无效")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxInvoiceFileBytes+1024*1024)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		common.ApiErrorMsg(c, "请选择电子发票 PDF")
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > service.MaxInvoiceFileBytes {
		common.ApiErrorMsg(c, "电子发票 PDF 不能超过 10 MB")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(filepathExtension(fileHeader)), ".pdf") {
		common.ApiErrorMsg(c, "只允许上传 PDF 文件")
		return
	}
	invoiceNumber := strings.TrimSpace(c.PostForm("invoice_number"))
	if invoiceNumber == "" || utf8.RuneCountInString(invoiceNumber) > 128 {
		common.ApiErrorMsg(c, "发票号码不能为空且不能超过 128 个字符")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer file.Close()
	fileKey, err := service.StoreInvoicePDF(file)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.IssueInvoiceApplication(applicationID, invoiceNumber, fileKey, common.GetTimestamp()); err != nil {
		_ = service.DeleteInvoicePDF(fileKey)
		if errors.Is(err, model.ErrInvoiceNumberExists) {
			common.ApiErrorMsg(c, "发票号码已被使用")
			return
		}
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invoice.issue", map[string]interface{}{
		"application_id": applicationID,
		"invoice_number": invoiceNumber,
	})
	common.ApiSuccess(c, nil)
}

func RejectInvoiceApplication(c *gin.Context) {
	applicationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || applicationID <= 0 {
		common.ApiErrorMsg(c, "申请编号无效")
		return
	}
	var request rejectInvoiceApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "驳回参数无效")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Reason == "" || utf8.RuneCountInString(request.Reason) > 500 {
		common.ApiErrorMsg(c, "请填写不超过 500 个字符的驳回原因")
		return
	}
	if err := model.RejectInvoiceApplication(applicationID, request.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invoice.reject", map[string]interface{}{
		"application_id": applicationID,
	})
	common.ApiSuccess(c, nil)
}

func RedFlushInvoiceApplication(c *gin.Context) {
	applicationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || applicationID <= 0 {
		common.ApiErrorMsg(c, "申请编号无效")
		return
	}
	if err := model.RedFlushInvoiceApplication(applicationID, common.GetTimestamp()); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invoice.red_flush", map[string]interface{}{
		"application_id": applicationID,
	})
	common.ApiSuccess(c, nil)
}

func GetInvoiceConfig(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetInvoiceSetting())
}

func UpdateInvoiceConfig(c *gin.Context) {
	var request updateInvoiceConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "开票配置无效")
		return
	}
	request.InvoiceContent = strings.TrimSpace(request.InvoiceContent)
	if request.MinAmountCents <= 0 {
		common.ApiErrorMsg(c, "最低开票金额必须大于 0")
		return
	}
	if request.Enabled && (request.HistoryCutoff <= 0 || request.InvoiceContent == "") {
		common.ApiErrorMsg(c, "启用前必须填写起始时间和发票项目")
		return
	}
	if utf8.RuneCountInString(request.InvoiceContent) > 191 {
		common.ApiErrorMsg(c, "发票项目不能超过 191 个字符")
		return
	}
	currentSetting := operation_setting.GetInvoiceSetting()
	if request.HistoryCutoff != currentSetting.HistoryCutoff {
		hasCredits, err := model.HasInvoiceCredits()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if hasCredits {
			common.ApiErrorMsg(c, "已有开票权益后不能直接修改历史起始时间")
			return
		}
	}
	operatorIDs := make([]int, 0, len(request.OperatorUserIDs))
	seen := make(map[int]struct{}, len(request.OperatorUserIDs))
	for _, userID := range request.OperatorUserIDs {
		if userID <= 0 {
			common.ApiErrorMsg(c, "开票操作员用户 ID 无效")
			return
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		if _, err := model.GetUserById(userID, false); err != nil {
			common.ApiErrorMsg(c, fmt.Sprintf("用户 ID %d 不存在", userID))
			return
		}
		seen[userID] = struct{}{}
		operatorIDs = append(operatorIDs, userID)
	}
	operatorJSON, err := common.Marshal(operatorIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	values := map[string]string{
		"invoice_setting.enabled":           strconv.FormatBool(request.Enabled),
		"invoice_setting.min_amount_cents":  strconv.FormatInt(request.MinAmountCents, 10),
		"invoice_setting.history_cutoff":    strconv.FormatInt(request.HistoryCutoff, 10),
		"invoice_setting.invoice_content":   request.InvoiceContent,
		"invoice_setting.operator_user_ids": string(operatorJSON),
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "invoice.config_update", map[string]interface{}{
		"enabled":           request.Enabled,
		"min_amount_cents":  request.MinAmountCents,
		"history_cutoff":    request.HistoryCutoff,
		"operator_user_ids": operatorIDs,
	})
	common.ApiSuccess(c, operation_setting.GetInvoiceSetting())
}

func invoicePagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func isInvoiceApplicationStatus(status string) bool {
	switch status {
	case model.InvoiceApplicationStatusPending,
		model.InvoiceApplicationStatusIssued,
		model.InvoiceApplicationStatusRejected,
		model.InvoiceApplicationStatusCancelled,
		model.InvoiceApplicationStatusRedFlushed:
		return true
	default:
		return false
	}
}

func filepathExtension(header *multipart.FileHeader) string {
	filename := strings.TrimSpace(header.Filename)
	index := strings.LastIndex(filename, ".")
	if index < 0 {
		return ""
	}
	return filename[index:]
}
