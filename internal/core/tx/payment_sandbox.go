package tx

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// PaymentSandbox provides isolated, reversible state changes for payment processing.
// It wraps a LedgerView and tracks modifications without immediately applying them.
// Credits made during payment execution are tracked via DeferredCredits to prevent
// liquidity double-counting across different payment paths.
//
// Based on rippled's PaymentSandbox implementation.
type PaymentSandbox struct {
	// parent is the parent PaymentSandbox (nil for root sandbox)
	parent *PaymentSandbox

	// view is the underlying ledger view (used only for root sandbox)
	view LedgerView

	// modifications holds modified ledger entries (key -> new data)
	modifications map[[32]byte][]byte

	// preImages holds original ledger entries before modification (for metadata)
	preImages map[[32]byte][]byte

	// insertions holds newly created ledger entries
	insertions map[[32]byte][]byte

	// deletions holds deleted ledger entry keys
	deletions map[[32]byte]bool

	// tab holds deferred credits for balance adjustments
	tab *DeferredCredits

	// dropsDestroyed tracks XRP destroyed during this sandbox's operations
	dropsDestroyed XRPAmount.XRPAmount

	// txHash is the current transaction hash (for PreviousTxnID updates)
	txHash [32]byte

	// ledgerSeq is the current ledger sequence (for PreviousTxnLgrSeq updates)
	ledgerSeq uint32
}

// DeferredCredits tracks credits that shouldn't be spendable mid-transaction.
// This prevents consuming liquidity from one path from affecting other paths.
type DeferredCredits struct {
	// credits maps (lowAccount, highAccount, currency) -> adjustment
	credits map[deferredKey]*deferredValue

	// ownerCounts tracks maximum owner count seen for each account
	ownerCounts map[[20]byte]uint32
}

// deferredKey is the key for deferred credits lookup
// Accounts are stored in canonical order (low < high)
type deferredKey struct {
	low      [20]byte
	high     [20]byte
	currency string
}

// deferredValue holds the accumulated credits and original balance
type deferredValue struct {
	lowAcctCredits    IOUAmount
	highAcctCredits   IOUAmount
	lowAcctOrigBalance IOUAmount
}

// Adjustment represents the deferred credit adjustments for a balance lookup
type Adjustment struct {
	Debits      IOUAmount
	Credits     IOUAmount
	OrigBalance IOUAmount
}

// NewPaymentSandbox creates a new root PaymentSandbox wrapping the given LedgerView
func NewPaymentSandbox(view LedgerView) *PaymentSandbox {
	return &PaymentSandbox{
		parent:        nil,
		view:          view,
		modifications: make(map[[32]byte][]byte),
		preImages:     make(map[[32]byte][]byte),
		insertions:    make(map[[32]byte][]byte),
		deletions:     make(map[[32]byte]bool),
		tab:           newDeferredCredits(),
	}
}

// SetTransactionContext sets the current transaction hash and ledger sequence
// for PreviousTxnID/PreviousTxnLgrSeq updates when modifying ledger objects.
func (s *PaymentSandbox) SetTransactionContext(txHash [32]byte, ledgerSeq uint32) {
	s.txHash = txHash
	s.ledgerSeq = ledgerSeq
}

// GetTransactionContext returns the current transaction hash and ledger sequence
func (s *PaymentSandbox) GetTransactionContext() ([32]byte, uint32) {
	// Walk up the chain to find the root sandbox with the context
	if s.txHash != [32]byte{} || s.ledgerSeq != 0 {
		return s.txHash, s.ledgerSeq
	}
	if s.parent != nil {
		return s.parent.GetTransactionContext()
	}
	return s.txHash, s.ledgerSeq
}

// NewChildSandbox creates a child PaymentSandbox on top of a parent.
// Changes are pushed to the parent when Apply() is called.
func NewChildSandbox(parent *PaymentSandbox) *PaymentSandbox {
	txHash, ledgerSeq := parent.GetTransactionContext()
	return &PaymentSandbox{
		parent:        parent,
		txHash:        txHash,
		ledgerSeq:     ledgerSeq,
		view:          nil, // Use parent's view
		modifications: make(map[[32]byte][]byte),
		preImages:     make(map[[32]byte][]byte),
		insertions:    make(map[[32]byte][]byte),
		deletions:     make(map[[32]byte]bool),
		tab:           newDeferredCredits(),
	}
}

// newDeferredCredits creates a new DeferredCredits instance
func newDeferredCredits() *DeferredCredits {
	return &DeferredCredits{
		credits:     make(map[deferredKey]*deferredValue),
		ownerCounts: make(map[[20]byte]uint32),
	}
}

// makeKey creates a canonical key for deferred credits lookup.
// Accounts are ordered so that low < high.
func makeKey(a1, a2 [20]byte, currency string) deferredKey {
	if bytes.Compare(a1[:], a2[:]) < 0 {
		return deferredKey{low: a1, high: a2, currency: currency}
	}
	return deferredKey{low: a2, high: a1, currency: currency}
}

// Read reads a ledger entry from the sandbox or underlying view
func (s *PaymentSandbox) Read(k keylet.Keylet) ([]byte, error) {
	key := k.Key

	// Check deletions first
	if s.deletions[key] {
		return nil, nil
	}

	// Check local modifications
	if data, ok := s.modifications[key]; ok {
		return data, nil
	}

	// Check local insertions
	if data, ok := s.insertions[key]; ok {
		return data, nil
	}

	// Check parent sandbox
	if s.parent != nil {
		return s.parent.Read(k)
	}

	// Read from underlying view
	if s.view != nil {
		return s.view.Read(k)
	}

	return nil, nil
}

// Exists checks if a ledger entry exists in the sandbox or underlying view
func (s *PaymentSandbox) Exists(k keylet.Keylet) (bool, error) {
	key := k.Key

	// Check deletions first
	if s.deletions[key] {
		return false, nil
	}

	// Check local modifications or insertions
	if _, ok := s.modifications[key]; ok {
		return true, nil
	}
	if _, ok := s.insertions[key]; ok {
		return true, nil
	}

	// Check parent sandbox
	if s.parent != nil {
		return s.parent.Exists(k)
	}

	// Check underlying view
	if s.view != nil {
		return s.view.Exists(k)
	}

	return false, nil
}

// Insert adds a new ledger entry to the sandbox
func (s *PaymentSandbox) Insert(k keylet.Keylet, data []byte) error {
	key := k.Key
	// Remove from deletions if present
	delete(s.deletions, key)
	// Copy data to avoid external mutations
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	s.insertions[key] = dataCopy
	return nil
}

// Update modifies an existing ledger entry in the sandbox
func (s *PaymentSandbox) Update(k keylet.Keylet, data []byte) error {
	key := k.Key
	// Copy data to avoid external mutations
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// If it was an insertion, update the insertion
	if _, ok := s.insertions[key]; ok {
		s.insertions[key] = dataCopy
		return nil
	}

	// Capture pre-image if we haven't already
	if _, hasPreImage := s.preImages[key]; !hasPreImage {
		// Get the original value from parent chain or underlying view
		origData, err := s.readOriginal(k)
		if err == nil && origData != nil {
			preImageCopy := make([]byte, len(origData))
			copy(preImageCopy, origData)
			s.preImages[key] = preImageCopy
		}
	}

	s.modifications[key] = dataCopy
	return nil
}

// readOriginal reads the original value from parent chain or underlying view,
// bypassing any local modifications in this sandbox.
func (s *PaymentSandbox) readOriginal(k keylet.Keylet) ([]byte, error) {
	// If we have a parent, ask the parent for the original
	if s.parent != nil {
		return s.parent.Read(k)
	}
	// Otherwise read from the underlying view
	if s.view != nil {
		return s.view.Read(k)
	}
	return nil, nil
}

// Erase marks a ledger entry for deletion
func (s *PaymentSandbox) Erase(k keylet.Keylet) error {
	key := k.Key
	delete(s.modifications, key)
	delete(s.insertions, key)
	s.deletions[key] = true
	return nil
}

// AdjustDropsDestroyed records XRP that has been destroyed
func (s *PaymentSandbox) AdjustDropsDestroyed(drops XRPAmount.XRPAmount) {
	s.dropsDestroyed = s.dropsDestroyed.Add(drops)
}

// ForEach iterates over all state entries visible from this sandbox
func (s *PaymentSandbox) ForEach(fn func(key [32]byte, data []byte) bool) error {
	// Track visited keys to avoid duplicates
	visited := make(map[[32]byte]bool)

	// Visit local insertions
	for key, data := range s.insertions {
		if s.deletions[key] {
			continue
		}
		visited[key] = true
		if !fn(key, data) {
			return nil
		}
	}

	// Visit local modifications
	for key, data := range s.modifications {
		if s.deletions[key] || visited[key] {
			continue
		}
		visited[key] = true
		if !fn(key, data) {
			return nil
		}
	}

	// Visit parent or underlying view
	if s.parent != nil {
		return s.parent.ForEach(func(key [32]byte, data []byte) bool {
			if s.deletions[key] || visited[key] {
				return true // Skip but continue iteration
			}
			return fn(key, data)
		})
	}

	if s.view != nil {
		return s.view.ForEach(func(key [32]byte, data []byte) bool {
			if s.deletions[key] || visited[key] {
				return true // Skip but continue iteration
			}
			return fn(key, data)
		})
	}

	return nil
}

// Credit records a credit from sender to receiver.
// This is called when currency moves between accounts during payment execution.
func (s *PaymentSandbox) Credit(sender, receiver [20]byte, amount IOUAmount, preCreditSenderBalance IOUAmount) {
	s.tab.credit(sender, receiver, amount, preCreditSenderBalance)
}

// credit records a credit in the deferred credits table
func (dc *DeferredCredits) credit(sender, receiver [20]byte, amount IOUAmount, preCreditSenderBalance IOUAmount) {
	if sender == receiver {
		return // No self-credits
	}

	// Amount should be non-negative
	if amount.IsNegative() {
		return
	}

	key := makeKey(sender, receiver, amount.Currency)
	v, exists := dc.credits[key]

	if !exists {
		v = &deferredValue{
			lowAcctCredits:    NewIOUAmount("0", amount.Currency, amount.Issuer),
			highAcctCredits:   NewIOUAmount("0", amount.Currency, amount.Issuer),
			lowAcctOrigBalance: NewIOUAmount("0", amount.Currency, amount.Issuer),
		}

		if bytes.Compare(sender[:], receiver[:]) < 0 {
			// sender is low account
			v.highAcctCredits = amount
			v.lowAcctOrigBalance = preCreditSenderBalance
		} else {
			// sender is high account
			v.lowAcctCredits = amount
			v.lowAcctOrigBalance = preCreditSenderBalance.Negate()
		}

		dc.credits[key] = v
	} else {
		// Only record the balance the first time
		if bytes.Compare(sender[:], receiver[:]) < 0 {
			v.highAcctCredits = v.highAcctCredits.Add(amount)
		} else {
			v.lowAcctCredits = v.lowAcctCredits.Add(amount)
		}
	}
}

// adjustments returns the deferred credit adjustments for a balance lookup
func (dc *DeferredCredits) adjustments(main, other [20]byte, currency string) *Adjustment {
	key := makeKey(main, other, currency)
	v, exists := dc.credits[key]
	if !exists {
		return nil
	}

	if bytes.Compare(main[:], other[:]) < 0 {
		// main is low account
		return &Adjustment{
			Debits:      v.highAcctCredits,
			Credits:     v.lowAcctCredits,
			OrigBalance: v.lowAcctOrigBalance,
		}
	}

	// main is high account
	return &Adjustment{
		Debits:      v.lowAcctCredits,
		Credits:     v.highAcctCredits,
		OrigBalance: v.lowAcctOrigBalance.Negate(),
	}
}

// ownerCount records the owner count for an account
func (dc *DeferredCredits) ownerCount(id [20]byte, cur, next uint32) {
	v := cur
	if next > v {
		v = next
	}

	existing, exists := dc.ownerCounts[id]
	if !exists {
		dc.ownerCounts[id] = v
	} else if v > existing {
		dc.ownerCounts[id] = v
	}
}

// getOwnerCount returns the recorded owner count for an account
func (dc *DeferredCredits) getOwnerCount(id [20]byte) (uint32, bool) {
	v, ok := dc.ownerCounts[id]
	return v, ok
}

// apply merges deferred credits into another DeferredCredits
func (dc *DeferredCredits) apply(to *DeferredCredits) {
	for key, fromVal := range dc.credits {
		toVal, exists := to.credits[key]
		if !exists {
			// Copy the entire value
			to.credits[key] = &deferredValue{
				lowAcctCredits:    fromVal.lowAcctCredits,
				highAcctCredits:   fromVal.highAcctCredits,
				lowAcctOrigBalance: fromVal.lowAcctOrigBalance,
			}
		} else {
			// Accumulate credits, don't update origBalance
			toVal.lowAcctCredits = toVal.lowAcctCredits.Add(fromVal.lowAcctCredits)
			toVal.highAcctCredits = toVal.highAcctCredits.Add(fromVal.highAcctCredits)
		}
	}

	for id, fromCount := range dc.ownerCounts {
		toCount, exists := to.ownerCounts[id]
		if !exists {
			to.ownerCounts[id] = fromCount
		} else if fromCount > toCount {
			to.ownerCounts[id] = fromCount
		}
	}
}

// BalanceHook adjusts a reported balance by subtracting deferred credits.
// This prevents liquidity double-counting across payment paths.
//
// The algorithm walks the sandbox chain, accumulating debits and tracking
// the minimum balance seen. The returned balance is the minimum of:
// - The current amount
// - The last recorded original balance minus accumulated debits
// - The minimum balance seen in the chain
func (s *PaymentSandbox) BalanceHook(account, issuer [20]byte, amount IOUAmount) IOUAmount {
	currency := amount.Currency

	// Initialize with zeroed amount for delta tracking
	delta := NewIOUAmount("0", currency, amount.Issuer)
	lastBal := amount
	minBal := amount

	// Walk up the sandbox chain
	for curSB := s; curSB != nil; curSB = curSB.parent {
		if adj := curSB.tab.adjustments(account, issuer, currency); adj != nil {
			delta = delta.Add(adj.Debits)
			lastBal = adj.OrigBalance
			if lastBal.Compare(minBal) < 0 {
				minBal = lastBal
			}
		}
	}

	// Compute adjusted amount: min(amount, lastBal - delta, minBal)
	adjustedFromOrig := lastBal.Sub(delta)

	result := amount
	if adjustedFromOrig.Compare(result) < 0 {
		result = adjustedFromOrig
	}
	if minBal.Compare(result) < 0 {
		result = minBal
	}

	// Preserve the issuer
	result.Issuer = amount.Issuer

	// A negative XRP balance is not an error - just return zero
	if issuer == [20]byte{} && result.IsNegative() {
		return NewIOUAmount("0", currency, amount.Issuer)
	}

	return result
}

// OwnerCountHook returns the maximum owner count seen for an account
// during payment processing.
func (s *PaymentSandbox) OwnerCountHook(account [20]byte, count uint32) uint32 {
	result := count
	for curSB := s; curSB != nil; curSB = curSB.parent {
		if adj, ok := curSB.tab.getOwnerCount(account); ok {
			if adj > result {
				result = adj
			}
		}
	}
	return result
}

// AdjustOwnerCount records an owner count change for an account
func (s *PaymentSandbox) AdjustOwnerCount(account [20]byte, cur, next uint32) {
	s.tab.ownerCount(account, cur, next)
}

// Apply merges this sandbox's changes into a parent PaymentSandbox
func (s *PaymentSandbox) Apply(to *PaymentSandbox) {
	if s.parent != to {
		panic("PaymentSandbox.Apply: parent mismatch")
	}

	// Apply ledger item changes
	for key := range s.deletions {
		to.deletions[key] = true
		delete(to.modifications, key)
		delete(to.insertions, key)
	}

	for key, data := range s.insertions {
		if to.deletions[key] {
			delete(to.deletions, key)
		}
		// If parent has this as a modification, keep it as modification
		if _, ok := to.modifications[key]; ok {
			to.modifications[key] = data
		} else {
			to.insertions[key] = data
		}
	}

	for key, data := range s.modifications {
		if to.deletions[key] {
			continue
		}
		to.modifications[key] = data
	}

	// Propagate preImages (only if parent doesn't already have one)
	for key, data := range s.preImages {
		if _, hasPreImage := to.preImages[key]; !hasPreImage {
			preImageCopy := make([]byte, len(data))
			copy(preImageCopy, data)
			to.preImages[key] = preImageCopy
		}
	}

	// Apply deferred credits
	s.tab.apply(to.tab)

	// Apply drops destroyed
	to.dropsDestroyed = to.dropsDestroyed.Add(s.dropsDestroyed)
}

// ApplyToView merges this sandbox's changes into an underlying LedgerView.
// This should only be called on a root sandbox (parent == nil).
func (s *PaymentSandbox) ApplyToView(view LedgerView) error {
	if s.parent != nil {
		panic("PaymentSandbox.ApplyToView: not a root sandbox")
	}

	// Apply deletions
	for key := range s.deletions {
		if err := view.Erase(keylet.Keylet{Key: key}); err != nil {
			return err
		}
	}

	// Apply insertions
	for key, data := range s.insertions {
		if err := view.Insert(keylet.Keylet{Key: key}, data); err != nil {
			return err
		}
	}

	// Apply modifications
	for key, data := range s.modifications {
		if err := view.Update(keylet.Keylet{Key: key}, data); err != nil {
			return err
		}
	}

	// Apply drops destroyed
	if s.dropsDestroyed.Drops() != 0 {
		view.AdjustDropsDestroyed(s.dropsDestroyed)
	}

	return nil
}

// Reset clears all changes from this sandbox
func (s *PaymentSandbox) Reset() {
	s.modifications = make(map[[32]byte][]byte)
	s.preImages = make(map[[32]byte][]byte)
	s.insertions = make(map[[32]byte][]byte)
	s.deletions = make(map[[32]byte]bool)
	s.tab = newDeferredCredits()
	s.dropsDestroyed = XRPAmount.XRPAmount(0)
}

// GetView returns the underlying LedgerView for this sandbox chain
func (s *PaymentSandbox) GetView() LedgerView {
	if s.view != nil {
		return s.view
	}
	if s.parent != nil {
		return s.parent.GetView()
	}
	return nil
}

// IOUAmountFromBigFloat creates an IOUAmount from a big.Float value
func IOUAmountFromBigFloat(value *big.Float, currency, issuer string) IOUAmount {
	return IOUAmount{
		Value:    value,
		Currency: currency,
		Issuer:   issuer,
	}
}

// ZeroIOUAmount creates a zero IOUAmount with the given currency and issuer
func ZeroIOUAmount(currency, issuer string) IOUAmount {
	return NewIOUAmount("0", currency, issuer)
}

// GenerateAffectedNodes creates AffectedNode entries for all sandbox changes.
// This converts the binary pre-images and modifications into metadata format.
func (s *PaymentSandbox) GenerateAffectedNodes() []AffectedNode {
	nodes := make([]AffectedNode, 0)

	// Process modifications
	for key, postData := range s.modifications {
		preData := s.preImages[key]
		if preData == nil {
			// Try to get pre-image from parent chain or underlying view
			preData, _ = s.readOriginal(keylet.Keylet{Key: key})
		}

		if preData == nil {
			// Can't generate proper metadata without pre-image
			continue
		}

		node := buildModifiedNodeFromBinary(key, preData, postData)
		if node != nil {
			nodes = append(nodes, *node)
		}
	}

	// Process insertions
	for key, data := range s.insertions {
		node := buildCreatedNodeFromBinary(key, data)
		if node != nil {
			nodes = append(nodes, *node)
		}
	}

	// Process deletions
	for key := range s.deletions {
		// Get the data before deletion
		preData := s.preImages[key]
		if preData == nil {
			preData, _ = s.readOriginal(keylet.Keylet{Key: key})
		}
		if preData != nil {
			node := buildDeletedNodeFromBinary(key, preData)
			if node != nil {
				nodes = append(nodes, *node)
			}
		}
	}

	return nodes
}

// buildModifiedNodeFromBinary creates an AffectedNode for a modified entry
func buildModifiedNodeFromBinary(key [32]byte, original, current []byte) *AffectedNode {
	entryType := getLedgerEntryType(current)

	node := &AffectedNode{
		NodeType:        "ModifiedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		FinalFields:     make(map[string]any),
		PreviousFields:  make(map[string]any),
	}

	// Extract fields from original and current
	origFields, err := extractLedgerFields(original, entryType)
	if err != nil {
		return nil
	}

	currFields, err := extractLedgerFields(current, entryType)
	if err != nil {
		return nil
	}

	// Extract PreviousTxnID and PreviousTxnLgrSeq from original
	if prevTxnID, ok := origFields["PreviousTxnID"]; ok {
		node.PreviousTxnID = fmt.Sprintf("%v", prevTxnID)
	}
	if prevTxnLgrSeq, ok := origFields["PreviousTxnLgrSeq"]; ok {
		if seq, ok := prevTxnLgrSeq.(uint32); ok {
			node.PreviousTxnLgrSeq = seq
		}
	}

	// PreviousFields: fields that changed
	for name, origValue := range origFields {
		if shouldIncludeInPreviousFields(name) {
			if currValue, exists := currFields[name]; exists {
				if !fieldsEqual(origValue, currValue) {
					node.PreviousFields[name] = origValue
				}
			} else {
				// Field was removed
				node.PreviousFields[name] = origValue
			}
		}
	}

	// FinalFields: fields with sMD_Always | sMD_ChangeNew
	for name, currValue := range currFields {
		if shouldIncludeInFinalFields(name) {
			node.FinalFields[name] = currValue
		}
	}

	// Clean up empty maps
	if len(node.FinalFields) == 0 {
		node.FinalFields = nil
	}
	if len(node.PreviousFields) == 0 {
		node.PreviousFields = nil
	}

	return node
}

// buildCreatedNodeFromBinary creates an AffectedNode for a created entry
func buildCreatedNodeFromBinary(key [32]byte, data []byte) *AffectedNode {
	entryType := getLedgerEntryType(data)

	node := &AffectedNode{
		NodeType:        "CreatedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		NewFields:       make(map[string]any),
	}

	fields, err := extractLedgerFields(data, entryType)
	if err != nil {
		return nil
	}

	for name, value := range fields {
		if shouldIncludeInCreate(name) && !isDefaultFieldValue(value) {
			node.NewFields[name] = value
		}
	}

	return node
}

// buildDeletedNodeFromBinary creates an AffectedNode for a deleted entry
func buildDeletedNodeFromBinary(key [32]byte, data []byte) *AffectedNode {
	entryType := getLedgerEntryType(data)

	node := &AffectedNode{
		NodeType:        "DeletedNode",
		LedgerEntryType: entryType,
		LedgerIndex:     strings.ToUpper(hex.EncodeToString(key[:])),
		FinalFields:     make(map[string]any),
	}

	fields, err := extractLedgerFields(data, entryType)
	if err != nil {
		return nil
	}

	for name, value := range fields {
		if shouldIncludeInDeleteFinal(name) {
			node.FinalFields[name] = value
		}
	}

	return node
}

// isDefaultFieldValue checks if a value is a default value that should be omitted
func isDefaultFieldValue(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case uint32:
		return v == 0
	case uint64:
		return v == 0
	case int64:
		return v == 0
	case string:
		return v == "" || v == "0" || v == "0000000000000000"
	case map[string]any:
		// For amounts, check if value is "0"
		if val, ok := v["value"]; ok {
			if strVal, ok := val.(string); ok {
				return strVal == "0"
			}
		}
		return false
	default:
		return false
	}
}
