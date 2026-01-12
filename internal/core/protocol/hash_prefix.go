package protocol

// HashPrefix defines the prefix bytes used in XRPL hashing operations.
// These prefixes provide domain separation for different hash contexts.
type HashPrefix [4]byte

var (
	// HashPrefixLedgerMaster is used for calculating ledger hashes
	HashPrefixLedgerMaster = HashPrefix{'L', 'W', 'R', 0x00}

	// HashPrefixInnerNode is used for inner nodes in SHAMap
	HashPrefixInnerNode = HashPrefix{'M', 'I', 'N', 0x00}

	// HashPrefixLeafNode is used for leaf nodes in SHAMap (transaction tree)
	HashPrefixLeafNode = HashPrefix{'M', 'L', 'N', 0x00}

	// HashPrefixTxNode is used for transaction with metadata in SHAMap
	HashPrefixTxNode = HashPrefix{'S', 'N', 'D', 0x00}

	// HashPrefixAccountStateEntry is used for account state entries
	HashPrefixAccountStateEntry = HashPrefix{'M', 'L', 'N', 0x00}

	// HashPrefixTransaction is used for signing transactions
	HashPrefixTransaction = HashPrefix{'S', 'T', 'X', 0x00}

	// HashPrefixTransactionID is used for computing transaction IDs
	HashPrefixTransactionID = HashPrefix{'T', 'X', 'N', 0x00}

	// HashPrefixValidation is used for validation messages
	HashPrefixValidation = HashPrefix{'V', 'A', 'L', 0x00}

	// HashPrefixProposal is used for consensus proposals
	HashPrefixProposal = HashPrefix{'P', 'R', 'P', 0x00}

	// HashPrefixManifest is used for validator manifests
	HashPrefixManifest = HashPrefix{'M', 'A', 'N', 0x00}

	// HashPrefixPaymentChannelClaim is used for payment channel claims
	HashPrefixPaymentChannelClaim = HashPrefix{'C', 'L', 'M', 0x00}
)

// Bytes returns the prefix as a byte slice
func (h HashPrefix) Bytes() []byte {
	return h[:]
}
