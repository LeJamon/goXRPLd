package methods

import (
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/server/utils"
)

// AccountInfoRequest represents the request structure for the "account_info" method.
type AccountInfoRequest struct {
	Account     string `json:"account"`
	LedgerIndex string `json:"ledger_index"`
	Queue       bool   `json:"queue"`
}

// AccountInfoResponse represents the response structure for the "account_info" method.
type AccountInfoResponse struct {
	Account       string `json:"account"`
	LedgerCurrent int    `json:"ledger_current"`
	LedgerIndex   int    `json:"ledger_index"`
	LedgerHash    string `json:"ledger_hash"`
	Status        string `json:"status"`
	AccountData   struct {
		Account           string `json:"account"`
		Balance           string `json:"balance"`
		Flags             int    `json:"flags"`
		LedgerEntryType   string `json:"ledger_entry_type"`
		OwnerCount        int    `json:"owner_count"`
		PreviousTxnID     string `json:"previous_txn_id"`
		PreviousTxnLgrSeq int    `json:"previous_txn_lgr_seq"`
		Sequence          int    `json:"sequence"`
		Index             string `json:"index"`
	} `json:"account_data"`
}

func HandleAccountInfo(params interface{}) (interface{}, error) {
	var request AccountInfoRequest
	if err := utils.ConvertParams(params, &request); err != nil {
		return nil, fmt.Errorf("invalid params for account_info: %v", err)
	}
	// Mock response
	response := AccountInfoResponse{
		Account:       request.Account,
		LedgerCurrent: 1234567,
		LedgerIndex:   1234567,
		LedgerHash:    "ABC123...",
		Status:        "success",
		AccountData: struct {
			Account           string `json:"account"`
			Balance           string `json:"balance"`
			Flags             int    `json:"flags"`
			LedgerEntryType   string `json:"ledger_entry_type"`
			OwnerCount        int    `json:"owner_count"`
			PreviousTxnID     string `json:"previous_txn_id"`
			PreviousTxnLgrSeq int    `json:"previous_txn_lgr_seq"`
			Sequence          int    `json:"sequence"`
			Index             string `json:"index"`
		}{
			Account:           request.Account,
			Balance:           "1000000000",
			Flags:             0,
			LedgerEntryType:   "AccountRoot",
			OwnerCount:        0,
			PreviousTxnID:     "0000000000000000000000000000000000000000000000000000000000000000",
			PreviousTxnLgrSeq: 0,
			Sequence:          1,
			Index:             "13F1A95D7AAB7108D5CE7EEAF504B2894B8C674E6D68499076441C4837282BF8",
		},
	}

	return response, nil
}
