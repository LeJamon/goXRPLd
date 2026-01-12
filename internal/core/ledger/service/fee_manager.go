package service

import (
	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
)

// FeeManager manages fee settings and calculations.
type FeeManager struct {
	// Current fee settings
	fees XRPAmount.Fees

	// Default values
	defaultBaseFee         uint64
	defaultReserveBase     uint64
	defaultReserveIncrement uint64
}

// FeeSettings contains the current fee settings.
type FeeSettings struct {
	BaseFee          uint64
	ReserveBase      uint64
	ReserveIncrement uint64
	LoadBase         uint64
	LoadFactor       uint64
}

// NewFeeManager creates a new fee manager with default settings.
func NewFeeManager() *FeeManager {
	return &FeeManager{
		defaultBaseFee:         10,         // 10 drops
		defaultReserveBase:     10_000_000, // 10 XRP
		defaultReserveIncrement: 2_000_000,  // 2 XRP
	}
}

// GetCurrentFees returns the current fee settings.
func (m *FeeManager) GetCurrentFees() (baseFee, reserveBase, reserveIncrement uint64) {
	// Return current fees if set, otherwise defaults
	if m.fees.BaseFee > 0 {
		return uint64(m.fees.BaseFee), uint64(m.fees.ReserveBase), uint64(m.fees.ReserveIncrement)
	}
	return m.defaultBaseFee, m.defaultReserveBase, m.defaultReserveIncrement
}

// GetFeeSettings returns the complete fee settings.
func (m *FeeManager) GetFeeSettings() FeeSettings {
	baseFee, reserveBase, reserveIncrement := m.GetCurrentFees()
	return FeeSettings{
		BaseFee:          baseFee,
		ReserveBase:      reserveBase,
		ReserveIncrement: reserveIncrement,
		LoadBase:         256,
		LoadFactor:       256,
	}
}

// SetFees updates the fee settings.
func (m *FeeManager) SetFees(fees XRPAmount.Fees) {
	m.fees = fees
}

// CalculateTransactionFee calculates the fee for a transaction.
func (m *FeeManager) CalculateTransactionFee(feeString string, txType string) uint64 {
	baseFee, _, _ := m.GetCurrentFees()

	// Parse the fee string if provided
	if feeString != "" {
		var fee uint64
		for _, c := range feeString {
			if c >= '0' && c <= '9' {
				fee = fee*10 + uint64(c-'0')
			}
		}
		if fee > 0 {
			return fee
		}
	}

	// Return base fee
	return baseFee
}

// CalculateReserve calculates the reserve requirement for an account.
func (m *FeeManager) CalculateReserve(ownerCount uint32) uint64 {
	_, reserveBase, reserveIncrement := m.GetCurrentFees()
	return reserveBase + uint64(ownerCount)*reserveIncrement
}

// IsFeeSufficient checks if a fee meets the minimum requirement.
func (m *FeeManager) IsFeeSufficient(fee uint64) bool {
	baseFee, _, _ := m.GetCurrentFees()
	return fee >= baseFee
}
