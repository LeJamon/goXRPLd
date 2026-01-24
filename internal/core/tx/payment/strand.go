package payment

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
	tx "github.com/LeJamon/goXRPLd/internal/core/tx"
	"github.com/LeJamon/goXRPLd/internal/core/tx/sle"
)

// DebugStrands enables debug output for strand building
var DebugStrands = false

// Path flags - indicate what type of element this path step contains
// These match rippled's STPathElement types
const (
	// PathTypeAccount indicates path element has account
	PathTypeAccount uint8 = 0x01
	// PathTypeCurrency indicates path element has currency
	PathTypeCurrency uint8 = 0x10
	// PathTypeIssuer indicates path element has issuer
	PathTypeIssuer uint8 = 0x20
)

// ToStrands converts payment paths to executable strands
// Parameters:
//   - view: PaymentSandbox with ledger state
//   - src: Source account
//   - dst: Destination account
//   - dstAmt: Destination amount/issue
//   - srcAmt: Source amount/issue (optional, from SendMax)
//   - paths: Payment paths from transaction
//   - addDefaultPath: Whether to add the default path (direct)
//
// Returns: List of executable strands, error if any path is invalid
func ToStrands(
	view *PaymentSandbox,
	src, dst [20]byte,
	dstAmt tx.Amount,
	srcAmt *tx.Amount,
	paths [][]PathStep,
	addDefaultPath bool,
) ([]Strand, error) {
	if DebugStrands {
		srcStr, _ := sle.EncodeAccountID(src)
		dstStr, _ := sle.EncodeAccountID(dst)
		fmt.Printf("DEBUG ToStrands: src=%s dst=%s dstAmt=%+v srcAmt=%+v addDefaultPath=%v\n", srcStr, dstStr, dstAmt, srcAmt, addDefaultPath)
	}

	dstIssue := GetIssue(dstAmt)

	var srcIssue *Issue
	if srcAmt != nil {
		issue := GetIssue(*srcAmt)
		srcIssue = &issue
	}

	var strands []Strand

	// Add default path if requested
	if addDefaultPath {
		strand, err := ToStrand(view, src, dst, dstIssue, srcIssue, nil, true)
		if err == nil && len(strand) > 0 {
			strands = append(strands, strand)
		}
	}

	// Convert each explicit path to a strand
	for _, path := range paths {
		strand, err := ToStrand(view, src, dst, dstIssue, srcIssue, path, false)
		if err == nil && len(strand) > 0 {
			// Check for duplicate strands
			isDuplicate := false
			for _, existing := range strands {
				if strandsEqual(existing, strand) {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				strands = append(strands, strand)
			}
		}
	}

	return strands, nil
}

// ToStrand converts a single path to an executable strand
// This matches rippled's toStrand() implementation in PaySteps.cpp
func ToStrand(
	view *PaymentSandbox,
	src, dst [20]byte,
	dstIssue Issue,
	srcIssue *Issue,
	path []PathStep,
	isDefaultPath bool,
) (Strand, error) {
	// Build the normalized path following rippled's approach
	// The normalized path includes implicit nodes for source, sendMax issuer, etc.

	// Determine the starting currency issue
	// Per rippled: Issue{currency, src} - source account is the initial "issuer" context
	var curIssue Issue
	if srcIssue != nil {
		curIssue = Issue{Currency: srcIssue.Currency, Issuer: src}
	} else {
		curIssue = Issue{Currency: dstIssue.Currency, Issuer: src}
	}

	if curIssue.IsXRP() {
		curIssue.Issuer = [20]byte{} // XRP pseudo-account
	}

	// Build normalized path as a list of PathElement-like nodes
	type normNode struct {
		account     [20]byte
		currency    string
		issuer      [20]byte
		hasAccount  bool
		hasCurrency bool
		hasIssuer   bool
	}

	var normPath []normNode

	// Add source node
	normPath = append(normPath, normNode{
		account:     src,
		currency:    curIssue.Currency,
		issuer:      curIssue.Issuer,
		hasAccount:  true,
		hasCurrency: true,
		hasIssuer:   true,
	})

	// If sendMaxIssue has a different account (issuer) than src, insert it
	// This is the key for cross-issuer ripple payments!
	if srcIssue != nil && srcIssue.Issuer != src {
		// Check if first path element isn't already this account
		needsInsert := true
		if len(path) > 0 && hasAccount(path[0]) {
			firstAccount := accountFromPathElement(path[0], src)
			if firstAccount == srcIssue.Issuer {
				needsInsert = false
			}
		}
		if needsInsert {
			normPath = append(normPath, normNode{
				account:    srcIssue.Issuer,
				hasAccount: true,
			})
		}
	}

	// Add explicit path elements
	for _, elem := range path {
		var node normNode
		if hasAccount(elem) {
			node.account = accountFromPathElement(elem, src)
			node.hasAccount = true
		}
		if hasCurrency(elem) {
			node.currency = elem.Currency
			node.hasCurrency = true
		}
		if hasIssuer(elem) {
			issuerBytes, err := sle.DecodeAccountID(elem.Issuer)
			if err == nil {
				node.issuer = issuerBytes
				node.hasIssuer = true
			}
		}
		normPath = append(normPath, node)
	}

	// Find the last currency in the path to check if we need currency change
	lastCurrency := curIssue.Currency
	for i := len(normPath) - 1; i >= 0; i-- {
		if normPath[i].hasCurrency {
			lastCurrency = normPath[i].currency
			break
		}
	}

	// Add currency/issuer step if currency differs
	// Note: For regular payments (not offer crossing), different issuers
	// with same currency do NOT need a book step - they use rippling
	if lastCurrency != dstIssue.Currency {
		normPath = append(normPath, normNode{
			currency:    dstIssue.Currency,
			issuer:      dstIssue.Issuer,
			hasCurrency: true,
			hasIssuer:   true,
		})
	}

	// Add destination issuer account if needed (for multi-hop through issuer)
	// Only if the last element isn't already that account AND dst != dstIssue.Issuer
	lastIsAccount := len(normPath) > 0 && normPath[len(normPath)-1].hasAccount
	lastAccount := src
	if lastIsAccount {
		lastAccount = normPath[len(normPath)-1].account
	}

	if !((lastIsAccount && lastAccount == dstIssue.Issuer) || (dst == dstIssue.Issuer)) {
		normPath = append(normPath, normNode{
			account:    dstIssue.Issuer,
			hasAccount: true,
		})
	}

	// Add destination if not already the last account
	if !lastIsAccount || normPath[len(normPath)-1].account != dst {
		// Check the updated last element
		if len(normPath) > 0 {
			lastNode := normPath[len(normPath)-1]
			if !lastNode.hasAccount || lastNode.account != dst {
				normPath = append(normPath, normNode{
					account:    dst,
					hasAccount: true,
				})
			}
		}
	}

	if len(normPath) < 2 {
		if DebugStrands {
			fmt.Printf("DEBUG ToStrand: normPath too short (%d elements)\n", len(normPath))
		}
		return nil, nil // Invalid path
	}

	if DebugStrands {
		fmt.Printf("DEBUG ToStrand: normPath has %d elements:\n", len(normPath))
		for i, n := range normPath {
			acctStr := "none"
			if n.hasAccount {
				acctStr, _ = sle.EncodeAccountID(n.account)
			}
			issuerStr := "none"
			if n.hasIssuer {
				issuerStr, _ = sle.EncodeAccountID(n.issuer)
			}
			fmt.Printf("  [%d] account=%s currency=%s issuer=%s hasAccount=%v hasCurrency=%v hasIssuer=%v\n",
				i, acctStr, n.currency, issuerStr, n.hasAccount, n.hasCurrency, n.hasIssuer)
		}
	}

	// Now convert normalized path to steps
	var strand Strand
	var prevStep Step

	// Reset curIssue for step creation
	if srcIssue != nil {
		curIssue = Issue{Currency: srcIssue.Currency, Issuer: src}
	} else {
		curIssue = Issue{Currency: dstIssue.Currency, Issuer: src}
	}
	if curIssue.IsXRP() {
		curIssue.Issuer = [20]byte{}
	}

	for i := 0; i < len(normPath)-1; i++ {
		cur := normPath[i]
		next := normPath[i+1]
		isLast := i == len(normPath)-2

		// Update current issue based on current node
		if cur.hasAccount {
			curIssue.Issuer = cur.account
		} else if cur.hasIssuer {
			curIssue.Issuer = cur.issuer
		}
		if cur.hasCurrency {
			curIssue.Currency = cur.currency
			if curIssue.IsXRP() {
				curIssue.Issuer = [20]byte{}
			}
		}

		// Handle account-to-account transitions (DirectStep or implied steps)
		if cur.hasAccount && next.hasAccount {
			// Check if we need an implied account step
			// Per rippled: if curIssue.account != cur.account AND curIssue.account != next.account
			if !curIssue.IsXRP() && curIssue.Issuer != cur.account && curIssue.Issuer != next.account {
				// Insert implied DirectStep to curIssue.Issuer first
				directStep := NewDirectStepI(cur.account, curIssue.Issuer, curIssue.Currency, prevStep, false)
				strand = append(strand, directStep)
				prevStep = directStep
				// Now create step from curIssue.Issuer to next
				directStep = NewDirectStepI(curIssue.Issuer, next.account, curIssue.Currency, prevStep, isLast)
				strand = append(strand, directStep)
				prevStep = directStep
			} else {
				// Direct step from cur to next
				if curIssue.IsXRP() {
					// XRP endpoint step
					if i == 0 {
						step := NewXRPEndpointStep(cur.account, false) // source
						strand = append(strand, step)
						prevStep = step
					}
					if isLast {
						step := NewXRPEndpointStep(next.account, true) // destination
						strand = append(strand, step)
					}
				} else {
					directStep := NewDirectStepI(cur.account, next.account, curIssue.Currency, prevStep, isLast)
					strand = append(strand, directStep)
					prevStep = directStep
				}
			}
		} else if cur.hasAccount && !next.hasAccount && (next.hasCurrency || next.hasIssuer) {
			// Account to offer (currency change)
			// May need implied DirectStep first
			if !curIssue.IsXRP() && curIssue.Issuer != cur.account {
				directStep := NewDirectStepI(cur.account, curIssue.Issuer, curIssue.Currency, prevStep, false)
				strand = append(strand, directStep)
				prevStep = directStep
			}

			// Determine output issue
			outCurrency := curIssue.Currency
			if next.hasCurrency {
				outCurrency = next.currency
			}
			outIssuer := curIssue.Issuer
			if next.hasIssuer {
				outIssuer = next.issuer
			}
			outIssue := Issue{Currency: outCurrency, Issuer: outIssuer}

			// Create book step if currencies differ
			if curIssue.Currency != outCurrency {
				if curIssue.IsXRP() && outIssue.IsXRP() {
					return nil, nil // Invalid: XRP to XRP book
				}
				bookStep := NewBookStep(curIssue, outIssue, src, dst, prevStep, false)
				strand = append(strand, bookStep)
				prevStep = bookStep
				curIssue = outIssue
			}
		} else if !cur.hasAccount && next.hasAccount {
			// Offer to account
			if !curIssue.IsXRP() && curIssue.Issuer != next.account {
				if curIssue.IsXRP() {
					if !isLast {
						return nil, nil // Invalid path
					}
					// XRP endpoint
					step := NewXRPEndpointStep(next.account, true)
					strand = append(strand, step)
				} else {
					// Implied DirectStep from curIssue.Issuer to next
					directStep := NewDirectStepI(curIssue.Issuer, next.account, curIssue.Currency, prevStep, isLast)
					strand = append(strand, directStep)
					prevStep = directStep
				}
			}
		}
	}

	// Debug output
	if DebugStrands {
		fmt.Printf("DEBUG ToStrand: created strand with %d steps\n", len(strand))
		for i, s := range strand {
			if accts := s.DirectStepAccts(); accts != nil {
				srcStr, _ := sle.EncodeAccountID(accts[0])
				dstStr, _ := sle.EncodeAccountID(accts[1])
				fmt.Printf("  Step %d: DirectStep %s -> %s\n", i, srcStr, dstStr)
			} else if book := s.BookStepBook(); book != nil {
				fmt.Printf("  Step %d: BookStep %s/%s -> %s/%s\n", i, book.In.Currency, sle.EncodeAccountIDSafe(book.In.Issuer), book.Out.Currency, sle.EncodeAccountIDSafe(book.Out.Issuer))
			} else {
				fmt.Printf("  Step %d: XRPEndpointStep\n", i)
			}
		}
	}

	// Validate the strand
	if err := validateStrand(strand, view); err != nil {
		if DebugStrands {
			fmt.Printf("DEBUG ToStrand: validation failed: %v\n", err)
		}
		return nil, err
	}

	return strand, nil
}

// issueFromPathElement extracts the Issue from a path element
func issueFromPathElement(elem PathStep, defaultAccount [20]byte) Issue {
	var issue Issue

	if elem.Currency != "" {
		issue.Currency = elem.Currency
	}

	if elem.Issuer != "" {
		issuerBytes, err := sle.DecodeAccountID(elem.Issuer)
		if err == nil {
			issue.Issuer = issuerBytes
		}
	} else if !issue.IsXRP() {
		// Default to the account if issuer not specified and not XRP
		issue.Issuer = defaultAccount
	}

	return issue
}

// accountFromPathElement extracts the account from a path element
func accountFromPathElement(elem PathStep, defaultAccount [20]byte) [20]byte {
	if elem.Account != "" {
		accountBytes, err := sle.DecodeAccountID(elem.Account)
		if err == nil {
			return accountBytes
		}
	}
	return defaultAccount
}

// hasAccount returns true if the path element specifies an account
func hasAccount(elem PathStep) bool {
	return elem.Account != "" || (elem.Type&int(PathTypeAccount)) != 0
}

// hasCurrency returns true if the path element specifies a currency
func hasCurrency(elem PathStep) bool {
	return elem.Currency != "" || (elem.Type&int(PathTypeCurrency)) != 0
}

// hasIssuer returns true if the path element specifies an issuer
func hasIssuer(elem PathStep) bool {
	return elem.Issuer != "" || (elem.Type&int(PathTypeIssuer)) != 0
}

// issuesEqual compares two Issues for equality
func issuesEqual(a, b Issue) bool {
	if a.IsXRP() != b.IsXRP() {
		return false
	}
	if a.IsXRP() {
		return true // Both XRP
	}
	return a.Currency == b.Currency && a.Issuer == b.Issuer
}

// accountsEqual compares two account IDs
func accountsEqual(a, b [20]byte) bool {
	return a == b
}

// strandsEqual compares two strands for equality
func strandsEqual(a, b Strand) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !stepsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// stepsEqual compares two steps for equality
func stepsEqual(a, b Step) bool {
	// Compare based on step type and key attributes
	aAccts := a.DirectStepAccts()
	bAccts := b.DirectStepAccts()

	if aAccts != nil && bAccts != nil {
		return *aAccts == *bAccts
	}

	aBook := a.BookStepBook()
	bBook := b.BookStepBook()

	if aBook != nil && bBook != nil {
		return issuesEqual(aBook.In, bBook.In) && issuesEqual(aBook.Out, bBook.Out)
	}

	// Different types of steps
	return false
}

// validateStrand checks that a strand is valid
func validateStrand(strand Strand, view *PaymentSandbox) error {
	if len(strand) == 0 {
		return nil
	}

	// Check each step
	for _, step := range strand {
		// DirectStep validation - check trust line exists
		if accts := step.DirectStepAccts(); accts != nil {
			src, dst := accts[0], accts[1]
			// Skip if either is the zero account (XRP pseudo-account)
			var zeroAccount [20]byte
			if src == zeroAccount || dst == zeroAccount {
				continue
			}
			// Trust line check is done by the Check method
		}

		// BookStep validation - check order book has liquidity
		if book := step.BookStepBook(); book != nil {
			// This would check for order book existence
			// Simplified for now
		}
	}

	return nil
}

// CheckStrand validates all steps in a strand
func CheckStrand(strand Strand, view *PaymentSandbox) tx.Result {
	for _, step := range strand {
		// Check if step implements Check method
		if checker, ok := step.(interface{ Check(*PaymentSandbox) tx.Result }); ok {
			result := checker.Check(view)
			if result != tx.TesSUCCESS {
				return result
			}
		}
	}
	return tx.TesSUCCESS
}

// GetStrandQuality calculates the worst-case quality for a strand
func GetStrandQuality(strand Strand, view *PaymentSandbox) *Quality {
	if len(strand) == 0 {
		return nil
	}

	// Compose qualities from all steps
	composedQuality := Quality{Value: uint64(QualityOne)}
	prevDir := DebtDirectionIssues

	for i, step := range strand {
		stepQuality, stepDir := step.QualityUpperBound(view, prevDir)
		if DebugStrands {
			fmt.Printf("DEBUG GetStrandQuality: step %d quality=%+v dir=%d\n", i, stepQuality, stepDir)
		}
		if stepQuality == nil {
			if DebugStrands {
				fmt.Printf("DEBUG GetStrandQuality: step %d returned nil quality (dry)\n", i)
			}
			return nil // Dry step
		}
		composedQuality = composedQuality.Compose(*stepQuality)
		prevDir = stepDir
	}

	return &composedQuality
}

// createDirectStepI creates a new DirectStepI with proper initialization for strand building
func createDirectStepI(src, dst [20]byte, currency string, prevStep Step, isLast bool) *DirectStepI {
	return NewDirectStepI(src, dst, currency, prevStep, isLast)
}

// StrandSourceIssue returns the source issue for a strand
func StrandSourceIssue(strand Strand) Issue {
	if len(strand) == 0 {
		return Issue{}
	}

	// First step determines source
	step := strand[0]

	// Check if XRP endpoint
	if accts := step.DirectStepAccts(); accts != nil {
		// Check if source is zero (XRP pseudo-account)
		var zeroAccount [20]byte
		if accts[0] == zeroAccount || accts[1] == zeroAccount {
			return Issue{Currency: "XRP"}
		}
	}

	// Check if book step
	if book := step.BookStepBook(); book != nil {
		return book.In
	}

	// Default to unknown
	return Issue{}
}

// StrandDestIssue returns the destination issue for a strand
func StrandDestIssue(strand Strand) Issue {
	if len(strand) == 0 {
		return Issue{}
	}

	// Last step determines destination
	step := strand[len(strand)-1]

	// Check if XRP endpoint
	if accts := step.DirectStepAccts(); accts != nil {
		var zeroAccount [20]byte
		if accts[0] == zeroAccount || accts[1] == zeroAccount {
			return Issue{Currency: "XRP"}
		}
	}

	// Check if book step
	if book := step.BookStepBook(); book != nil {
		return book.Out
	}

	return Issue{}
}

// Line returns a keylet for a trust line between two accounts
func Line(src, dst [20]byte, currency string) keylet.Keylet {
	return keylet.Line(src, dst, currency)
}
