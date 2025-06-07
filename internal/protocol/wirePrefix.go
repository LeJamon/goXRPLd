package protocol

const (
	WireTypeTransaction = iota
	WireTypeAccountState
	WireTypeInner
	WireTypeCompressedInner
	WireTypeTransactionWithMeta // This one for your function
)
