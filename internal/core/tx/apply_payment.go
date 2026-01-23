package tx

import (
	"fmt"
	"math"
	"math/big"

	"github.com/LeJamon/goXRPLd/internal/core/ledger/keylet"
)

// compareAccountIDsForLine compares account IDs for trust line ordering
func compareAccountIDsForLine(a, b [20]byte) int {
	for i := 0; i < 20; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// TransferRate constants (QUALITY_ONE = 1000000000)
const (
	qualityOne uint32 = 1000000000 // 1e9 = 100% (no fee)
)

// getTransferRate returns the transfer rate for an issuer account
// Returns qualityOne (1e9) if no transfer rate is set
func (e *Engine) getTransferRate(issuerAddress string) uint32 {
	issuerID, err := decodeAccountID(issuerAddress)
	if err != nil {
		return qualityOne
	}
	issuerKey := keylet.Account(issuerID)
	issuerData, err := e.view.Read(issuerKey)
	if err != nil {
		return qualityOne
	}
	issuerAccount, err := parseAccountRoot(issuerData)
	if err != nil {
		return qualityOne
	}
	if issuerAccount.TransferRate > 0 {
		return issuerAccount.TransferRate
	}
	return qualityOne
}

// getAccountIOUBalance returns the IOU balance an account holds for a specific currency/issuer
// Returns the balance from the trust line, accounting for which side is low/high
func (e *Engine) getAccountIOUBalance(accountAddress string, currency string, issuerAddress string) IOUAmount {
	accountID, err := decodeAccountID(accountAddress)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}
	issuerID, err := decodeAccountID(issuerAddress)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	trustLineKey := keylet.Line(accountID, issuerID, currency)
	trustLineData, err := e.view.Read(trustLineKey)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return IOUAmount{Value: big.NewFloat(0), Currency: currency, Issuer: issuerAddress}
	}

	// Determine account's balance based on low/high position
	// Balance is stored from low's perspective:
	// - Negative balance = low owes high (high holds tokens)
	// - Positive balance = high owes low (low holds tokens)
	accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0

	balance := rs.Balance
	if !accountIsLow {
		// Account is HIGH, negate to get their perspective
		// If balance is negative (low owes high), account holds tokens
		balance = balance.Negate()
	}

	// Positive balance means account holds tokens
	balance.Currency = currency
	balance.Issuer = issuerAddress
	return balance
}

// applyTransferFee applies the transfer fee to an amount
// Used when sending IOUs through offers
func applyTransferFee(amount IOUAmount, transferRate uint32) IOUAmount {
	if transferRate == qualityOne || transferRate == 0 {
		return amount
	}

	// Transfer rate is expressed as fraction of 1e9
	// Example: 1.01 (1% fee) = 1010000000
	// To apply: multiply amount by (transferRate / 1e9)
	// Use big.Float for full precision
	rate := new(big.Float).SetPrec(128).SetUint64(uint64(transferRate))
	one := new(big.Float).SetPrec(128).SetUint64(uint64(qualityOne))
	rateRatio := new(big.Float).SetPrec(128).Quo(rate, one)

	amountValue := new(big.Float).SetPrec(128).Set(amount.Value)
	adjustedValue := new(big.Float).SetPrec(128).Mul(amountValue, rateRatio)

	return IOUAmount{
		Value:    adjustedValue,
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// removeTransferFee removes the transfer fee from an amount
// Used to calculate the actual amount received after fees
func removeTransferFee(amount IOUAmount, transferRate uint32) IOUAmount {
	if transferRate == qualityOne || transferRate == 0 {
		return amount
	}

	// To remove fee: divide amount by (transferRate / 1e9)
	// Use big.Float for full precision
	rate := new(big.Float).SetPrec(128).SetUint64(uint64(transferRate))
	one := new(big.Float).SetPrec(128).SetUint64(uint64(qualityOne))
	rateRatio := new(big.Float).SetPrec(128).Quo(rate, one)

	amountValue := new(big.Float).SetPrec(128).Set(amount.Value)
	adjustedValue := new(big.Float).SetPrec(128).Quo(amountValue, rateRatio)

	return IOUAmount{
		Value:    adjustedValue,
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}


// isZeroHash256 checks if a hex string represents a zero 256-bit hash
func isZeroHash256(hexStr string) bool {
	// Zero hash is 64 hex zeros
	if len(hexStr) != 64 {
		return false
	}
	for _, c := range hexStr {
		if c != '0' {
			return false
		}
	}
	return true
}


// matchOffers attempts to match the new offer against existing offers
// Returns the amounts obtained and paid through matching
func (e *Engine) matchOffers(offer *OfferCreate, account *AccountRoot, view LedgerView) (takerGot, takerPaid Amount) {
	// Find matching offers by scanning the ledger
	// This is a simplified implementation - production would use book directories

	// XRPL Offer semantics (from offer CREATOR's perspective):
	// - TakerGets = what creator is SELLING
	// - TakerPays = what creator is BUYING
	//
	// Our offer: TakerGets=BTC (selling BTC), TakerPays=XRP (buying XRP)
	// Their offer: TakerGets=XRP (selling XRP), TakerPays=BTC (buying BTC)
	//
	// We want to find offers where:
	// - Their TakerGets (what they're selling) matches our TakerPays (what we want to buy)
	// - Their TakerPays (what they're buying) matches our TakerGets (what we're selling)

	// What we want to BUY (receive)
	wantCurrency := offer.TakerPays.Currency
	wantIssuer := offer.TakerPays.Issuer
	// What we're SELLING (paying)
	payCurrency := offer.TakerGets.Currency
	payIssuer := offer.TakerGets.Issuer

	// Determine if matching native XRP
	wantingXRP := offer.TakerPays.IsNative() // We want to receive XRP
	payingXRP := offer.TakerGets.IsNative()  // We're paying XRP

	// Collect matching offers
	type matchOffer struct {
		key     [32]byte
		offer   *LedgerOffer
		quality float64 // TakerPays/TakerGets (lower is better for us)
	}
	var matches []matchOffer

	// Iterate through ledger entries to find offers
	view.ForEach(func(key [32]byte, data []byte) bool {
		// Check if this is an offer (first byte after header indicates type)
		if len(data) < 3 {
			return true // continue
		}

		// Parse the entry type from serialized data
		// LedgerEntryType is the first field (UInt16)
		if data[0] != (fieldTypeUInt16<<4)|fieldCodeLedgerEntryType {
			return true // continue
		}
		entryType := uint16(data[1])<<8 | uint16(data[2])
		if entryType != 0x006F { // Offer type
			return true // continue
		}

		// Parse the offer
		ledgerOffer, err := parseLedgerOffer(data)
		if err != nil {
			return true // continue
		}

		// Skip if same account (can't match own offers)
		if ledgerOffer.Account == account.Account {
			return true // continue
		}

		// Check if this offer crosses with ours:
		// - Their TakerGets (what they're selling) = what we want to buy (our TakerPays)
		// - Their TakerPays (what they're buying) = what we're selling (our TakerGets)
		theirGetsMatchesWhatWeWant := false
		if wantingXRP && ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesWhatWeWant = true
		} else if !wantingXRP && !ledgerOffer.TakerGets.IsNative() {
			theirGetsMatchesWhatWeWant = ledgerOffer.TakerGets.Currency == wantCurrency &&
				ledgerOffer.TakerGets.Issuer == wantIssuer
		}

		theirPaysMatchesWhatWeSell := false
		if payingXRP && ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesWhatWeSell = true
		} else if !payingXRP && !ledgerOffer.TakerPays.IsNative() {
			theirPaysMatchesWhatWeSell = ledgerOffer.TakerPays.Currency == payCurrency &&
				ledgerOffer.TakerPays.Issuer == payIssuer
		}

		if !theirGetsMatchesWhatWeWant || !theirPaysMatchesWhatWeSell {
			return true // continue
		}

		// Calculate quality (price) of this offer
		// Quality = TakerPays / TakerGets (what they're selling / what they want)
		// Lower quality = better price for us
		quality := calculateQuality(ledgerOffer.TakerPays, ledgerOffer.TakerGets)

		matches = append(matches, matchOffer{
			key:     key,
			offer:   ledgerOffer,
			quality: quality,
		})

		return true // continue
	})

	// If no matches, return empty
	if len(matches) == 0 {
		return Amount{}, Amount{}
	}

	// Sort by quality (lowest/best first)
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].quality < matches[i].quality {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Calculate our offer's limit quality
	ourQuality := calculateQuality(offer.TakerPays, offer.TakerGets)

	// Match against offers
	var totalGot, totalPaid Amount
	remainingWant := offer.TakerPays // How much we still want to BUY (receive)
	remainingPay := offer.TakerGets  // How much we can still SELL (pay)

	for _, match := range matches {

		// Check if price crosses (their quality <= our inverse quality)
		// For us: we want high TakerGets, low TakerPays
		// For them: they want high TakerGets, low TakerPays
		// Match if: their_price <= 1/our_price
		// Use a small tolerance to account for floating-point precision issues
		// rippled uses integer-based quality which avoids this problem
		crossingThreshold := 1.0 / ourQuality
		// Tolerance: allow up to 1e-14 relative error (about 15 decimal digits precision)
		tolerance := crossingThreshold * 1e-10
		if match.quality > crossingThreshold+tolerance {
			continue // Price doesn't cross
		}

		// Calculate how much we can trade
		// theirGets = what they're SELLING = what we RECEIVE
		// theirPays = what they're BUYING = what we PAY
		originalTheirGets := match.offer.TakerGets
		originalTheirPays := match.offer.TakerPays
		theirGets := originalTheirGets
		theirPays := originalTheirPays

		// IMPORTANT: Limit amounts by actual balances
		// Reference: rippled OfferStream.cpp - checks ownerFunds to limit offers

		// 1. Check maker's funds for what they're selling (theirGets)
		// If maker is selling IOU, check their IOU balance
		if !theirGets.IsNative() && match.offer.Account != theirGets.Issuer {
			makerIOUBalance := e.getAccountIOUBalance(match.offer.Account, theirGets.Currency, theirGets.Issuer)
			if makerIOUBalance.Value.Sign() > 0 {
				offerGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
				if makerIOUBalance.Compare(offerGetsIOU) < 0 {
					// Maker has less than offer amount - scale down proportionally
					ratio := divideIOUAmounts(makerIOUBalance, offerGetsIOU)
					theirGets = Amount{
						Value:    formatIOUValue(makerIOUBalance.Value),
						Currency: theirGets.Currency,
						Issuer:   theirGets.Issuer,
					}
					// Scale theirPays proportionally
					if theirPays.IsNative() {
						theirPaysDrops, _ := parseDropsString(theirPays.Value)
						scaledPays := uint64(float64(theirPaysDrops) * ratio)
						theirPays = Amount{Value: formatDrops(scaledPays)}
					} else {
						theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
						scaledPays := multiplyIOUByRatio(theirPaysIOU, ratio)
						theirPays = Amount{
							Value:    formatIOUValue(scaledPays.Value),
							Currency: theirPays.Currency,
							Issuer:   theirPays.Issuer,
						}
					}
				}
			}
		}

		// 2. Check taker's funds for what they're paying (theirPays = what maker wants = what taker pays)
		// If taker is paying IOU, check their IOU balance and apply transfer fee
		// Reference: rippled applies transfer fee when IOUs move through issuer
		if !theirPays.IsNative() && account.Account != theirPays.Issuer {
			takerIOUBalance := e.getAccountIOUBalance(account.Account, theirPays.Currency, theirPays.Issuer)
			if takerIOUBalance.Value.Sign() > 0 {
				// Get the transfer rate for this IOU's issuer
				transferRate := e.getTransferRate(theirPays.Issuer)

				// Calculate effective amount maker will receive after transfer fee
				// effective = taker_balance / transfer_rate
				// This is the MAX the taker can deliver to the maker
				effectiveBalance := removeTransferFee(takerIOUBalance, transferRate)

				// The trade is limited by the taker's effective balance
				// If effectiveBalance < theirPays (what maker's offer portion wants), scale down
				offerPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
				if effectiveBalance.Compare(offerPaysIOU) < 0 {
					// The maker will only receive effectiveBalance amount
					// Scale the exchange proportionally
					ratio := divideIOUAmounts(effectiveBalance, offerPaysIOU)
					theirPays = Amount{
						Value:    formatIOUValue(effectiveBalance.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
					// Scale theirGets proportionally (round up to match rippled's mulRound)
					if theirGets.IsNative() {
						theirGetsDrops, _ := parseDropsString(theirGets.Value)
						scaledGetsFloat := float64(theirGetsDrops) * ratio
						scaledGets := uint64(math.Ceil(scaledGetsFloat))
						theirGets = Amount{Value: formatDrops(scaledGets)}
					} else {
						theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
						scaledGets := multiplyIOUByRatio(theirGetsIOU, ratio)
						theirGets = Amount{
							Value:    formatIOUValue(scaledGets.Value),
							Currency: theirGets.Currency,
							Issuer:   theirGets.Issuer,
						}
					}
				}
			}
		}

		// We want to receive as much as possible up to remainingWant (from their TakerGets)
		// We'll pay proportionally based on their exchange rate
		var gotAmount, paidAmount Amount

		if theirGets.IsNative() {
			// They're selling XRP (we receive XRP)
			theirGetsDrops, _ := parseDropsString(theirGets.Value)
			remainingWantDrops, _ := parseDropsString(remainingWant.Value)

			takeDrops := theirGetsDrops
			if takeDrops > remainingWantDrops {
				takeDrops = remainingWantDrops
			}

			gotAmount = Amount{Value: formatDrops(takeDrops)}

			// Calculate what we pay based on their rate
			// Rate: theirPays / theirGets (what they want per unit they sell)
			if takeDrops == theirGetsDrops {
				paidAmount = theirPays
			} else {
				// Partial fill - calculate proportionally
				ratio := float64(takeDrops) / float64(theirGetsDrops)
				if theirPays.IsNative() {
					theirPaysDrops, _ := parseDropsString(theirPays.Value)
					payDrops := uint64(float64(theirPaysDrops) * ratio)
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
					payValue := multiplyIOUByRatio(theirPaysIOU, ratio)
					paidAmount = Amount{
						Value:    formatIOUValue(payValue.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
				}
			}
		} else {
			// They're selling IOU (we receive IOU)
			theirGetsIOU := NewIOUAmount(theirGets.Value, theirGets.Currency, theirGets.Issuer)
			remainingWantIOU := NewIOUAmount(remainingWant.Value, remainingWant.Currency, remainingWant.Issuer)

			takeIOU := theirGetsIOU
			if takeIOU.Compare(remainingWantIOU) > 0 {
				takeIOU = remainingWantIOU
			}

			gotAmount = Amount{
				Value:    formatIOUValue(takeIOU.Value),
				Currency: theirGets.Currency,
				Issuer:   theirGets.Issuer,
			}

			// Calculate what we pay
			if takeIOU.Compare(theirGetsIOU) == 0 {
				paidAmount = theirPays
			} else {
				// Partial fill
				ratio := divideIOUAmounts(takeIOU, theirGetsIOU)
				if theirPays.IsNative() {
					theirPaysDrops, _ := parseDropsString(theirPays.Value)
					payDrops := uint64(float64(theirPaysDrops) * ratio)
					paidAmount = Amount{Value: formatDrops(payDrops)}
				} else {
					theirPaysIOU := NewIOUAmount(theirPays.Value, theirPays.Currency, theirPays.Issuer)
					payValue := multiplyIOUByRatio(theirPaysIOU, ratio)
					paidAmount = Amount{
						Value:    formatIOUValue(payValue.Value),
						Currency: theirPays.Currency,
						Issuer:   theirPays.Issuer,
					}
				}
			}
		}

		// Update the matched offer in the ledger
		// Calculate remaining amounts for matched offer
		// Their TakerGets decreases by what we took (gotAmount)
		// Their TakerPays decreases by what we gave them (paidAmount)
		// Use ORIGINAL amounts, not balance-limited effective amounts
		matchRemainingGets := subtractAmount(originalTheirGets, gotAmount)
		matchRemainingPays := subtractAmount(originalTheirPays, paidAmount)

		matchKey := keylet.Keylet{Key: match.key}
		matchKey.Type = 0x6F // Offer type

		if isZeroAmount(matchRemainingGets) || isZeroAmount(matchRemainingPays) {
			// Fully consumed - update offer with zeroed amounts first, then delete
			// Reference: rippled's TOffer::consume() updates TakerGets/TakerPays before offerDelete()
			// This ensures proper PreviousFields in DeletedNode metadata
			matchOfferData, readErr := view.Read(matchKey)
			if readErr == nil {
				consumedOffer, parseErr := parseLedgerOffer(matchOfferData)
				if parseErr == nil {
					consumedOffer.TakerGets = Amount{Value: "0", Currency: match.offer.TakerGets.Currency, Issuer: match.offer.TakerGets.Issuer}
					consumedOffer.TakerPays = Amount{Value: "0", Currency: match.offer.TakerPays.Currency, Issuer: match.offer.TakerPays.Issuer}
					if updatedOfferData, serErr := serializeLedgerOffer(consumedOffer); serErr == nil {
						view.Update(matchKey, updatedOfferData)
					}
				}
			}
			view.Erase(matchKey)

			// Decrement maker's OwnerCount
			// Reference: rippled offerDelete() adjusts owner count
			makerID, err := decodeAccountID(match.offer.Account)
			if err == nil {
				makerAccountKey := keylet.Account(makerID)
				makerAccountData, err := view.Read(makerAccountKey)
				if err == nil {
					makerAccount, err := parseAccountRoot(makerAccountData)
					if err == nil && makerAccount.OwnerCount > 0 {
						makerAccount.OwnerCount--
						updatedMakerData, err := serializeAccountRoot(makerAccount)
						if err == nil {
							view.Update(makerAccountKey, updatedMakerData)
							// Account modification tracked automatically by ApplyStateTable
						}
					}
				}
			}

			// Remove offer from book directory and delete if empty
			// Reference: rippled View.cpp offerDelete() calls dirRemove()
			bookDirKey := keylet.Keylet{Key: match.offer.BookDirectory}
			bookDirKey.Type = 0x64 // DirectoryNode type
			bookDirData, err := view.Read(bookDirKey)
			if err == nil {
				bookDir, err := parseDirectoryNode(bookDirData)
				if err == nil {
					// Remove the offer from the directory's Indexes
					newIndexes := make([][32]byte, 0, len(bookDir.Indexes))
					for _, idx := range bookDir.Indexes {
						if idx != match.key {
							newIndexes = append(newIndexes, idx)
						}
					}
					bookDir.Indexes = newIndexes

					if len(bookDir.Indexes) == 0 {
						// Directory is now empty - delete it (tracked by ApplyStateTable)
						view.Erase(bookDirKey)
					} else {
						// Directory still has entries - update it
						updatedBookDirData, err := serializeDirectoryNode(bookDir, true) // true = book directory
						if err == nil {
							view.Update(bookDirKey, updatedBookDirData)
						}
					}
				}
			}

			// Remove offer from owner directory
			// Reference: rippled View.cpp offerDelete() removes from owner dir via dirRemove()
			if makerID != [20]byte{} {
				ownerDirKey := keylet.OwnerDir(makerID)
				ownerDirData, err := view.Read(ownerDirKey)
				if err == nil {
					ownerDir, err := parseDirectoryNode(ownerDirData)
					if err == nil {
						// Remove the offer from the owner directory's Indexes
						newOwnerIndexes := make([][32]byte, 0, len(ownerDir.Indexes))
						for _, idx := range ownerDir.Indexes {
							if idx != match.key {
								newOwnerIndexes = append(newOwnerIndexes, idx)
							}
						}
						ownerDir.Indexes = newOwnerIndexes

						// Update owner directory (don't delete even if empty - owner dirs persist)
						// Modification tracked automatically by ApplyStateTable
						updatedOwnerDirData, err := serializeDirectoryNode(ownerDir, false) // false = owner directory
						if err == nil {
							view.Update(ownerDirKey, updatedOwnerDirData)
						}
					}
				}
			}

			// Offer deletion tracked automatically by ApplyStateTable
		} else {
			// Partially consumed - update offer
			match.offer.TakerGets = matchRemainingGets
			match.offer.TakerPays = matchRemainingPays
			match.offer.PreviousTxnID = e.currentTxHash
			match.offer.PreviousTxnLgrSeq = e.config.LedgerSequence

			updatedData, err := serializeLedgerOffer(match.offer)
			if err == nil {
				view.Update(matchKey, updatedData)
				// Offer modification tracked automatically by ApplyStateTable
			}
		}

		// Transfer funds for this match
		// If trade fails (insufficient funds), skip this match and continue
		if err := e.executeOfferTrade(account, match.offer, gotAmount, paidAmount, view); err != nil {
			continue // Skip this match if trade can't be executed
		}

		// Accumulate totals
		totalGot = addAmount(totalGot, gotAmount)
		totalPaid = addAmount(totalPaid, paidAmount)

		// Update remaining
		remainingWant = subtractAmount(remainingWant, gotAmount)
		remainingPay = subtractAmount(remainingPay, paidAmount)

		// Check if our offer is filled
		if isZeroAmount(remainingWant) {
			break
		}
	}

	return totalGot, totalPaid
}

// calculateQuality calculates the quality (price) of an offer
// Quality = TakerPays / TakerGets
func calculateQuality(pays, gets Amount) float64 {
	var paysVal, getsVal float64

	if pays.IsNative() {
		drops, _ := parseDropsString(pays.Value)
		paysVal = float64(drops)
	} else {
		iou := NewIOUAmount(pays.Value, pays.Currency, pays.Issuer)
		paysVal, _ = iou.Value.Float64()
	}

	if gets.IsNative() {
		drops, _ := parseDropsString(gets.Value)
		getsVal = float64(drops)
	} else {
		iou := NewIOUAmount(gets.Value, gets.Currency, gets.Issuer)
		getsVal, _ = iou.Value.Float64()
	}

	if getsVal == 0 {
		return 0
	}
	return paysVal / getsVal
}

// multiplyIOUByRatio multiplies an IOU amount by a ratio
func multiplyIOUByRatio(amount IOUAmount, ratio float64) IOUAmount {
	val, _ := amount.Value.Float64()
	newVal := val * ratio
	return IOUAmount{
		Value:    new(big.Float).SetFloat64(newVal),
		Currency: amount.Currency,
		Issuer:   amount.Issuer,
	}
}

// divideIOUAmounts divides two IOU amounts and returns the ratio
func divideIOUAmounts(a, b IOUAmount) float64 {
	aVal, _ := a.Value.Float64()
	bVal, _ := b.Value.Float64()
	if bVal == 0 {
		return 0
	}
	return aVal / bVal
}

// addAmount adds two amounts of the same type
func addAmount(a, b Amount) Amount {
	if a.Value == "" {
		return b
	}
	if b.Value == "" {
		return a
	}

	if a.IsNative() {
		aDrops, _ := parseDropsString(a.Value)
		bDrops, _ := parseDropsString(b.Value)
		return Amount{Value: formatDrops(aDrops + bDrops)}
	}

	aIOU := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	bIOU := NewIOUAmount(b.Value, b.Currency, b.Issuer)
	result := aIOU.Add(bIOU)
	return Amount{
		Value:    formatIOUValue(result.Value),
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

// isZeroAmount checks if an amount is zero or empty
func isZeroAmount(a Amount) bool {
	if a.Value == "" || a.Value == "0" {
		return true
	}
	if a.IsNative() {
		drops, _ := parseDropsString(a.Value)
		return drops == 0
	}
	iou := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	return iou.IsZero()
}

// executeOfferTrade executes the fund transfer for an offer trade
// Reference: rippled's offer crossing via flowCross
func (e *Engine) executeOfferTrade(taker *AccountRoot, maker *LedgerOffer, takerGot, takerPaid Amount, view LedgerView) error {
	// Get maker account
	makerAccountID, err := decodeAccountID(maker.Account)
	if err != nil {
		return err
	}
	makerKey := keylet.Account(makerAccountID)
	makerData, err := view.Read(makerKey)
	if err != nil {
		return err
	}
	makerAccount, err := parseAccountRoot(makerData)
	if err != nil {
		return err
	}

	// Transfer takerGot from maker to taker
	if takerGot.IsNative() {
		drops, _ := parseDropsString(takerGot.Value)

		// Verify maker has sufficient balance (including reserve)
		// Reference: rippled accountFunds checks available balance
		makerReserve := e.AccountReserve(makerAccount.OwnerCount)
		if makerAccount.Balance < drops+makerReserve {
			// Maker doesn't have sufficient funds
			return fmt.Errorf("maker has insufficient balance")
		}

		makerAccount.Balance -= drops
		taker.Balance += drops
	} else {
		// IOU transfer - update trust lines
		if err := e.transferIOU(maker.Account, taker.Account, takerGot, view); err != nil {
			return err
		}
	}

	// Transfer takerPaid from taker to maker
	if takerPaid.IsNative() {
		drops, _ := parseDropsString(takerPaid.Value)

		// Verify taker has sufficient balance (including reserve)
		takerReserve := e.AccountReserve(taker.OwnerCount)
		if taker.Balance < drops+takerReserve {
			// Taker doesn't have sufficient funds
			return fmt.Errorf("taker has insufficient balance")
		}

		taker.Balance -= drops
		makerAccount.Balance += drops
	} else {
		// IOU transfer - update trust lines
		// Apply transfer fee: taker sends MORE than maker receives
		// Reference: rippled applies transfer rate during offer crossing
		transferRate := e.getTransferRate(takerPaid.Issuer)
		takerPaidIOU := NewIOUAmount(takerPaid.Value, takerPaid.Currency, takerPaid.Issuer)

		// Calculate what taker must send to deliver takerPaid to maker
		takerSendsIOU := applyTransferFee(takerPaidIOU, transferRate)
		takerSends := Amount{
			Value:    formatIOUValue(takerSendsIOU.Value),
			Currency: takerPaid.Currency,
			Issuer:   takerPaid.Issuer,
		}

		// Transfer with fee: taker sends takerSends, maker receives takerPaid
		if err := e.transferIOUWithFee(taker.Account, maker.Account, takerSends, takerPaid, view); err != nil {
			return err
		}
	}

	// Update PreviousTxnID and LgrSeq for the maker account
	makerAccount.PreviousTxnID = e.currentTxHash
	makerAccount.PreviousTxnLgrSeq = e.config.LedgerSequence

	// Update maker account - modification tracked automatically by ApplyStateTable
	updatedMakerData, err := serializeAccountRoot(makerAccount)
	if err != nil {
		return err
	}
	view.Update(makerKey, updatedMakerData)

	return nil
}

// transferIOU transfers an IOU amount between accounts via trust lines
// Reference: rippled's flow engine for IOU transfers
func (e *Engine) transferIOU(fromAccount, toAccount string, amount Amount, view LedgerView) error {
	fromID, err := decodeAccountID(fromAccount)
	if err != nil {
		return err
	}
	toID, err := decodeAccountID(toAccount)
	if err != nil {
		return err
	}
	issuerID, err := decodeAccountID(amount.Issuer)
	if err != nil {
		return err
	}

	iouAmount := NewIOUAmount(amount.Value, amount.Currency, amount.Issuer)

	// Update from's trust line (decrease balance)
	fromIsIssuer := fromAccount == amount.Issuer
	toIsIssuer := toAccount == amount.Issuer

	if fromIsIssuer {
		// Issuer is sending - increase to's trust line balance
		trustLineKey := keylet.Line(toID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, toID, issuerID, iouAmount, true, view); err != nil {
			return err
		}
	} else if toIsIssuer {
		// Sending to issuer - decrease from's trust line balance
		trustLineKey := keylet.Line(fromID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, fromID, issuerID, iouAmount, false, view); err != nil {
			return err
		}
	} else {
		// Transfer between non-issuers
		// Decrease from's balance with issuer
		fromTrustKey := keylet.Line(fromID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(fromTrustKey, fromID, issuerID, iouAmount, false, view); err != nil {
			return err
		}

		// Increase to's balance with issuer
		toTrustKey := keylet.Line(toID, issuerID, amount.Currency)
		if err := e.updateTrustLineBalance(toTrustKey, toID, issuerID, iouAmount, true, view); err != nil {
			return err
		}
	}

	return nil
}

// transferIOUWithFee transfers IOU with transfer fee applied
// senderAmount is what the sender pays (includes fee)
// receiverAmount is what the receiver gets (after fee)
// Reference: rippled applies transfer rate during IOU transfers
func (e *Engine) transferIOUWithFee(fromAccount, toAccount string, senderAmount, receiverAmount Amount, view LedgerView) error {
	fromID, err := decodeAccountID(fromAccount)
	if err != nil {
		return err
	}
	toID, err := decodeAccountID(toAccount)
	if err != nil {
		return err
	}
	issuerID, err := decodeAccountID(senderAmount.Issuer)
	if err != nil {
		return err
	}

	senderIOU := NewIOUAmount(senderAmount.Value, senderAmount.Currency, senderAmount.Issuer)
	receiverIOU := NewIOUAmount(receiverAmount.Value, receiverAmount.Currency, receiverAmount.Issuer)

	fromIsIssuer := fromAccount == senderAmount.Issuer
	toIsIssuer := toAccount == senderAmount.Issuer

	if fromIsIssuer {
		// Issuer is sending - no transfer fee, increase to's trust line
		trustLineKey := keylet.Line(toID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, toID, issuerID, receiverIOU, true, view); err != nil {
			return err
		}
	} else if toIsIssuer {
		// Sending to issuer - no transfer fee, decrease from's trust line
		trustLineKey := keylet.Line(fromID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(trustLineKey, fromID, issuerID, senderIOU, false, view); err != nil {
			return err
		}
	} else {
		// Transfer between non-issuers - apply transfer fee
		// Sender pays senderAmount (includes fee)
		fromTrustKey := keylet.Line(fromID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(fromTrustKey, fromID, issuerID, senderIOU, false, view); err != nil {
			return err
		}

		// Receiver gets receiverAmount (after fee)
		toTrustKey := keylet.Line(toID, issuerID, senderAmount.Currency)
		if err := e.updateTrustLineBalance(toTrustKey, toID, issuerID, receiverIOU, true, view); err != nil {
			return err
		}
	}

	return nil
}

// updateTrustLineBalance updates a trust line balance
// RippleState balance semantics:
// - Negative balance = LOW owes HIGH (HIGH holds tokens)
// - Positive balance = HIGH owes LOW (LOW holds tokens)
func (e *Engine) updateTrustLineBalance(key keylet.Keylet, accountID, issuerID [20]byte, amount IOUAmount, increase bool, view LedgerView) error {
	trustLineData, err := view.Read(key)
	if err != nil {
		return fmt.Errorf("trust line not found: %w", err)
	}

	rs, err := parseRippleState(trustLineData)
	if err != nil {
		return fmt.Errorf("failed to parse trust line: %w", err)
	}

	accountIsLow := compareAccountIDsForLine(accountID, issuerID) < 0

	var newBalance IOUAmount
	if accountIsLow {
		// Account is LOW, issuer is HIGH
		// Positive balance = account holds tokens (HIGH owes LOW)
		if increase {
			newBalance = rs.Balance.Add(amount) // More positive = more holdings
		} else {
			newBalance = rs.Balance.Sub(amount) // Less positive = less holdings
		}
	} else {
		// Account is HIGH, issuer is LOW
		// Negative balance = account holds tokens (LOW owes HIGH)
		if increase {
			newBalance = rs.Balance.Sub(amount) // More negative = more holdings
		} else {
			newBalance = rs.Balance.Add(amount) // Less negative = less holdings
		}
	}
	// Ensure the new balance has the correct currency and issuer
	newBalance.Currency = amount.Currency
	newBalance.Issuer = amount.Issuer

	// Update the RippleState
	rs.Balance = newBalance
	rs.PreviousTxnID = e.currentTxHash
	rs.PreviousTxnLgrSeq = e.config.LedgerSequence

	updatedData, err := serializeRippleState(rs)
	if err != nil {
		return fmt.Errorf("failed to serialize trust line: %w", err)
	}

	// RippleState modification tracked automatically by ApplyStateTable
	if err := view.Update(key, updatedData); err != nil {
		return fmt.Errorf("failed to update trust line: %w", err)
	}

	return nil
}

// subtractAmount subtracts b from a
func subtractAmount(a, b Amount) Amount {
	if a.IsNative() {
		aDrops, _ := parseDropsString(a.Value)
		bDrops, _ := parseDropsString(b.Value)
		if bDrops >= aDrops {
			return Amount{Value: "0"}
		}
		return Amount{Value: formatDrops(aDrops - bDrops)}
	}

	aIOU := NewIOUAmount(a.Value, a.Currency, a.Issuer)
	bIOU := NewIOUAmount(b.Value, b.Currency, b.Issuer)
	result := aIOU.Sub(bIOU)
	if result.IsNegative() {
		return Amount{Value: "0", Currency: a.Currency, Issuer: a.Issuer}
	}
	return Amount{
		Value:    formatIOUValue(result.Value),
		Currency: a.Currency,
		Issuer:   a.Issuer,
	}
}

