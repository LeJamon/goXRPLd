package entry

import (
	"encoding/binary"

	"github.com/LeJamon/goXRPLd/internal/core/XRPAmount"
	"github.com/LeJamon/goXRPLd/internal/core/ledger/entry"
)

// FeeSettings represents the singleton fee settings ledger entry.
// This entry stores the current network fee configuration.
type FeeSettings struct {
	BaseEntry

	// Modern fee fields (XRPFees amendment)
	BaseFeeDrops         XRPAmount.XRPAmount
	ReserveBaseDrops     XRPAmount.XRPAmount
	ReserveIncrementDrops XRPAmount.XRPAmount

	// Legacy fee fields (deprecated, for backward compatibility)
	// These are used if XRPFees amendment is not enabled
	BaseFee            *uint64
	ReferenceFeeUnits  *uint32
	ReserveBase        *uint32
	ReserveIncrement   *uint32
}

// NewFeeSettings creates a new FeeSettings entry with the specified fees.
func NewFeeSettings(baseFee, reserveBase, reserveIncrement XRPAmount.XRPAmount) *FeeSettings {
	return &FeeSettings{
		BaseFeeDrops:          baseFee,
		ReserveBaseDrops:      reserveBase,
		ReserveIncrementDrops: reserveIncrement,
	}
}

// NewLegacyFeeSettings creates a FeeSettings entry using legacy fields.
// This is for networks where XRPFees amendment is not enabled.
func NewLegacyFeeSettings(baseFee uint64, refFeeUnits, reserveBase, reserveIncrement uint32) *FeeSettings {
	return &FeeSettings{
		BaseFee:           &baseFee,
		ReferenceFeeUnits: &refFeeUnits,
		ReserveBase:       &reserveBase,
		ReserveIncrement:  &reserveIncrement,
	}
}

// Type returns the ledger entry type for FeeSettings.
func (f *FeeSettings) Type() entry.Type {
	return entry.TypeFeeSettings
}

// Validate checks that the FeeSettings entry is valid.
func (f *FeeSettings) Validate() error {
	// At minimum, either modern or legacy fields should be set
	// Modern fields take precedence
	if f.BaseFeeDrops > 0 || f.ReserveBaseDrops > 0 || f.ReserveIncrementDrops > 0 {
		return nil
	}
	if f.BaseFee != nil || f.ReserveBase != nil {
		return nil
	}
	return nil // Empty fee settings is technically valid (uses defaults)
}

// Hash computes the hash for this FeeSettings entry.
func (f *FeeSettings) Hash() ([32]byte, error) {
	hash := f.BaseEntry.Hash()

	// Include fee values in hash
	var buf [24]byte
	binary.BigEndian.PutUint64(buf[0:8], uint64(f.BaseFeeDrops))
	binary.BigEndian.PutUint64(buf[8:16], uint64(f.ReserveBaseDrops))
	binary.BigEndian.PutUint64(buf[16:24], uint64(f.ReserveIncrementDrops))

	for i := 0; i < 24 && i < 32; i++ {
		hash[i] ^= buf[i]
	}

	return hash, nil
}

// GetBaseFee returns the base transaction fee.
func (f *FeeSettings) GetBaseFee() XRPAmount.XRPAmount {
	if f.BaseFeeDrops > 0 {
		return f.BaseFeeDrops
	}
	if f.BaseFee != nil {
		return XRPAmount.NewXRPAmount(int64(*f.BaseFee))
	}
	return XRPAmount.NewXRPAmount(10) // Default: 10 drops
}

// GetReserveBase returns the account reserve base.
func (f *FeeSettings) GetReserveBase() XRPAmount.XRPAmount {
	if f.ReserveBaseDrops > 0 {
		return f.ReserveBaseDrops
	}
	if f.ReserveBase != nil {
		return XRPAmount.NewXRPAmount(int64(*f.ReserveBase))
	}
	return XRPAmount.DropsPerXRP * 10 // Default: 10 XRP
}

// GetReserveIncrement returns the owner reserve increment.
func (f *FeeSettings) GetReserveIncrement() XRPAmount.XRPAmount {
	if f.ReserveIncrementDrops > 0 {
		return f.ReserveIncrementDrops
	}
	if f.ReserveIncrement != nil {
		return XRPAmount.NewXRPAmount(int64(*f.ReserveIncrement))
	}
	return XRPAmount.DropsPerXRP * 2 // Default: 2 XRP
}

// IsUsingModernFees returns true if using XRPFees amendment fields.
func (f *FeeSettings) IsUsingModernFees() bool {
	return f.BaseFeeDrops > 0 || f.ReserveBaseDrops > 0 || f.ReserveIncrementDrops > 0
}
