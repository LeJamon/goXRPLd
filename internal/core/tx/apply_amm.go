package tx

import (
	"encoding/hex"
	"fmt"
	"strconv"

	addresscodec "github.com/LeJamon/goXRPLd/internal/codec/address-codec"
	binarycodec "github.com/LeJamon/goXRPLd/internal/codec/binary-codec"
	crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// AMM data structures

// AMMData represents an AMM ledger entry
type AMMData struct {
	Account        [20]byte // AMM account
	Asset          [20]byte // First asset currency (20 bytes)
	Asset2         [20]byte // Second asset currency (20 bytes)
	TradingFee     uint16
	LPTokenBalance uint64
	VoteSlots      []VoteSlotData
	AuctionSlot    *AuctionSlotData
}

// VoteSlotData represents a voting slot in an AMM
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData represents the auction slot in an AMM
type AuctionSlotData struct {
	Account      [20]byte
	Expiration   uint32
	Price        uint64
	AuthAccounts [][20]byte
}

// computeAMMAccountID derives the AMM account from the asset pair
func computeAMMAccountID(asset1, asset2 Asset) [20]byte {
	// In rippled, this is computed by hashing the asset pair
	// Simplified implementation: hash the currency codes
	var result [20]byte
	data := []byte(asset1.Currency + asset1.Issuer + asset2.Currency + asset2.Issuer)
	hash := crypto.Sha512Half(data)
	copy(result[:], hash[:20])
	return result
}

// applyAMMCreate applies an AMMCreate transaction
func (e *Engine) applyAMMCreate(tx *AMMCreate, account *AccountRoot, metadata *Metadata) Result {
	accountID, _ := decodeAccountID(tx.Account)

	// Parse amounts
	amount1, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
	if err != nil {
		amount1 = 0
	}
	amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
	if err != nil {
		amount2 = 0
	}

	// For XRP amounts, check balance
	if tx.Amount.Currency == "" && amount1 > 0 {
		if account.Balance < amount1 {
			return TecUNFUNDED
		}
		account.Balance -= amount1
	}
	if tx.Amount2.Currency == "" && amount2 > 0 {
		if account.Balance < amount2 {
			return TecUNFUNDED
		}
		account.Balance -= amount2
	}

	// Compute AMM account ID
	asset1 := Asset{Currency: tx.Amount.Currency, Issuer: tx.Amount.Issuer}
	asset2 := Asset{Currency: tx.Amount2.Currency, Issuer: tx.Amount2.Issuer}
	ammAccountID := computeAMMAccountID(asset1, asset2)

	// Check if AMM already exists
	ammKey := keylet.Account(ammAccountID)
	exists, _ := e.view.Exists(ammKey)
	if exists {
		return TecDUPLICATE
	}

	// Create the AMM account
	ammAccount := &AccountRoot{
		Balance:  0,
		Sequence: 0,
		Flags:    0,
	}
	ammAccount.Account, _ = encodeAccountID(ammAccountID)

	ammAccountData, err := serializeAccountRoot(ammAccount)
	if err != nil {
		return TefINTERNAL
	}

	if err := e.view.Insert(ammKey, ammAccountData); err != nil {
		return TefINTERNAL
	}

	// Create the AMM entry (simplified)
	ammData := &AMMData{
		Account:        ammAccountID,
		TradingFee:     tx.TradingFee,
		LPTokenBalance: amount1 + amount2, // Simplified LP calculation
	}

	ammEntryData, err := serializeAMM(ammData, accountID)
	if err != nil {
		return TefINTERNAL
	}

	// Use AMM keylet (simplified - using account keylet)
	if err := e.view.Insert(ammKey, ammEntryData); err != nil {
		return TefINTERNAL
	}

	// Increase owner count (for AMM and LP tokens)
	account.OwnerCount++

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
		NewFields: map[string]any{
			"Account":    ammAccount.Account,
			"TradingFee": tx.TradingFee,
		},
	})

	return TesSUCCESS
}

// applyAMMDeposit applies an AMMDeposit transaction
func (e *Engine) applyAMMDeposit(tx *AMMDeposit, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Parse deposit amounts
	if tx.Amount != nil {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil && tx.Amount.Currency == "" {
			if account.Balance < amount {
				return TecUNFUNDED
			}
			account.Balance -= amount
		}
	}
	if tx.Amount2 != nil {
		amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
		if err == nil && tx.Amount2.Currency == "" {
			if account.Balance < amount2 {
				return TecUNFUNDED
			}
			account.Balance -= amount2
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMWithdraw applies an AMMWithdraw transaction
func (e *Engine) applyAMMWithdraw(tx *AMMWithdraw, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Process withdrawal - simplified, just update metadata
	if tx.Amount != nil {
		amount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil && tx.Amount.Currency == "" {
			account.Balance += amount
		}
	}
	if tx.Amount2 != nil {
		amount2, err := strconv.ParseUint(tx.Amount2.Value, 10, 64)
		if err == nil && tx.Amount2.Currency == "" {
			account.Balance += amount2
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMVote applies an AMMVote transaction
func (e *Engine) applyAMMVote(tx *AMMVote, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Record vote - simplified, in full implementation would update vote slots
	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
		FinalFields: map[string]any{
			"TradingFee": tx.TradingFee,
		},
	})

	return TesSUCCESS
}

// applyAMMBid applies an AMMBid transaction
func (e *Engine) applyAMMBid(tx *AMMBid, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Process bid - simplified
	if tx.BidMin != nil {
		bidAmount, err := strconv.ParseUint(tx.BidMin.Value, 10, 64)
		if err == nil && tx.BidMin.Currency == "" {
			if account.Balance < bidAmount {
				return TecUNFUNDED
			}
			account.Balance -= bidAmount
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMDelete applies an AMMDelete transaction
func (e *Engine) applyAMMDelete(tx *AMMDelete, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Delete the AMM (only works if empty)
	if err := e.view.Erase(ammKey); err != nil {
		return TefINTERNAL
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// applyAMMClawback applies an AMMClawback transaction
func (e *Engine) applyAMMClawback(tx *AMMClawback, account *AccountRoot, metadata *Metadata) Result {
	// Find the AMM
	ammAccountID := computeAMMAccountID(tx.Asset, tx.Asset2)
	ammKey := keylet.Account(ammAccountID)

	exists, _ := e.view.Exists(ammKey)
	if !exists {
		return TecNO_ENTRY
	}

	// Find the holder
	holderID, err := decodeAccountID(tx.Holder)
	if err != nil {
		return TemINVALID
	}

	holderKey := keylet.Account(holderID)
	exists, _ = e.view.Exists(holderKey)
	if !exists {
		return TecNO_TARGET
	}

	// Clawback LP tokens - simplified
	if tx.Amount != nil {
		clawbackAmount, err := strconv.ParseUint(tx.Amount.Value, 10, 64)
		if err == nil {
			// Transfer clawed back value to issuer
			account.Balance += clawbackAmount
		}
	}

	metadata.AffectedNodes = append(metadata.AffectedNodes, AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: "AMM",
		LedgerIndex:     hex.EncodeToString(ammKey.Key[:]),
	})

	return TesSUCCESS
}

// serializeAMM serializes an AMM ledger entry
func serializeAMM(amm *AMMData, ownerID [20]byte) ([]byte, error) {
	accountAddress, err := addresscodec.EncodeAccountIDToClassicAddress(amm.Account[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM account address: %w", err)
	}

	jsonObj := map[string]any{
		"LedgerEntryType": "AMM",
		"Account":         accountAddress,
		"TradingFee":      amm.TradingFee,
		"OwnerNode":       "0",
		"Flags":           uint32(0),
	}

	hexStr, err := binarycodec.Encode(jsonObj)
	if err != nil {
		return nil, fmt.Errorf("failed to encode AMM: %w", err)
	}

	return hex.DecodeString(hexStr)
}
