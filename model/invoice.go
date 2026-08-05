package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvoiceCreditStatusActive = "active"
	InvoiceCreditStatusVoided = "voided"

	InvoiceApplicationStatusPending    = "pending"
	InvoiceApplicationStatusIssued     = "issued"
	InvoiceApplicationStatusRejected   = "rejected"
	InvoiceApplicationStatusCancelled  = "cancelled"
	InvoiceApplicationStatusRedFlushed = "red_flushed"

	InvoiceTitleTypePersonal = "personal"
	InvoiceTitleTypeCompany  = "company"
)

var (
	ErrInvoiceInsufficientAmount = errors.New("insufficient invoiceable amount")
	ErrInvoiceApplicationState   = errors.New("invalid invoice application state")
	ErrInvoiceApplicationMissing = errors.New("invoice application not found")
	ErrInvoiceNumberExists       = errors.New("invoice number already exists")
)

type InvoiceCredit struct {
	Id          int64  `json:"id"`
	UserId      int    `json:"user_id" gorm:"index"`
	SourceType  string `json:"source_type" gorm:"type:varchar(32);not null"`
	SourceRef   string `json:"source_ref" gorm:"type:varchar(191);uniqueIndex;not null"`
	AmountCents int64  `json:"amount_cents" gorm:"not null"`
	Status      string `json:"status" gorm:"type:varchar(32);index;not null"`
	OccurredAt  int64  `json:"occurred_at" gorm:"index;not null"`
	Reason      string `json:"reason,omitempty" gorm:"type:varchar(255)"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type InvoiceApplication struct {
	Id               int64   `json:"id"`
	UserId           int     `json:"user_id" gorm:"index;uniqueIndex:idx_invoice_user_idempotency,priority:1"`
	RequestNo        string  `json:"request_no" gorm:"type:varchar(64);uniqueIndex;not null"`
	IdempotencyKey   string  `json:"-" gorm:"type:varchar(128);uniqueIndex:idx_invoice_user_idempotency,priority:2"`
	AmountCents      int64   `json:"amount_cents" gorm:"not null"`
	Status           string  `json:"status" gorm:"type:varchar(32);index;not null"`
	TitleType        string  `json:"title_type" gorm:"type:varchar(32);not null"`
	TitleName        string  `json:"title_name" gorm:"type:varchar(191);not null"`
	TaxNumber        string  `json:"tax_number,omitempty" gorm:"type:varchar(64)"`
	Email            string  `json:"email" gorm:"type:varchar(191);not null"`
	InvoiceContent   string  `json:"invoice_content" gorm:"type:varchar(191);not null"`
	InvoiceNumber    string  `json:"invoice_number,omitempty" gorm:"type:varchar(128)"`
	InvoiceNumberKey *string `json:"-" gorm:"type:varchar(128);uniqueIndex"`
	InvoiceFileKey   string  `json:"-" gorm:"type:varchar(128)"`
	RejectReason     string  `json:"reject_reason,omitempty" gorm:"type:varchar(500)"`
	IssuedAt         int64   `json:"issued_at,omitempty"`
	RedFlushedAt     int64   `json:"red_flushed_at,omitempty"`
	CreatedAt        int64   `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt        int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type InvoiceAllocation struct {
	Id            int64 `json:"id"`
	ApplicationId int64 `json:"application_id" gorm:"index;uniqueIndex:idx_invoice_application_credit,priority:1"`
	CreditId      int64 `json:"credit_id" gorm:"index;uniqueIndex:idx_invoice_application_credit,priority:2"`
	AmountCents   int64 `json:"amount_cents" gorm:"not null"`
	CreatedAt     int64 `json:"created_at" gorm:"autoCreateTime"`
}

type InvoiceSummary struct {
	Enabled              bool  `json:"enabled"`
	EligibleAmountCents  int64 `json:"eligible_amount_cents"`
	PendingAmountCents   int64 `json:"pending_amount_cents"`
	IssuedAmountCents    int64 `json:"issued_amount_cents"`
	AvailableAmountCents int64 `json:"available_amount_cents"`
	MinAmountCents       int64 `json:"min_amount_cents"`
}

type CreateInvoiceApplicationParams struct {
	UserId         int
	AmountCents    int64
	TitleType      string
	TitleName      string
	TaxNumber      string
	Email          string
	InvoiceContent string
	IdempotencyKey string
}

type InvoiceApplicationView struct {
	InvoiceApplication
	Username string `json:"username,omitempty"`
	HasFile  bool   `json:"has_file" gorm:"-"`
}

func ReconcileInvoiceCreditsForUser(userID int, cutoff int64) (int64, error) {
	if userID <= 0 || cutoff <= 0 {
		return 0, nil
	}
	var topUps []TopUp
	err := DB.Where(
		"user_id = ? AND payment_provider = ? AND status = ? AND create_time >= ? AND money > 0",
		userID,
		PaymentProviderEpay,
		common.TopUpStatusSuccess,
		cutoff,
	).Order("id asc").Find(&topUps).Error
	if err != nil {
		return 0, err
	}

	var created int64
	for _, topUp := range topUps {
		amountCents := decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
		if amountCents <= 0 {
			continue
		}
		occurredAt := topUp.CompleteTime
		if occurredAt <= 0 {
			occurredAt = topUp.CreateTime
		}
		credit := InvoiceCredit{
			UserId:      topUp.UserId,
			SourceType:  "topup",
			SourceRef:   fmt.Sprintf("topup:%d", topUp.Id),
			AmountCents: amountCents,
			Status:      InvoiceCreditStatusActive,
			OccurredAt:  occurredAt,
		}
		result := DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "source_ref"}},
			DoNothing: true,
		}).Create(&credit)
		if result.Error != nil {
			return created, result.Error
		}
		created += result.RowsAffected
	}
	return created, nil
}

func HasInvoiceCredits() (bool, error) {
	var count int64
	if err := DB.Model(&InvoiceCredit{}).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func GetInvoiceSummary(userID int, enabled bool, minAmountCents int64) (*InvoiceSummary, error) {
	summary := &InvoiceSummary{Enabled: enabled, MinAmountCents: minAmountCents}
	if err := DB.Model(&InvoiceCredit{}).
		Where("user_id = ? AND status = ?", userID, InvoiceCreditStatusActive).
		Select("COALESCE(SUM(amount_cents), 0)").Scan(&summary.EligibleAmountCents).Error; err != nil {
		return nil, err
	}

	type statusTotal struct {
		Status string
		Amount int64
	}
	var totals []statusTotal
	err := DB.Table("invoice_allocations AS allocation").
		Select("application.status AS status, COALESCE(SUM(allocation.amount_cents), 0) AS amount").
		Joins("JOIN invoice_applications AS application ON application.id = allocation.application_id").
		Where("application.user_id = ? AND application.status IN ?", userID, []string{
			InvoiceApplicationStatusPending,
			InvoiceApplicationStatusIssued,
		}).
		Group("application.status").Scan(&totals).Error
	if err != nil {
		return nil, err
	}
	for _, total := range totals {
		switch total.Status {
		case InvoiceApplicationStatusPending:
			summary.PendingAmountCents = total.Amount
		case InvoiceApplicationStatusIssued:
			summary.IssuedAmountCents = total.Amount
		}
	}
	summary.AvailableAmountCents = summary.EligibleAmountCents - summary.PendingAmountCents - summary.IssuedAmountCents
	if summary.AvailableAmountCents < 0 {
		summary.AvailableAmountCents = 0
	}
	return summary, nil
}

func CreateInvoiceApplication(params CreateInvoiceApplicationParams) (*InvoiceApplication, bool, error) {
	var application InvoiceApplication
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND idempotency_key = ?", params.UserId, params.IdempotencyKey).
			First(&application).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var credits []InvoiceCredit
		if err := lockForUpdate(tx).
			Where("user_id = ? AND status = ?", params.UserId, InvoiceCreditStatusActive).
			Order("occurred_at asc, id asc").Find(&credits).Error; err != nil {
			return err
		}
		if len(credits) == 0 {
			return ErrInvoiceInsufficientAmount
		}

		creditIDs := make([]int64, 0, len(credits))
		for _, credit := range credits {
			creditIDs = append(creditIDs, credit.Id)
		}
		type usedAmount struct {
			CreditId int64
			Amount   int64
		}
		var usedAmounts []usedAmount
		if err := tx.Table("invoice_allocations AS allocation").
			Select("allocation.credit_id AS credit_id, COALESCE(SUM(allocation.amount_cents), 0) AS amount").
			Joins("JOIN invoice_applications AS application ON application.id = allocation.application_id").
			Where("allocation.credit_id IN ? AND application.status IN ?", creditIDs, []string{
				InvoiceApplicationStatusPending,
				InvoiceApplicationStatusIssued,
			}).Group("allocation.credit_id").Scan(&usedAmounts).Error; err != nil {
			return err
		}
		usedByCredit := make(map[int64]int64, len(usedAmounts))
		for _, used := range usedAmounts {
			usedByCredit[used.CreditId] = used.Amount
		}

		remaining := params.AmountCents
		allocations := make([]InvoiceAllocation, 0, len(credits))
		for _, credit := range credits {
			available := credit.AmountCents - usedByCredit[credit.Id]
			if available <= 0 {
				continue
			}
			allocated := available
			if allocated > remaining {
				allocated = remaining
			}
			allocations = append(allocations, InvoiceAllocation{
				CreditId:    credit.Id,
				AmountCents: allocated,
			})
			remaining -= allocated
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return ErrInvoiceInsufficientAmount
		}

		application = InvoiceApplication{
			UserId:         params.UserId,
			RequestNo:      fmt.Sprintf("INV%d%s", common.GetTimestamp(), strings.ToUpper(common.GetRandomString(6))),
			IdempotencyKey: params.IdempotencyKey,
			AmountCents:    params.AmountCents,
			Status:         InvoiceApplicationStatusPending,
			TitleType:      params.TitleType,
			TitleName:      params.TitleName,
			TaxNumber:      params.TaxNumber,
			Email:          params.Email,
			InvoiceContent: params.InvoiceContent,
		}
		if err := tx.Create(&application).Error; err != nil {
			return err
		}
		for i := range allocations {
			allocations[i].ApplicationId = application.Id
		}
		if err := tx.Create(&allocations).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return &application, created, err
}

func ListUserInvoiceApplications(userID int, offset int, limit int) ([]InvoiceApplicationView, int64, error) {
	var total int64
	if err := DB.Model(&InvoiceApplication{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var applications []InvoiceApplication
	if err := DB.Where("user_id = ?", userID).Order("id desc").Offset(offset).Limit(limit).Find(&applications).Error; err != nil {
		return nil, 0, err
	}
	views := make([]InvoiceApplicationView, 0, len(applications))
	for _, application := range applications {
		views = append(views, InvoiceApplicationView{
			InvoiceApplication: application,
			HasFile:            application.InvoiceFileKey != "",
		})
	}
	return views, total, nil
}

func ListOperatorInvoiceApplications(status string, offset int, limit int) ([]InvoiceApplicationView, int64, error) {
	query := DB.Model(&InvoiceApplication{})
	if status != "" {
		query = query.Where("invoice_applications.status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var applications []InvoiceApplicationView
	err := query.Select("invoice_applications.*, users.username AS username").
		Joins("LEFT JOIN users ON users.id = invoice_applications.user_id").
		Order("invoice_applications.id desc").Offset(offset).Limit(limit).Scan(&applications).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range applications {
		applications[i].HasFile = applications[i].InvoiceFileKey != ""
	}
	return applications, total, nil
}

func CancelInvoiceApplication(userID int, applicationID int64) error {
	return updateInvoiceApplicationStatus(applicationID, userID, InvoiceApplicationStatusPending, InvoiceApplicationStatusCancelled, nil)
}

func RejectInvoiceApplication(applicationID int64, reason string) error {
	return updateInvoiceApplicationStatus(applicationID, 0, InvoiceApplicationStatusPending, InvoiceApplicationStatusRejected, map[string]interface{}{
		"reject_reason": reason,
	})
}

func IssueInvoiceApplication(applicationID int64, invoiceNumber string, fileKey string, issuedAt int64) error {
	invoiceNumber = strings.TrimSpace(invoiceNumber)
	invoiceNumberKey := strings.ToUpper(invoiceNumber)
	var count int64
	if err := DB.Model(&InvoiceApplication{}).Where("invoice_number_key = ?", invoiceNumberKey).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrInvoiceNumberExists
	}

	err := updateInvoiceApplicationStatus(applicationID, 0, InvoiceApplicationStatusPending, InvoiceApplicationStatusIssued, map[string]interface{}{
		"invoice_number":     invoiceNumber,
		"invoice_number_key": invoiceNumberKey,
		"invoice_file_key":   fileKey,
		"issued_at":          issuedAt,
		"reject_reason":      "",
	})
	if err == nil {
		return nil
	}

	count = 0
	if queryErr := DB.Model(&InvoiceApplication{}).Where("invoice_number_key = ?", invoiceNumberKey).Count(&count).Error; queryErr == nil && count > 0 {
		return ErrInvoiceNumberExists
	}
	return err
}

func RedFlushInvoiceApplication(applicationID int64, redFlushedAt int64) error {
	return updateInvoiceApplicationStatus(applicationID, 0, InvoiceApplicationStatusIssued, InvoiceApplicationStatusRedFlushed, map[string]interface{}{
		"red_flushed_at": redFlushedAt,
	})
}

func updateInvoiceApplicationStatus(applicationID int64, userID int, fromStatus string, toStatus string, updates map[string]interface{}) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var application InvoiceApplication
		query := lockForUpdate(tx).Where("id = ?", applicationID)
		if userID > 0 {
			query = query.Where("user_id = ?", userID)
		}
		if err := query.First(&application).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvoiceApplicationMissing
			}
			return err
		}
		if application.Status != fromStatus {
			return ErrInvoiceApplicationState
		}
		if updates == nil {
			updates = map[string]interface{}{}
		}
		updates["status"] = toStatus
		return tx.Model(&application).Updates(updates).Error
	})
}

func GetIssuedInvoiceApplicationForUser(userID int, applicationID int64) (*InvoiceApplication, error) {
	var application InvoiceApplication
	err := DB.Where("id = ? AND user_id = ? AND status = ?", applicationID, userID, InvoiceApplicationStatusIssued).
		First(&application).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvoiceApplicationMissing
	}
	return &application, err
}
