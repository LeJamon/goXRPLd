package XRPAmount

import "fmt"

type XRPAmount int64

const DropsPerXRP XRPAmount = 1_000_000

func NewXRPAmount(drops int64) XRPAmount {
	return XRPAmount(drops)
}

func FromDecimalXRP(xrp float64) XRPAmount {
	return XRPAmount(xrp * float64(DropsPerXRP))
}

func (x XRPAmount) Drops() int64 {
	return int64(x)
}

func (x XRPAmount) DecimalXRP() float64 {
	return float64(x) / float64(DropsPerXRP)
}

func (x XRPAmount) Add(other XRPAmount) XRPAmount {
	return x + other
}

func (x XRPAmount) Sub(other XRPAmount) XRPAmount {
	return x - other
}

func (x XRPAmount) Mul(factor int64) XRPAmount {
	return x * XRPAmount(factor)
}

func (x XRPAmount) IsPositive() bool {
	return x > 0
}

func (x XRPAmount) IsZero() bool {
	return x == 0
}

func (x XRPAmount) String() string {
	return fmt.Sprintf("%d", int64(x))
}
