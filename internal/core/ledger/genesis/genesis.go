package genesis

import crypto "github.com/LeJamon/goXRPLd/internal/crypto/common"

type LedgerInfo struct {
	seq
	drops
	parentHash
	txHash
	accountHash
	parentCloseTime
	closeTime
	closeTimeResolution
	closeFlags
}
return sha512Half(
HashPrefix::ledgerMaster,
std::uint32_t(info.seq),
std::uint64_t(info.drops.drops()),
info.parentHash,
info.txHash,
info.accountHash,
std::uint32_t(info.parentCloseTime.time_since_epoch().count()),
std::uint32_t(info.closeTime.time_since_epoch().count()),
std::uint8_t(info.closeTimeResolution.count()),
std::uint8_t(info.closeFlags));

// create a genesis ledger
func createGenesis() {

}

func calculateLedgerHash(info LedgerInfo) [32]byte {
return crypto.Sha512Half(info)
}
