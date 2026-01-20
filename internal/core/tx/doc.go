// Package tx provides the XRPL transaction processing engine.
//
// This package implements the complete transaction lifecycle for the XRP Ledger,
// including parsing, validation, signature verification, and application to the
// ledger state.
//
// # Architecture
//
// The package is organized into the following components:
//
// ## Core Types (transaction.go, types.go, result.go)
//   - Transaction interface and common types (Amount, Memo, Signer, Common, BaseTx)
//   - Transaction type constants (Type enum)
//   - Result codes matching rippled's tes/tec/tef/tel/tem/ter system
//
// ## Engine (engine*.go)
//   - engine.go: Main Engine struct and Apply method
//   - engine_config.go: EngineConfig and validation constants
//   - engine_preflight.go: Syntax validation and signature verification
//   - engine_preclaim.go: Ledger state validation
//   - engine_apply.go: Transaction dispatch to type-specific handlers
//
// ## Registry (registry.go)
//   - Map-based transaction factory for creating types from codes
//   - Supports registration of custom transaction types
//
// ## Parsing (parse.go)
//   - JSON and binary transaction parsing
//   - Type-safe deserialization
//
// ## Signing (signature.go, signer.go, serialize.go)
//   - Single and multi-signature verification
//   - Transaction serialization for signing
//   - Signer list quorum validation
//
// ## Transaction Types (tx_*.go)
//   - tx_payment.go: Payment transactions
//   - tx_account.go: AccountSet, AccountDelete, SetRegularKey, SignerListSet
//   - tx_trust.go: TrustSet
//   - tx_offer.go: OfferCreate, OfferCancel
//   - tx_escrow.go: EscrowCreate, EscrowFinish, EscrowCancel
//   - tx_check.go: CheckCreate, CheckCash, CheckCancel
//   - tx_paychan.go: PaymentChannel operations
//   - tx_nftoken.go: NFToken operations
//   - tx_amm.go: AMM operations
//   - tx_xchain.go: Cross-chain bridge operations
//   - tx_did.go: DID operations
//   - tx_oracle.go: Oracle operations
//   - tx_mptoken.go: MPToken operations
//   - tx_credential.go: Credential operations
//   - tx_domain.go: PermissionedDomain operations
//   - tx_vault.go: Vault operations
//   - tx_misc.go: TicketCreate, Clawback, DelegateSet, Batch, LedgerStateFix
//
// ## Apply Functions (apply_*.go)
//   - Type-specific transaction application logic
//   - Each file corresponds to a transaction type group
//   - Large apply functions are split into focused sub-files:
//     - apply_payment.go, apply_payment_xrp.go, apply_payment_iou.go
//     - apply_nftoken.go, apply_nftoken_mint.go, apply_nftoken_offer.go
//     - apply_amm.go, apply_amm_create.go, apply_amm_deposit.go, etc.
//
// ## Ledger State (state_*.go)
//   - state_account.go: AccountRoot ledger entry
//   - state_trustline.go: RippleState (trust line) ledger entry
//   - state_directory.go: Directory entry management
//   - state_misc.go: Other ledger entry types
//
// # Transaction Processing Pipeline
//
// Transactions are processed through the following pipeline:
//
//  1. Parsing: JSON/binary → typed Transaction object
//  2. Preflight: Syntax validation, signature verification
//  3. Preclaim: Ledger state validation (account exists, sequence valid)
//  4. Fee calculation: Base fee * (1 + numSigners) for multi-sig
//  5. Apply: Type-specific logic via registered handlers
//  6. Metadata: Record all affected ledger entries
//
// # Example Usage
//
//	// Create engine with ledger view
//	engine := tx.NewEngine(view, tx.EngineConfig{
//	    BaseFee:          10,
//	    ReserveBase:      10000000,
//	    ReserveIncrement: 2000000,
//	    LedgerSequence:   12345,
//	})
//
//	// Parse and apply transaction
//	txn, err := tx.ParseJSON(jsonBytes)
//	if err != nil {
//	    return err
//	}
//
//	result := engine.Apply(txn)
//	if !result.Result.IsSuccess() {
//	    return fmt.Errorf("transaction failed: %s", result.Message)
//	}
package tx
