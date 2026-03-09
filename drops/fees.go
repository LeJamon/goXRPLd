package drops

type Fees struct {
	Base      XRPAmount
	Reserve   XRPAmount
	Increment XRPAmount
}

func (f *Fees) AccountReserve(ownerSize int64) XRPAmount {
	return f.Reserve + f.Increment.Mul(ownerSize)
}
