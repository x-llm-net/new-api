package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileInvoiceCreditsForUser_IsIdempotentAndUsesPaidAmount(t *testing.T) {
	truncateTables(t)
	topUp := &TopUp{
		UserId:          41,
		Amount:          120,
		Money:           99.99,
		TradeNo:         "invoice-reconcile-1",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      1000,
	}
	require.NoError(t, DB.Create(topUp).Error)

	created, err := ReconcileInvoiceCreditsForUser(41, 900)
	require.NoError(t, err)
	assert.Equal(t, int64(1), created)
	created, err = ReconcileInvoiceCreditsForUser(41, 900)
	require.NoError(t, err)
	assert.Equal(t, int64(0), created)

	var credits []InvoiceCredit
	require.NoError(t, DB.Find(&credits).Error)
	require.Len(t, credits, 1)
	assert.Equal(t, int64(9999), credits[0].AmountCents)
	assert.Equal(t, "topup:"+strconv.Itoa(topUp.Id), credits[0].SourceRef)
}

func TestReconcileInvoiceCreditsForUser_FiltersProviderStatusAndCutoff(t *testing.T) {
	truncateTables(t)
	topUps := []TopUp{
		{
			UserId: 71, Money: 100, TradeNo: "invoice-eligible",
			PaymentProvider: PaymentProviderEpay,
			Status:          common.TopUpStatusSuccess,
			CreateTime:      2000,
		},
		{
			UserId: 71, Money: 200, TradeNo: "invoice-other-provider",
			PaymentProvider: PaymentProviderStripe,
			Status:          common.TopUpStatusSuccess,
			CreateTime:      2000,
		},
		{
			UserId: 71, Money: 300, TradeNo: "invoice-pending",
			PaymentProvider: PaymentProviderEpay,
			Status:          common.TopUpStatusPending,
			CreateTime:      2000,
		},
		{
			UserId: 71, Money: 400, TradeNo: "invoice-before-cutoff",
			PaymentProvider: PaymentProviderEpay,
			Status:          common.TopUpStatusSuccess,
			CreateTime:      999,
		},
	}
	require.NoError(t, DB.Create(&topUps).Error)

	created, err := ReconcileInvoiceCreditsForUser(71, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), created)

	var credits []InvoiceCredit
	require.NoError(t, DB.Find(&credits).Error)
	require.Len(t, credits, 1)
	assert.Equal(t, int64(10000), credits[0].AmountCents)
}

func TestCreateInvoiceApplication_AllocatesFIFOAndCancelReleasesAmount(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]InvoiceCredit{
		{UserId: 52, SourceType: "topup", SourceRef: "topup:101", AmountCents: 10000, Status: InvoiceCreditStatusActive, OccurredAt: 100},
		{UserId: 52, SourceType: "topup", SourceRef: "topup:102", AmountCents: 20000, Status: InvoiceCreditStatusActive, OccurredAt: 200},
	}).Error)

	params := CreateInvoiceApplicationParams{
		UserId:         52,
		AmountCents:    25000,
		TitleType:      InvoiceTitleTypeCompany,
		TitleName:      "Example Company",
		TaxNumber:      "91320000TEST",
		Email:          "finance@example.com",
		InvoiceContent: "Technical service",
		IdempotencyKey: "invoice-create-idempotency",
	}
	application, created, err := CreateInvoiceApplication(params)
	require.NoError(t, err)
	assert.True(t, created)

	var allocations []InvoiceAllocation
	require.NoError(t, DB.Order("credit_id asc").Find(&allocations).Error)
	require.Len(t, allocations, 2)
	assert.Equal(t, int64(10000), allocations[0].AmountCents)
	assert.Equal(t, int64(15000), allocations[1].AmountCents)

	duplicate, created, err := CreateInvoiceApplication(params)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, application.Id, duplicate.Id)

	summary, err := GetInvoiceSummary(52, true, 10000)
	require.NoError(t, err)
	assert.Equal(t, int64(30000), summary.EligibleAmountCents)
	assert.Equal(t, int64(25000), summary.PendingAmountCents)
	assert.Equal(t, int64(5000), summary.AvailableAmountCents)

	require.NoError(t, CancelInvoiceApplication(52, application.Id))
	summary, err = GetInvoiceSummary(52, true, 10000)
	require.NoError(t, err)
	assert.Equal(t, int64(0), summary.PendingAmountCents)
	assert.Equal(t, int64(30000), summary.AvailableAmountCents)
}

func TestCreateInvoiceApplication_RejectsOverAllocation(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&InvoiceCredit{
		UserId: 63, SourceType: "topup", SourceRef: "topup:201", AmountCents: 10000, Status: InvoiceCreditStatusActive, OccurredAt: 100,
	}).Error)

	_, _, err := CreateInvoiceApplication(CreateInvoiceApplicationParams{
		UserId:         63,
		AmountCents:    10001,
		TitleType:      InvoiceTitleTypePersonal,
		TitleName:      "Example User",
		Email:          "user@example.com",
		InvoiceContent: "Technical service",
		IdempotencyKey: "invoice-over-allocation",
	})
	assert.ErrorIs(t, err, ErrInvoiceInsufficientAmount)
}

func TestInvoiceApplication_StatusTransitionsReleaseAmount(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&InvoiceCredit{
		UserId: 81, SourceType: "topup", SourceRef: "topup:301",
		AmountCents: 10000, Status: InvoiceCreditStatusActive, OccurredAt: 100,
	}).Error)

	params := CreateInvoiceApplicationParams{
		UserId:         81,
		AmountCents:    10000,
		TitleType:      InvoiceTitleTypeCompany,
		TitleName:      "Example Company",
		TaxNumber:      "91320000TEST",
		Email:          "finance@example.com",
		InvoiceContent: "Technical service",
		IdempotencyKey: "invoice-state-rejected",
	}
	rejected, _, err := CreateInvoiceApplication(params)
	require.NoError(t, err)
	require.NoError(t, RejectInvoiceApplication(rejected.Id, "invalid title"))

	summary, err := GetInvoiceSummary(81, true, 10000)
	require.NoError(t, err)
	assert.Equal(t, int64(10000), summary.AvailableAmountCents)

	params.IdempotencyKey = "invoice-state-issued"
	issued, _, err := CreateInvoiceApplication(params)
	require.NoError(t, err)
	require.NoError(t, IssueInvoiceApplication(issued.Id, "INV-001", "0123456789abcdef0123456789abcdef", 200))

	summary, err = GetInvoiceSummary(81, true, 10000)
	require.NoError(t, err)
	assert.Equal(t, int64(10000), summary.IssuedAmountCents)
	assert.Zero(t, summary.AvailableAmountCents)
	assert.ErrorIs(t, CancelInvoiceApplication(81, issued.Id), ErrInvoiceApplicationState)

	require.NoError(t, RedFlushInvoiceApplication(issued.Id, 300))
	summary, err = GetInvoiceSummary(81, true, 10000)
	require.NoError(t, err)
	assert.Zero(t, summary.IssuedAmountCents)
	assert.Equal(t, int64(10000), summary.AvailableAmountCents)
	assert.ErrorIs(t, RedFlushInvoiceApplication(issued.Id, 400), ErrInvoiceApplicationState)
}

func TestIssueInvoiceApplication_RejectsDuplicateInvoiceNumber(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]InvoiceCredit{
		{UserId: 91, SourceType: "topup", SourceRef: "topup:401", AmountCents: 10000, Status: InvoiceCreditStatusActive, OccurredAt: 100},
		{UserId: 91, SourceType: "topup", SourceRef: "topup:402", AmountCents: 10000, Status: InvoiceCreditStatusActive, OccurredAt: 200},
	}).Error)

	params := CreateInvoiceApplicationParams{
		UserId:         91,
		AmountCents:    10000,
		TitleType:      InvoiceTitleTypeCompany,
		TitleName:      "Example Company",
		TaxNumber:      "91320000TEST",
		Email:          "finance@example.com",
		InvoiceContent: "Technical service",
		IdempotencyKey: "invoice-number-first",
	}
	first, _, err := CreateInvoiceApplication(params)
	require.NoError(t, err)
	params.IdempotencyKey = "invoice-number-second"
	second, _, err := CreateInvoiceApplication(params)
	require.NoError(t, err)

	require.NoError(t, IssueInvoiceApplication(first.Id, "Inv-Duplicate", "0123456789abcdef0123456789abcdef", 200))
	assert.ErrorIs(t, IssueInvoiceApplication(second.Id, "INV-DUPLICATE", "abcdef0123456789abcdef0123456789", 300), ErrInvoiceNumberExists)

	var unchanged InvoiceApplication
	require.NoError(t, DB.First(&unchanged, second.Id).Error)
	assert.Equal(t, InvoiceApplicationStatusPending, unchanged.Status)
	assert.Empty(t, unchanged.InvoiceNumber)
	assert.Nil(t, unchanged.InvoiceNumberKey)

	require.NoError(t, RedFlushInvoiceApplication(first.Id, 400))
	assert.ErrorIs(t, IssueInvoiceApplication(second.Id, "inv-duplicate", "abcdef0123456789abcdef0123456789", 500), ErrInvoiceNumberExists)
	require.NoError(t, IssueInvoiceApplication(second.Id, "INV-UNIQUE", "abcdef0123456789abcdef0123456789", 600))
}
