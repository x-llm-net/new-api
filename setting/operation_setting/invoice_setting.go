package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type InvoiceSetting struct {
	Enabled         bool   `json:"enabled"`
	MinAmountCents  int64  `json:"min_amount_cents"`
	HistoryCutoff   int64  `json:"history_cutoff"`
	InvoiceContent  string `json:"invoice_content"`
	OperatorUserIDs []int  `json:"operator_user_ids"`
}

var invoiceSetting = InvoiceSetting{
	Enabled:        false,
	MinAmountCents: 10000,
}

func init() {
	config.GlobalConfig.Register("invoice_setting", &invoiceSetting)
}

func GetInvoiceSetting() *InvoiceSetting {
	return &invoiceSetting
}

func IsInvoiceOperator(userID int, role int) bool {
	if role >= common.RoleRootUser {
		return true
	}
	for _, operatorID := range invoiceSetting.OperatorUserIDs {
		if operatorID == userID {
			return true
		}
	}
	return false
}
