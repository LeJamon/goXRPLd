package amm

import "github.com/LeJamon/goXRPLd/internal/core/tx"

// AMMData holds the internal AMM ledger entry data.
type AMMData struct {
	Account    [20]byte
	Asset      [20]byte
	Asset2     [20]byte
	TradingFee uint16
	// LPTokenBalance is the total LP tokens outstanding for this AMM
	LPTokenBalance tx.Amount
	// AssetBalance tracks the balance of Asset (XRP or IOU)
	// In production, IOU balances are read from trust lines, but we track here for simplicity
	AssetBalance tx.Amount
	// Asset2Balance tracks the balance of Asset2 (XRP or IOU)
	Asset2Balance tx.Amount
	VoteSlots     []VoteSlotData
	AuctionSlot   *AuctionSlotData
}

// VoteSlotData holds a single vote slot entry.
type VoteSlotData struct {
	Account    [20]byte
	TradingFee uint16
	VoteWeight uint32
}

// AuctionSlotData holds the auction slot state.
type AuctionSlotData struct {
	Account      [20]byte
	Expiration   uint32
	Price        tx.Amount
	AuthAccounts [][20]byte
}

// AuthAccount is an authorized account for AMM slot trading
type AuthAccount struct {
	AuthAccount AuthAccountData `json:"AuthAccount"`
}

// AuthAccountData contains the account address
type AuthAccountData struct {
	Account string `json:"Account"`
}
