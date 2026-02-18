package sle

import (
	"fmt"
	"testing"
)

func TestXRPLNumberAddDebug(t *testing.T) {
	// Test: -1.0 + 0.03350000000000001
	// Expected with switchover: -0.9665000000333333 = {-9665000000333333, -16}
	// Expected without switchover: -0.966500000033334 = {-9665000000333340, -16}

	SetNumberSwitchover(true)
	defer SetNumberSwitchover(false)

	// 1. Test XRPLNumber.Add directly
	x := NewXRPLNumber(-1000000000000000, -15)
	y := NewXRPLNumber(3350000000000001, -17)
	result := x.Add(y)
	fmt.Printf("XRPLNumber.Add({-1000000000000000,-15}, {3350000000000001,-17}) = {%d, %d}\n", result.mantissa, result.exponent)

	// 2. Test addIOUValues
	a := NewIOUAmountValue(-1000000000000000, -15)
	b := NewIOUAmountValue(3350000000000001, -17)
	r := addIOUValues(a, b)
	fmt.Printf("addIOUValues({-1000000000000000,-15}, {3350000000000001,-17}) = {%d, %d}\n", r.mantissa, r.exponent)

	// 3. Test through Amount.Sub (1.0 - 0.0335...)
	amtA := NewIssuedAmountFromValue(1000000000000000, -15, "USD", "gateway")
	amtB := NewIssuedAmountFromValue(3350000000000001, -17, "USD", "gateway")
	result2, err := amtA.Sub(amtB)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	iou2 := result2.IOU()
	fmt.Printf("Amount.Sub({1000000000000000,-15}, {3350000000000001,-17}) = {%d, %d} = %s\n", iou2.mantissa, iou2.exponent, result2.Value())

	// 4. Test: -1.0 + 0.0335... (as Amount.Add)
	amtC := NewIssuedAmountFromValue(-1000000000000000, -15, "USD", "gateway")
	result3, _ := amtC.Add(amtB)
	iou3 := result3.IOU()
	fmt.Printf("Amount.Add({-1000000000000000,-15}, {3350000000000001,-17}) = {%d, %d} = %s\n", iou3.mantissa, iou3.exponent, result3.Value())

	// 5. Check normalize with large mantissa
	n := NewXRPLNumber(334999999999999966, -19)
	fmt.Printf("NewXRPLNumber(334999999999999966, -19) = {%d, %d}\n", n.mantissa, n.exponent)

	// 6. MulRatio test: 3333333333333333e-17 * 1005/1000
	amtD := NewIssuedAmountFromValue(3333333333333333, -17, "USD", "gateway")
	mr := amtD.MulRatio(1005000000, 1000000000, true)
	iou4 := mr.IOU()
	fmt.Printf("MulRatio({3333333333333333,-17}, 1005000000, 1000000000, true) = {%d, %d} = %s\n", iou4.mantissa, iou4.exponent, mr.Value())

	// 7. What if we multiply differently? Test: 3333333333333333e-17 * 1005000000e-9
	na := NewXRPLNumber(3333333333333333, -17)
	nb := NewXRPLNumber(1005000000, -9)
	nmul := na.Mul(nb)
	fmt.Printf("XRPLNumber.Mul({3333333333333333,-17}, {1005000000,-9}) = {%d, %d}\n", nmul.mantissa, nmul.exponent)

	fmt.Printf("\nExpected with switchover: -0.9665000000333333 = {-9665000000333333, -16}\n")
}
