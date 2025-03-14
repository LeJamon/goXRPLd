package ledger

type LedgerHeader struct {
	ledger_index          uint32
	ledger_hash           [32]byte
	account_hash          [32]byte
	close_flags           uint8
	close_time            uint32
	close_time_resolution uint8
	closed                bool
	parent_hash           [32]byte
	total_coins           uint64
}
