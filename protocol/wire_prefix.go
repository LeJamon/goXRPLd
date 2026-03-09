package protocol

const (
	WireTypeTransaction = iota
	WireTypeAccountState
	WireTypeInner
	WireTypeCompressedInner
	WireTypeTransactionWithMeta
)
