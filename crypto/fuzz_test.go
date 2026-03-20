package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

func FuzzDERHexToSig(f *testing.F) {
	// Valid DER signature from test suite
	f.Add("3045022100E1617F1A3C85B5BC8FA6224F893FE9068BEA8F8D075EE144F6F9D255C829761802206FD9B361CDE83A0C3D5654232F1D7CFB1A614E9A8F9B1A861564029065516E64")
	// Empty input
	f.Add("")
	// Just a sequence tag
	f.Add("30")
	// Invalid hex characters
	f.Add("ZZZZ")
	// Empty DER sequence
	f.Add("3000")
	// Invalid signature tag
	f.Add("3145022100E1617F1A3C85B5BC8FA6224F893FE9068BEA8F8D075EE144F6F9D255C829761802206FD9B361CDE83A0C3D5654232F1D7CFB1A614E9A8F9B1A861564029065516E64")
	// Invalid length
	f.Add("3044022100E1617F1A3C85B5BC8FA6224F893FE9068BEA8F8D075EE144F6F9D255C829761802206FD9B361CDE83A0C3D5654232F1D7CFB1A614E9A8F9B1A861564029065516E64")
	// Leftover bytes
	f.Add("300702010102010101")
	// Minimal valid DER
	f.Add("3006020101020101")

	f.Fuzz(func(t *testing.T, hexSig string) {
		r, s, err := DERHexToSig(hexSig)
		if err != nil {
			return
		}

		// Round-trip: convert r,s back to DER hex and parse again
		rHex := hex.EncodeToString(r)
		sHex := hex.EncodeToString(s)

		roundTripped, err := DERHexFromSig(rHex, sHex)
		if err != nil {
			t.Fatalf("DERHexFromSig failed on output of DERHexToSig: r=%s s=%s err=%v", rHex, sHex, err)
		}

		// Parse the round-tripped signature
		r2, s2, err := DERHexToSig(roundTripped)
		if err != nil {
			t.Fatalf("DERHexToSig failed on round-tripped DER: %s err=%v", roundTripped, err)
		}

		// The r and s values should be identical after round-trip
		r2Hex := hex.EncodeToString(r2)
		s2Hex := hex.EncodeToString(s2)
		if !strings.EqualFold(rHex, r2Hex) {
			t.Fatalf("r mismatch after round-trip: %s != %s", rHex, r2Hex)
		}
		if !strings.EqualFold(sHex, s2Hex) {
			t.Fatalf("s mismatch after round-trip: %s != %s", sHex, s2Hex)
		}
	})
}

func FuzzECDSACanonicality(f *testing.F) {
	// Valid fully canonical DER signature
	validSig, _ := hex.DecodeString("304402206878b5690514437a2342405029426cc2b25b4a03fc396fef845d656cf62bad2c022018610a8d37f65ad02af907c8cb8f72becd0de43de7d5f42fefccb6c2a391a67c")
	f.Add(validSig)
	// Another valid signature from test data
	validSig2, _ := hex.DecodeString("3045022100E1617F1A3C85B5BC8FA6224F893FE9068BEA8F8D075EE144F6F9D255C829761802206FD9B361CDE83A0C3D5654232F1D7CFB1A614E9A8F9B1A861564029065516E64")
	f.Add(validSig2)
	// Too short (5 bytes)
	f.Add([]byte{0x30, 0x03, 0x02, 0x01, 0x01})
	// Too long (80 bytes)
	f.Add(make([]byte, 80))
	// Empty
	f.Add([]byte{})
	// Minimal valid
	f.Add([]byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x01})
	// Invalid sequence tag
	f.Add([]byte{0x31, 0x06, 0x02, 0x01, 0x00, 0x02, 0x01, 0x00})
	// Negative R (high bit set)
	f.Add([]byte{0x30, 0x06, 0x02, 0x01, 0x80, 0x02, 0x01, 0x01})

	f.Fuzz(func(t *testing.T, sig []byte) {
		result := ECDSACanonicality(sig)

		// Result must be one of the three valid enum values
		if result != CanonicityNone && result != CanonicityCanonical && result != CanonicityFullyCanonical {
			t.Fatalf("unexpected canonicality value: %d", result)
		}

		// If fully canonical, MakeSignatureCanonical must succeed and return fully canonical
		if result == CanonicityFullyCanonical {
			canonical := MakeSignatureCanonical(sig)
			if canonical == nil {
				t.Fatal("MakeSignatureCanonical returned nil for fully canonical input")
			}
			if ECDSACanonicality(canonical) != CanonicityFullyCanonical {
				t.Fatal("MakeSignatureCanonical output is not fully canonical")
			}
		}
	})
}

func FuzzEd25519Canonical(f *testing.F) {
	// 64 bytes of zeros
	f.Add(make([]byte, 64))
	// 63 bytes (wrong length)
	f.Add(make([]byte, 63))
	// 65 bytes (wrong length)
	f.Add(make([]byte, 65))
	// Empty
	f.Add([]byte{})
	// Valid ed25519 signature from test data
	validEd25519, _ := hex.DecodeString("e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b")
	f.Add(validEd25519)
	// Another valid ed25519 signature from tests
	validEd25519_2, _ := hex.DecodeString("C001CB8A9883497518917DD16391930F4FEE39CEA76C846CFF4330BA44ED19DC4730056C2C6D7452873DE8120A5023C6807135C6329A89A13BA1D476FE8E7100")
	f.Add(validEd25519_2)

	f.Fuzz(func(t *testing.T, sig []byte) {
		// Must not panic
		_ = Ed25519Canonical(sig)
	})
}

func FuzzMakeSignatureCanonical(f *testing.F) {
	// Valid fully canonical DER signature
	validSig, _ := hex.DecodeString("304402206878b5690514437a2342405029426cc2b25b4a03fc396fef845d656cf62bad2c022018610a8d37f65ad02af907c8cb8f72becd0de43de7d5f42fefccb6c2a391a67c")
	f.Add(validSig)
	// Another valid signature
	validSig2, _ := hex.DecodeString("3045022100E1617F1A3C85B5BC8FA6224F893FE9068BEA8F8D075EE144F6F9D255C829761802206FD9B361CDE83A0C3D5654232F1D7CFB1A614E9A8F9B1A861564029065516E64")
	f.Add(validSig2)
	// Empty
	f.Add([]byte{})
	// Short
	f.Add([]byte{0x30, 0x00})
	// Minimal valid
	f.Add([]byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x01})
	// Too short to be valid
	f.Add([]byte{0x30})
	// Just padding
	f.Add([]byte{0x00, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, sig []byte) {
		result := MakeSignatureCanonical(sig)

		if result != nil {
			// Output must be fully canonical
			c := ECDSACanonicality(result)
			if c != CanonicityFullyCanonical {
				t.Fatalf("MakeSignatureCanonical returned non-fully-canonical result: %d", c)
			}
		}
	})
}
