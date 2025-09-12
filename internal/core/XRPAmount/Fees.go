package XRPAmount

type Fees struct {
	base      XRPAmount
	reserve   XRPAmount
	increment XRPAmount
}

func (f *Fees) AccountReserve(ownerSize int64) XRPAmount {
	return f.reserve + f.increment.Mul(ownerSize)
}
