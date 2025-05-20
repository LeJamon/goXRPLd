package protocol

// makeHashPrefix combines three ASCII characters into a 4-byte prefix with the last byte set to zero.
func makeHashPrefix(a, b, c byte) [4]byte {
	return [4]byte{a, b, c, 0}
}

// HashPrefix constants for different XRPL object hash domains.
// These MUST match the C++ enum values from the XRPL protocol.
var (
	HashPrefixTransactionID       = makeHashPrefix('T', 'X', 'N') // Transaction ID
	HashPrefixTxNode              = makeHashPrefix('S', 'N', 'D') // Transaction + Metadata
	HashPrefixLeafNode            = makeHashPrefix('M', 'L', 'N') // Account State
	HashPrefixInnerNode           = makeHashPrefix('M', 'I', 'N') // Inner node (v1 tree)
	HashPrefixLedgerMaster        = makeHashPrefix('L', 'W', 'R') // Ledger master signing
	HashPrefixTxSign              = makeHashPrefix('S', 'T', 'X') // TX for signing
	HashPrefixTxMultiSign         = makeHashPrefix('S', 'M', 'T') // TX for multi-sign
	HashPrefixValidation          = makeHashPrefix('V', 'A', 'L') // Validation
	HashPrefixProposal            = makeHashPrefix('P', 'R', 'P') // Proposal
	HashPrefixManifest            = makeHashPrefix('M', 'A', 'N') // Manifest
	HashPrefixPaymentChannelClaim = makeHashPrefix('C', 'L', 'M') // Channel Claim
	HashPrefixCredential          = makeHashPrefix('C', 'R', 'D') // Credential Signature
)
