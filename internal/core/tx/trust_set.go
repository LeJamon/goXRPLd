package tx

import (
	"errors"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

func init() {
	Register(TypeTrustSet, func() Transaction {
		return &TrustSet{BaseTx: *NewBaseTx(TypeTrustSet, "")}
	})
}

// TrustSet creates or modifies a trust line between two accounts.
type TrustSet struct {
	BaseTx

	// LimitAmount defines the trust line (required)
	// The issuer field is the account to trust
	LimitAmount Amount `json:"LimitAmount" xrpl:"LimitAmount,amount"`

	// QualityIn is the quality in (1e9 = 1:1) - optional
	QualityIn *uint32 `json:"QualityIn,omitempty" xrpl:"QualityIn,omitempty"`

	// QualityOut is the quality out (1e9 = 1:1) - optional
	QualityOut *uint32 `json:"QualityOut,omitempty" xrpl:"QualityOut,omitempty"`
}

// TrustSet transaction flags
// Reference: rippled SetTrust.cpp
const (
	// tfSetfAuth authorizes the other party to hold currency
	TrustSetFlagSetfAuth uint32 = 0x00010000
	// tfSetNoRipple blocks rippling on this trust line
	TrustSetFlagSetNoRipple uint32 = 0x00020000
	// tfClearNoRipple clears the no ripple flag
	TrustSetFlagClearNoRipple uint32 = 0x00040000
	// tfSetFreeze freezes the trust line
	TrustSetFlagSetFreeze uint32 = 0x00100000
	// tfClearFreeze clears the freeze flag
	TrustSetFlagClearFreeze uint32 = 0x00200000
	// tfSetDeepFreeze deep freezes the trust line (requires featureDeepFreeze)
	TrustSetFlagSetDeepFreeze uint32 = 0x00400000
	// tfClearDeepFreeze clears the deep freeze flag
	TrustSetFlagClearDeepFreeze uint32 = 0x00800000

	// tfTrustSetMask is the mask for valid TrustSet transaction flags
	TrustSetFlagMask uint32 = ^(TrustSetFlagSetfAuth |
		TrustSetFlagSetNoRipple |
		TrustSetFlagClearNoRipple |
		TrustSetFlagSetFreeze |
		TrustSetFlagClearFreeze |
		TrustSetFlagSetDeepFreeze |
		TrustSetFlagClearDeepFreeze |
		TxFlagFullyCanonicalSig)
)

// QUALITY_ONE is the 1:1 quality ratio (1e9)
// Note: QualityOne is defined in payment_step.go

// NewTrustSet creates a new TrustSet transaction
func NewTrustSet(account string, limitAmount Amount) *TrustSet {
	return &TrustSet{
		BaseTx:      *NewBaseTx(TypeTrustSet, account),
		LimitAmount: limitAmount,
	}
}

// TxType returns the transaction type
func (t *TrustSet) TxType() Type {
	return TypeTrustSet
}

// Validate validates the TrustSet transaction
// Reference: rippled SetTrust.cpp preflight()
func (t *TrustSet) Validate() error {
	if err := t.BaseTx.Validate(); err != nil {
		return err
	}

	txFlags := t.GetFlags()

	// Check for invalid transaction flags
	// Reference: rippled SetTrust.cpp:81-85
	if txFlags&TrustSetFlagMask != 0 {
		return errors.New("temINVALID_FLAG: invalid transaction flags")
	}

	// LimitAmount must be an issued currency, not XRP
	// Reference: rippled SetTrust.cpp:102-107
	if t.LimitAmount.IsNative() {
		return errors.New("temBAD_LIMIT: cannot create trust line for XRP")
	}

	if t.LimitAmount.Currency == "" {
		return errors.New("temBAD_CURRENCY: currency is required")
	}

	// Check for XRP currency code
	// Reference: rippled SetTrust.cpp:109-113
	if t.LimitAmount.Currency == "XRP" {
		return errors.New("temBAD_CURRENCY: cannot use XRP as IOU currency")
	}

	// Negative limit is not allowed
	// Reference: rippled SetTrust.cpp:115-119
	if len(t.LimitAmount.Value) > 0 && t.LimitAmount.Value[0] == '-' {
		return errors.New("temBAD_LIMIT: negative credit limit")
	}

	// Check if destination makes sense
	// Reference: rippled SetTrust.cpp:122-128
	if t.LimitAmount.Issuer == "" {
		return errors.New("temDST_NEEDED: issuer is required")
	}

	// Cannot create trust line to self
	// Reference: rippled SetTrust.cpp:220-224
	if t.LimitAmount.Issuer == t.Account {
		return errors.New("temDST_IS_SRC: cannot create trust line to self")
	}

	// Check for contradictory NoRipple flags
	setNoRipple := txFlags&TrustSetFlagSetNoRipple != 0
	clearNoRipple := txFlags&TrustSetFlagClearNoRipple != 0
	if setNoRipple && clearNoRipple {
		return errors.New("temINVALID_FLAG: cannot set and clear NoRipple")
	}

	// Check for contradictory Freeze flags
	// Reference: rippled SetTrust.cpp:326-332
	setFreeze := txFlags&TrustSetFlagSetFreeze != 0
	clearFreeze := txFlags&TrustSetFlagClearFreeze != 0
	setDeepFreeze := txFlags&TrustSetFlagSetDeepFreeze != 0
	clearDeepFreeze := txFlags&TrustSetFlagClearDeepFreeze != 0

	if (setFreeze || setDeepFreeze) && (clearFreeze || clearDeepFreeze) {
		return errors.New("temINVALID_FLAG: cannot set and clear freeze in same transaction")
	}

	return nil
}

// Flatten returns a flat map of all transaction fields
func (t *TrustSet) Flatten() (map[string]any, error) {
	return ReflectFlatten(t)
}

// SetNoRipple sets the no ripple flag on this trust line
func (t *TrustSet) SetNoRipple() {
	flags := t.GetFlags() | TrustSetFlagSetNoRipple
	t.SetFlags(flags)
}

// ClearNoRipple clears the no ripple flag on this trust line
func (t *TrustSet) ClearNoRipple() {
	flags := t.GetFlags() | TrustSetFlagClearNoRipple
	t.SetFlags(flags)
}

// SetFreeze freezes this trust line
func (t *TrustSet) SetFreeze() {
	flags := t.GetFlags() | TrustSetFlagSetFreeze
	t.SetFlags(flags)
}

// Apply applies a TrustSet transaction to the ledger state.
// Reference: rippled SetTrust.cpp doApply
func (t *TrustSet) Apply(ctx *ApplyContext) Result {
	// TrustSet creates or modifies a trust line (RippleState object)

	// Cannot create trust line to self
	if t.LimitAmount.Issuer == ctx.Account.Account {
		return TemDST_IS_SRC
	}

	// Get the issuer account ID
	issuerAccountID, err := decodeAccountID(t.LimitAmount.Issuer)
	if err != nil {
		return TemBAD_ISSUER
	}
	issuerKey := keylet.Account(issuerAccountID)

	// Check issuer exists and get issuer account for flag checks
	issuerData, err := ctx.View.Read(issuerKey)
	if err != nil {
		return TecNO_ISSUER
	}
	issuerAccount, err := parseAccountRoot(issuerData)
	if err != nil {
		return TefINTERNAL
	}

	// Get the account ID
	accountID, _ := decodeAccountID(ctx.Account.Account)

	// Determine low/high accounts (for consistent trust line ordering)
	// bHigh = true means current account is the HIGH account
	bHigh := compareAccountIDsForLine(accountID, issuerAccountID) > 0

	// Get or create the trust line
	trustLineKey := keylet.Line(accountID, issuerAccountID, t.LimitAmount.Currency)

	trustLineExists, err := ctx.View.Exists(trustLineKey)
	if err != nil {
		return TefINTERNAL
	}

	// Parse transaction flags
	txFlags := uint32(0)
	if t.Flags != nil {
		txFlags = *t.Flags
	}

	bSetAuth := (txFlags & TrustSetFlagSetfAuth) != 0
	bSetNoRipple := (txFlags & TrustSetFlagSetNoRipple) != 0
	bClearNoRipple := (txFlags & TrustSetFlagClearNoRipple) != 0
	bSetFreeze := (txFlags & TrustSetFlagSetFreeze) != 0
	bClearFreeze := (txFlags & TrustSetFlagClearFreeze) != 0
	bSetDeepFreeze := (txFlags & TrustSetFlagSetDeepFreeze) != 0
	bClearDeepFreeze := (txFlags & TrustSetFlagClearDeepFreeze) != 0

	// Validate tfSetfAuth - requires issuer to have lsfRequireAuth set
	// Per rippled SetTrust.cpp preclaim: if bSetAuth && !(account.Flags & lsfRequireAuth) -> tefNO_AUTH_REQUIRED
	if bSetAuth && (ctx.Account.Flags&lsfRequireAuth) == 0 {
		return TefNO_AUTH_REQUIRED
	}

	// Validate freeze flags - cannot freeze if account has lsfNoFreeze set
	// Per rippled SetTrust.cpp preclaim: if bNoFreeze && (bSetFreeze || bSetDeepFreeze) -> tecNO_PERMISSION
	bNoFreeze := (ctx.Account.Flags & lsfNoFreeze) != 0
	if bNoFreeze && (bSetFreeze || bSetDeepFreeze) {
		return TecNO_PERMISSION
	}

	// Parse quality values from transaction
	// Per rippled: QUALITY_ONE (1e9 = 1000000000) is treated as default (stored as 0)
	const qualityOne uint32 = 1000000000
	var uQualityIn, uQualityOut uint32
	bQualityIn := t.QualityIn != nil
	bQualityOut := t.QualityOut != nil

	if bQualityIn {
		uQualityIn = *t.QualityIn
		if uQualityIn == qualityOne {
			uQualityIn = 0 // Normalize to default
		}
	}
	if bQualityOut {
		uQualityOut = *t.QualityOut
		if uQualityOut == qualityOne {
			uQualityOut = 0 // Normalize to default
		}
	}

	// Parse the limit amount
	// Per rippled SetTrust.cpp: saLimitAllow.setIssuer(account_)
	// The issuer of the limit is the account setting the trust line, not the LimitAmount.Issuer
	limitAmount := NewIOUAmount(t.LimitAmount.Value, t.LimitAmount.Currency, ctx.Account.Account)

	if !trustLineExists {
		// Check if setting zero limit without existing trust line
		if limitAmount.IsZero() && !bSetAuth && (!bQualityIn || uQualityIn == 0) && (!bQualityOut || uQualityOut == 0) {
			// Nothing to do - no trust line and setting default values
			return TesSUCCESS
		}

		// Check account has reserve for new trust line
		// Per rippled SetTrust.cpp:405-407, first 2 objects don't need extra reserve
		// Reference: The reserve required to create the line is 0 if ownerCount < 2,
		// otherwise it's accountReserve(ownerCount + 1)
		reserveCreate := ctx.ReserveForNewObject(ctx.Account.OwnerCount)
		if ctx.Account.Balance < reserveCreate {
			return TecINSUF_RESERVE_LINE
		}

		// Create new RippleState
		rs := &RippleState{
			Balance:           NewIOUAmount("0", t.LimitAmount.Currency, t.LimitAmount.Issuer),
			Flags:             0,
			LowNode:           0,
			HighNode:          0,
			PreviousTxnID:     ctx.TxHash,
			PreviousTxnLgrSeq: ctx.Config.LedgerSequence,
		}

		// Set the limit based on which side this account is
		// Per rippled trustCreate: limit issuers must be the respective account
		// LowLimit issuer = LOW account, HighLimit issuer = HIGH account
		if !bHigh {
			// Account is LOW, LimitAmount.Issuer is HIGH
			rs.LowLimit = limitAmount                                                                // issuer = account.Account (LOW)
			rs.HighLimit = NewIOUAmount("0", t.LimitAmount.Currency, t.LimitAmount.Issuer) // issuer = HIGH
			rs.Flags |= lsfLowReserve
		} else {
			// Account is HIGH, LimitAmount.Issuer is LOW
			rs.LowLimit = NewIOUAmount("0", t.LimitAmount.Currency, t.LimitAmount.Issuer) // issuer = LOW
			rs.HighLimit = limitAmount                                                                  // issuer = account.Account (HIGH)
			rs.Flags |= lsfHighReserve
		}

		// Handle Auth flag for new trust line
		if bSetAuth {
			if bHigh {
				rs.Flags |= lsfHighAuth
			} else {
				rs.Flags |= lsfLowAuth
			}
		}

		// Handle NoRipple flag from transaction
		if bSetNoRipple && !bClearNoRipple {
			if bHigh {
				rs.Flags |= lsfHighNoRipple
			} else {
				rs.Flags |= lsfLowNoRipple
			}
		}

		// Handle Freeze flag for new trust line
		// Per rippled computeFreezeFlags:
		//   if bSetFreeze && !bClearFreeze && !bNoFreeze -> set freeze
		if bSetFreeze && !bClearFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighFreeze
			} else {
				rs.Flags |= lsfLowFreeze
			}
		}

		// Handle DeepFreeze flag for new trust line
		// Per rippled computeFreezeFlags:
		//   if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze -> set deep freeze
		if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighDeepFreeze
			} else {
				rs.Flags |= lsfLowDeepFreeze
			}
		}

		// Handle QualityIn/QualityOut for new trust line
		if bQualityIn && uQualityIn != 0 {
			if bHigh {
				rs.HighQualityIn = uQualityIn
			} else {
				rs.LowQualityIn = uQualityIn
			}
		}
		if bQualityOut && uQualityOut != 0 {
			if bHigh {
				rs.HighQualityOut = uQualityOut
			} else {
				rs.LowQualityOut = uQualityOut
			}
		}

		// Determine the LOW and HIGH account IDs for directory operations
		// Low account is the one with the smaller account ID
		var lowAccountID, highAccountID [20]byte
		if !bHigh {
			// Current account is LOW
			lowAccountID = accountID
			highAccountID = issuerAccountID
		} else {
			// Current account is HIGH
			lowAccountID = issuerAccountID
			highAccountID = accountID
		}

		// Add trust line to LOW account's owner directory
		// Per rippled View.cpp trustCreate: insert into both accounts' directories
		lowDirKey := keylet.OwnerDir(lowAccountID)
		lowDirResult, err := ctx.Engine.dirInsert(ctx.View, lowDirKey, trustLineKey.Key, func(dir *DirectoryNode) {
			dir.Owner = lowAccountID
		})
		if err != nil {
			return TefINTERNAL
		}

		// Add trust line to HIGH account's owner directory
		highDirKey := keylet.OwnerDir(highAccountID)
		highDirResult, err := ctx.Engine.dirInsert(ctx.View, highDirKey, trustLineKey.Key, func(dir *DirectoryNode) {
			dir.Owner = highAccountID
		})
		if err != nil {
			return TefINTERNAL
		}

		// Set LowNode and HighNode on the RippleState (deletion hints)
		rs.LowNode = lowDirResult.Page
		rs.HighNode = highDirResult.Page

		// Serialize and insert the trust line
		trustLineData, err := serializeRippleState(rs)
		if err != nil {
			return TefINTERNAL
		}

		if err := ctx.View.Insert(trustLineKey, trustLineData); err != nil {
			return TefINTERNAL
		}

		// Increment owner count for the transaction sender
		ctx.Account.OwnerCount++

		// Directory, RippleState creation, and issuer account modifications tracked automatically by ApplyStateTable
	} else {
		// Modify existing trust line
		trustLineData, err := ctx.View.Read(trustLineKey)
		if err != nil {
			return TefINTERNAL
		}

		rs, err := parseRippleState(trustLineData)
		if err != nil {
			return TefINTERNAL
		}

		// Update the limit
		if !bHigh {
			rs.LowLimit = limitAmount
		} else {
			rs.HighLimit = limitAmount
		}

		// Handle Auth flag (can only be set, not cleared per rippled)
		if bSetAuth {
			if bHigh {
				rs.Flags |= lsfHighAuth
			} else {
				rs.Flags |= lsfLowAuth
			}
		}

		// Handle NoRipple flag
		// Per rippled SetTrust.cpp:577-584:
		// NoRipple can only be set if the balance from this account's perspective >= 0
		// Balance is stored from LOW account's perspective:
		//   - Positive balance means LOW owes HIGH
		//   - Negative balance means HIGH owes LOW
		// From HIGH's perspective: saHighBalance = -rs.Balance, so check rs.Balance <= 0
		// From LOW's perspective: saLowBalance = rs.Balance, so check rs.Balance >= 0
		if bSetNoRipple && !bClearNoRipple {
			// Check if balance from this account's perspective is >= 0
			balanceFromPerspective := true // Assume can set
			if rs.Balance.Value != nil {
				if bHigh {
					// HIGH account: balance from HIGH's perspective is >= 0 if stored balance <= 0
					balanceFromPerspective = rs.Balance.Value.Sign() <= 0
				} else {
					// LOW account: balance from LOW's perspective is >= 0 if stored balance >= 0
					balanceFromPerspective = rs.Balance.Value.Sign() >= 0
				}
			}
			// Only set NoRipple if balance from our perspective is non-negative
			if balanceFromPerspective {
				if bHigh {
					rs.Flags |= lsfHighNoRipple
				} else {
					rs.Flags |= lsfLowNoRipple
				}
			}
			// Note: If fix1578 amendment is enabled and balance < 0, we should return tecNO_PERMISSION
			// For now, we match pre-fix1578 behavior: silently don't set the flag
		} else if bClearNoRipple && !bSetNoRipple {
			if bHigh {
				rs.Flags &^= lsfHighNoRipple
			} else {
				rs.Flags &^= lsfLowNoRipple
			}
		}

		// Handle Freeze flag
		// Per rippled: bSetFreeze && !bClearFreeze && !bNoFreeze -> set freeze
		//              bClearFreeze && !bSetFreeze -> clear freeze
		if bSetFreeze && !bClearFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighFreeze
			} else {
				rs.Flags |= lsfLowFreeze
			}
		} else if bClearFreeze && !bSetFreeze {
			if bHigh {
				rs.Flags &^= lsfHighFreeze
			} else {
				rs.Flags &^= lsfLowFreeze
			}
		}

		// Handle DeepFreeze flag
		// Per rippled computeFreezeFlags:
		//   if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze -> set deep freeze
		//   if bClearDeepFreeze && !bSetDeepFreeze -> clear deep freeze
		if bSetDeepFreeze && !bClearDeepFreeze && !bNoFreeze {
			if bHigh {
				rs.Flags |= lsfHighDeepFreeze
			} else {
				rs.Flags |= lsfLowDeepFreeze
			}
		} else if bClearDeepFreeze && !bSetDeepFreeze {
			if bHigh {
				rs.Flags &^= lsfHighDeepFreeze
			} else {
				rs.Flags &^= lsfLowDeepFreeze
			}
		}

		// Handle QualityIn
		if bQualityIn {
			if uQualityIn != 0 {
				// Setting quality
				if bHigh {
					rs.HighQualityIn = uQualityIn
				} else {
					rs.LowQualityIn = uQualityIn
				}
			} else {
				// Clearing quality (setting to default)
				if bHigh {
					rs.HighQualityIn = 0
				} else {
					rs.LowQualityIn = 0
				}
			}
		}

		// Handle QualityOut
		if bQualityOut {
			if uQualityOut != 0 {
				// Setting quality
				if bHigh {
					rs.HighQualityOut = uQualityOut
				} else {
					rs.LowQualityOut = uQualityOut
				}
			} else {
				// Clearing quality (setting to default)
				if bHigh {
					rs.HighQualityOut = 0
				} else {
					rs.LowQualityOut = 0
				}
			}
		}

		// Normalize quality values (QUALITY_ONE -> 0)
		if rs.LowQualityIn == qualityOne {
			rs.LowQualityIn = 0
		}
		if rs.LowQualityOut == qualityOne {
			rs.LowQualityOut = 0
		}
		if rs.HighQualityIn == qualityOne {
			rs.HighQualityIn = 0
		}
		if rs.HighQualityOut == qualityOne {
			rs.HighQualityOut = 0
		}

		// Check if trust line should be deleted
		// Per rippled: bDefault = both sides have no reserve requirement
		// Reserve is needed if: quality != 0 || noRipple differs from default || freeze || limit || balance > 0
		bLowDefRipple := (issuerAccount.Flags & lsfDefaultRipple) != 0
		bHighDefRipple := (ctx.Account.Flags & lsfDefaultRipple) != 0
		if bHigh {
			bLowDefRipple = (issuerAccount.Flags & lsfDefaultRipple) != 0
			bHighDefRipple = (ctx.Account.Flags & lsfDefaultRipple) != 0
		} else {
			bLowDefRipple = (ctx.Account.Flags & lsfDefaultRipple) != 0
			bHighDefRipple = (issuerAccount.Flags & lsfDefaultRipple) != 0
		}

		bLowReserveSet := rs.LowQualityIn != 0 || rs.LowQualityOut != 0 ||
			((rs.Flags&lsfLowNoRipple) == 0) != bLowDefRipple ||
			(rs.Flags&lsfLowFreeze) != 0 || !rs.LowLimit.IsZero() ||
			(rs.Balance.Value != nil && rs.Balance.Value.Sign() > 0)

		bHighReserveSet := rs.HighQualityIn != 0 || rs.HighQualityOut != 0 ||
			((rs.Flags&lsfHighNoRipple) == 0) != bHighDefRipple ||
			(rs.Flags&lsfHighFreeze) != 0 || !rs.HighLimit.IsZero() ||
			(rs.Balance.Value != nil && rs.Balance.Value.Sign() < 0)

		bDefault := !bLowReserveSet && !bHighReserveSet

		if bDefault && rs.Balance.IsZero() {
			// Delete the trust line
			if err := ctx.View.Erase(trustLineKey); err != nil {
				return TefINTERNAL
			}

			// Decrement owner count
			if ctx.Account.OwnerCount > 0 {
				ctx.Account.OwnerCount--
			}

			// RippleState deletion tracked automatically by ApplyStateTable
		} else {
			// Update reserve flags based on reserve requirements
			if bLowReserveSet && (rs.Flags&lsfLowReserve) == 0 {
				rs.Flags |= lsfLowReserve
			} else if !bLowReserveSet && (rs.Flags&lsfLowReserve) != 0 {
				rs.Flags &^= lsfLowReserve
			}

			if bHighReserveSet && (rs.Flags&lsfHighReserve) == 0 {
				rs.Flags |= lsfHighReserve
			} else if !bHighReserveSet && (rs.Flags&lsfHighReserve) != 0 {
				rs.Flags &^= lsfHighReserve
			}

			// Update the trust line
			updatedData, err := serializeRippleState(rs)
			if err != nil {
				return TefINTERNAL
			}

			if err := ctx.View.Update(trustLineKey, updatedData); err != nil {
				return TefINTERNAL
			}

			// RippleState modification tracked automatically by ApplyStateTable
		}
	}

	return TesSUCCESS
}
