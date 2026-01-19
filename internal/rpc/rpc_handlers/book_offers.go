package rpc_handlers

import (
	"encoding/json"

	"github.com/LeJamon/goXRPLd/internal/rpc/rpc_types"
)

// BookOffersMethod handles the book_offers RPC method
type BookOffersMethod struct{}

func (m *BookOffersMethod) Handle(ctx *rpc_types.RpcContext, params json.RawMessage) (interface{}, *rpc_types.RpcError) {
	var request struct {
		TakerGets json.RawMessage `json:"taker_gets"`
		TakerPays json.RawMessage `json:"taker_pays"`
		Taker     string          `json:"taker,omitempty"`
		rpc_types.LedgerSpecifier
		rpc_types.PaginationParams
	}

	if params != nil {
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, rpc_types.RpcErrorInvalidParams("Invalid parameters: " + err.Error())
		}
	}

	if len(request.TakerGets) == 0 || len(request.TakerPays) == 0 {
		return nil, rpc_types.RpcErrorInvalidParams("Both taker_gets and taker_pays are required")
	}

	// Check if ledger service is available
	if rpc_types.Services == nil || rpc_types.Services.Ledger == nil {
		return nil, rpc_types.RpcErrorInternal("Ledger service not available")
	}

	// Parse taker_gets amount
	takerGets, err := ParseAmountFromJSON(request.TakerGets)
	if err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid taker_gets: " + err.Error())
	}

	// Parse taker_pays amount
	takerPays, err := ParseAmountFromJSON(request.TakerPays)
	if err != nil {
		return nil, rpc_types.RpcErrorInvalidParams("Invalid taker_pays: " + err.Error())
	}

	// Determine ledger index to use
	ledgerIndex := "current"
	if request.LedgerIndex != "" {
		ledgerIndex = request.LedgerIndex.String()
	}

	// Get book offers from the ledger service
	result, err := rpc_types.Services.Ledger.GetBookOffers(takerGets, takerPays, ledgerIndex, request.Limit)
	if err != nil {
		return nil, rpc_types.RpcErrorInternal("Failed to get book offers: " + err.Error())
	}

	// Build response
	response := map[string]interface{}{
		"ledger_hash":  FormatLedgerHash(result.LedgerHash),
		"ledger_index": result.LedgerIndex,
		"offers":       result.Offers,
		"validated":    result.Validated,
	}

	return response, nil
}

// ParseAmountFromJSON parses an amount from JSON (either XRP string or IOU object)
func ParseAmountFromJSON(data json.RawMessage) (rpc_types.Amount, error) {
	// Try parsing as string first (XRP amount)
	var xrpAmount string
	if err := json.Unmarshal(data, &xrpAmount); err == nil {
		return rpc_types.Amount{Value: xrpAmount}, nil
	}

	// Try parsing as IOU object
	var iouAmount struct {
		Currency string `json:"currency"`
		Issuer   string `json:"issuer"`
		Value    string `json:"value,omitempty"`
	}
	if err := json.Unmarshal(data, &iouAmount); err != nil {
		return rpc_types.Amount{}, err
	}

	return rpc_types.Amount{
		Currency: iouAmount.Currency,
		Issuer:   iouAmount.Issuer,
		Value:    iouAmount.Value,
	}, nil
}

func (m *BookOffersMethod) RequiredRole() rpc_types.Role {
	return rpc_types.RoleGuest
}

func (m *BookOffersMethod) SupportedApiVersions() []int {
	return []int{rpc_types.ApiVersion1, rpc_types.ApiVersion2, rpc_types.ApiVersion3}
}
